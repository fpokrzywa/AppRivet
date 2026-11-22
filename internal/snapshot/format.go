// Package snapshot creates and verifies source bundles. Integrity is not authorization.
package snapshot

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
	"time"
)

const (
	MaxFiles               = 4096
	MaxFileBytes     int64 = 16 << 20
	MaxTotalBytes    int64 = 64 << 20
	MaxManifestBytes       = 1 << 20
	MaxArchiveBytes  int64 = 70 << 20
)

type Entry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type Report struct {
	Format       string  `json:"format"`
	SHA256       string  `json:"sha256"`
	ArchiveBytes int64   `json:"archive_bytes"`
	TotalBytes   int64   `json:"total_bytes"`
	Files        []Entry `json:"files"`
}
type manifest struct {
	SchemaVersion int     `json:"schema_version"`
	Files         []Entry `json:"files"`
}
type Error struct{ Code, Message string }

func (e *Error) Error() string        { return e.Message }
func fail(code, message string) error { return &Error{code, message} }
func invalid() error {
	return fail("invalid_snapshot", "archive is not a complete canonical AppRivet source snapshot; no inventory accepted")
}
func limit() error { return fail("limit_exceeded", "snapshot exceeds a file, byte or record limit") }
func contextError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return fail("cancelled", "snapshot operation cancelled; no result accepted")
	}
	return err
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

type countingReader struct {
	r io.Reader
	n int64
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	r.n += int64(n)
	return n, err
}

func header(name string, size int64) *tar.Header {
	return &tar.Header{Name: name, Size: size, Mode: 0644, Typeflag: tar.TypeReg, ModTime: time.Unix(0, 0), Format: tar.FormatUSTAR}
}
func manifestBytes(files []Entry) []byte {
	b, _ := json.Marshal(manifest{SchemaVersion: 1, Files: files})
	return append(b, '\n')
}
func digestValid(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

// Verify consumes at most MaxArchiveBytes+1 bytes and never writes to a filesystem.
// Only our canonical format is supported, not general-purpose TAR imports.
func Verify(ctx context.Context, input io.Reader, expectedSHA256 string) (Report, error) {
	if err := contextError(ctx, nil); err != nil {
		return Report{}, err
	}
	if expectedSHA256 != "" && !digestValid(expectedSHA256) {
		return Report{}, fail("invalid_arguments", "expected SHA-256 must be 64 lowercase hexadecimal characters")
	}
	r, err := verify(ctx, input, expectedSHA256, nil)
	if err = contextError(ctx, err); err != nil {
		return Report{}, err
	}
	return r, nil
}

// A sink is only used inside a newly owned extraction directory. Verification
// still covers all archive bytes; callers remove output on any error.
type fileSink func(string) (io.WriteCloser, error)

func verify(ctx context.Context, input io.Reader, expected string, sink fileSink) (Report, error) {
	actualHash, canonicalHash := sha256.New(), sha256.New()
	counted := &countingReader{r: io.LimitReader(contextReader{ctx, input}, MaxArchiveBytes+1)}
	stream := io.TeeReader(counted, actualHash)
	reader := tar.NewReader(stream)
	canonical := tar.NewWriter(canonicalHash)
	report := Report{Format: "apprivet-source-v1", Files: []Entry{}}
	paths := pathSet{}
	last := ""
	for {
		h, err := reader.Next()
		if err != nil {
			return Report{}, invalid()
		}
		if h.Typeflag != tar.TypeReg || h.Format != tar.FormatUSTAR || h.Size < 0 {
			return Report{}, invalid()
		}
		if h.Name == "manifest.json" {
			if len(report.Files) == 0 {
				return Report{}, invalid()
			}
			if h.Size > MaxManifestBytes {
				return Report{}, limit()
			}
			body, err := io.ReadAll(reader)
			if err != nil {
				return Report{}, invalid()
			}
			// Byte equality also rejects duplicate/unknown JSON keys and altered hashes.
			wanted := manifestBytes(report.Files)
			if !bytes.Equal(body, wanted) {
				return Report{}, invalid()
			}
			if err = canonical.WriteHeader(header("manifest.json", int64(len(wanted)))); err != nil {
				return Report{}, invalid()
			}
			if _, err = canonical.Write(wanted); err != nil {
				return Report{}, invalid()
			}
			if _, err = reader.Next(); err != io.EOF {
				return Report{}, invalid()
			}
			if err = canonical.Close(); err != nil {
				return Report{}, invalid()
			}
			// Include every byte after TAR's EOF, so appended data/padding cannot hide.
			if _, err = io.Copy(io.Discard, stream); err != nil {
				return Report{}, invalid()
			}
			if counted.n > MaxArchiveBytes {
				return Report{}, limit()
			}
			if !bytes.Equal(actualHash.Sum(nil), canonicalHash.Sum(nil)) {
				return Report{}, invalid()
			}
			report.SHA256 = hex.EncodeToString(actualHash.Sum(nil))
			report.ArchiveBytes = counted.n
			if expected != "" && report.SHA256 != expected {
				return Report{}, fail("digest_mismatch", "snapshot differs from the expected SHA-256; no result accepted")
			}
			return report, nil
		}
		if !strings.HasPrefix(h.Name, "source/") {
			return Report{}, invalid()
		}
		name := strings.TrimPrefix(h.Name, "source/")
		if err := paths.add(name); err != nil {
			return Report{}, err
		}
		if name <= last {
			return Report{}, invalid()
		}
		last = name
		if len(report.Files) >= MaxFiles || h.Size > MaxFileBytes || h.Size > MaxTotalBytes-report.TotalBytes {
			return Report{}, limit()
		}
		if err = canonical.WriteHeader(header(h.Name, h.Size)); err != nil {
			return Report{}, invalid()
		}
		fileHash := sha256.New()
		var output io.WriteCloser
		writers := []io.Writer{canonical, fileHash}
		if sink != nil {
			output, err = sink(name)
			if err != nil {
				return Report{}, err
			}
			writers = append(writers, output)
		}
		n, err := io.Copy(io.MultiWriter(writers...), reader)
		if output != nil {
			closeErr := output.Close()
			if err != nil || closeErr != nil {
				return Report{}, ioError()
			}
		}
		if err != nil || n != h.Size {
			return Report{}, invalid()
		}
		report.TotalBytes += n
		report.Files = append(report.Files, Entry{Path: name, Size: n, SHA256: hex.EncodeToString(fileHash.Sum(nil))})
	}
}
