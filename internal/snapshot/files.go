package snapshot

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func ioError() error {
	return fail("io_error", "cannot access snapshot input/output; check file type, permissions and destination")
}
func unsafeSource() error {
	return fail("unsafe_source", "source must contain only selected regular files under real directories; symlinks and special files are refused")
}
func openRegular(name string) (*os.File, os.FileInfo, error) {
	before, err := os.Lstat(name)
	if err != nil {
		return nil, nil, ioError()
	}
	if !before.Mode().IsRegular() {
		return nil, nil, unsafeSource()
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, nil, ioError()
	}
	after, err := f.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		f.Close()
		return nil, nil, unsafeSource()
	}
	return f, after, nil
}

// ReadSelection accepts UTF-8 ASCII paths, optional UTF-8 BOM and LF/CRLF lines.
func ReadSelection(name string) ([]string, error) {
	f, info, err := openRegular(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if info.Size() > MaxManifestBytes {
		return nil, limit()
	}
	b, err := io.ReadAll(io.LimitReader(f, MaxManifestBytes+1))
	if err != nil {
		return nil, ioError()
	}
	if len(b) > MaxManifestBytes {
		return nil, limit()
	}
	var files []string
	for _, line := range strings.Split(strings.TrimPrefix(string(b), "\xef\xbb\xbf"), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line != "" {
			files = append(files, line)
		}
	}
	return selection(files)
}

func Inspect(ctx context.Context, name, expected string) (Report, error) {
	if err := contextError(ctx, nil); err != nil {
		return Report{}, err
	}
	if expected != "" && !digestValid(expected) {
		return Report{}, fail("invalid_arguments", "expected SHA-256 must be 64 lowercase hexadecimal characters")
	}
	f, info, err := openRegular(name)
	if err != nil {
		return Report{}, err
	}
	defer f.Close()
	if info.Size() > MaxArchiveBytes {
		return Report{}, limit()
	}
	r, err := Verify(ctx, f, expected)
	if err != nil {
		return Report{}, err
	}
	after, err := f.Stat()
	if err != nil || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		return Report{}, fail("source_changed", "archive changed during inspection; no result accepted")
	}
	return r, nil
}

func openSource(root *os.Root, name string) (*os.File, os.FileInfo, error) {
	parts := strings.Split(name, "/")
	var info os.FileInfo
	for i := range parts {
		var err error
		info, err = root.Lstat(strings.Join(parts[:i+1], "/"))
		if err != nil {
			return nil, nil, ioError()
		}
		if (i == len(parts)-1 && !info.Mode().IsRegular()) || (i < len(parts)-1 && !info.IsDir()) {
			return nil, nil, unsafeSource()
		}
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, nil, unsafeSource()
	}
	after, err := f.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(info, after) {
		f.Close()
		return nil, nil, unsafeSource()
	}
	return f, after, nil
}

func Create(ctx context.Context, rootName string, files []string, output string) (Report, error) {
	if err := contextError(ctx, nil); err != nil {
		return Report{}, err
	}
	ordered, err := selection(files)
	if err != nil {
		return Report{}, err
	}
	if rootName == "" || output == "" {
		return Report{}, fail("invalid_arguments", "root and output paths are required")
	}
	info, err := os.Lstat(rootName)
	if err != nil {
		return Report{}, ioError()
	}
	if !info.IsDir() {
		return Report{}, unsafeSource()
	}
	root, err := os.OpenRoot(rootName)
	if err != nil {
		return Report{}, ioError()
	}
	defer root.Close()
	outAbs, err := filepath.Abs(output)
	if err != nil {
		return Report{}, ioError()
	}
	for _, name := range ordered {
		inputAbs, err := filepath.Abs(filepath.Join(rootName, filepath.FromSlash(name)))
		if err != nil {
			return Report{}, ioError()
		}
		if strings.EqualFold(inputAbs, outAbs) {
			return Report{}, fail("invalid_arguments", "output must not be a selected source file")
		}
	}
	out, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if errors.Is(err, os.ErrExist) {
		return Report{}, fail("archive_exists", "output already exists; choose a new archive path")
	}
	if err != nil {
		return Report{}, ioError()
	}
	keep := false
	defer func() {
		out.Close()
		if !keep {
			_ = os.Remove(output)
		}
	}()
	w := tar.NewWriter(out)
	entries := make([]Entry, 0, len(ordered))
	var total int64
	for _, name := range ordered {
		entry, err := copySource(ctx, root, w, name, MaxTotalBytes-total)
		if err != nil {
			return Report{}, contextError(ctx, err)
		}
		entries = append(entries, entry)
		total += entry.Size
	}
	b := manifestBytes(entries)
	if len(b) > MaxManifestBytes {
		return Report{}, limit()
	}
	if err = w.WriteHeader(header("manifest.json", int64(len(b)))); err != nil {
		return Report{}, ioError()
	}
	if _, err = w.Write(b); err != nil {
		return Report{}, ioError()
	}
	if err = w.Close(); err != nil {
		return Report{}, ioError()
	}
	if err = out.Close(); err != nil {
		return Report{}, ioError()
	}
	// The same independent verifier accepts created archives before publication succeeds.
	report, err := Inspect(ctx, output, "")
	if err != nil {
		return Report{}, err
	}
	keep = true
	return report, nil
}

func copySource(ctx context.Context, root *os.Root, w *tar.Writer, name string, remaining int64) (Entry, error) {
	if err := contextError(ctx, nil); err != nil {
		return Entry{}, err
	}
	f, info, err := openSource(root, name)
	if err != nil {
		return Entry{}, err
	}
	defer f.Close()
	if info.Size() > MaxFileBytes || info.Size() > remaining {
		return Entry{}, limit()
	}
	if err = w.WriteHeader(header("source/"+name, info.Size())); err != nil {
		return Entry{}, ioError()
	}
	h := sha256.New()
	n, err := io.CopyN(io.MultiWriter(w, h), contextReader{ctx, f}, info.Size())
	if err != nil || n != info.Size() {
		return Entry{}, fail("source_changed", "source changed or became unreadable during packaging")
	}
	var extra [1]byte
	nExtra, readErr := f.Read(extra[:])
	after, statErr := f.Stat()
	if nExtra != 0 || readErr != io.EOF || statErr != nil || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		return Entry{}, fail("source_changed", "source changed during packaging")
	}
	return Entry{Path: name, Size: n, SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}
