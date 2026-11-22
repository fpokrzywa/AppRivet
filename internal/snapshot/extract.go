package snapshot

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Extraction struct {
	Destination string `json:"destination"`
	Snapshot    Report `json:"snapshot"`
}

// Extract verifies an immutable private copy before creating a destination.
// It requires a separately selected digest and never executes source.
func Extract(ctx context.Context, archive, destination, expected string) (Extraction, error) {
	if err := contextError(ctx, nil); err != nil {
		return Extraction{}, err
	}
	if !digestValid(expected) {
		return Extraction{}, fail("invalid_arguments", "extraction requires an expected SHA-256 of 64 lowercase hexadecimal characters")
	}
	f, info, err := openRegular(archive)
	if err != nil {
		return Extraction{}, err
	}
	defer f.Close()
	if info.Size() > MaxArchiveBytes {
		return Extraction{}, limit()
	}
	return extractFrom(ctx, f, destination, expected)
}

func extractFrom(ctx context.Context, input io.Reader, destination, expected string) (Extraction, error) {
	if err := contextError(ctx, nil); err != nil {
		return Extraction{}, err
	}
	if !digestValid(expected) {
		return Extraction{}, fail("invalid_arguments", "extraction requires an expected SHA-256")
	}
	parent, _, _, err := destinationParent(destination)
	if err != nil {
		return Extraction{}, err
	}
	parent.Close()
	staged, err := os.CreateTemp("", "apprivet-verified-*.tar")
	if err != nil {
		return Extraction{}, ioError()
	}
	defer func() { staged.Close(); _ = os.Remove(staged.Name()) }()
	if _, err = Verify(ctx, io.TeeReader(input, staged), expected); err != nil {
		return Extraction{}, err
	}
	if _, err = staged.Seek(0, io.SeekStart); err != nil {
		return Extraction{}, ioError()
	}
	return materialize(ctx, staged, destination, expected)
}

func destinationParent(destination string) (*os.Root, string, string, error) {
	if destination == "" {
		return nil, "", "", fail("invalid_arguments", "a new destination directory is required")
	}
	abs, err := filepath.Abs(destination)
	if err != nil {
		return nil, "", "", ioError()
	}
	parentName := filepath.Dir(abs)
	name := filepath.Base(abs)
	if abs == parentName {
		return nil, "", "", fail("unsafe_destination", "a filesystem root cannot be an extraction destination")
	}
	info, err := os.Lstat(parentName)
	if err != nil {
		return nil, "", "", ioError()
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, "", "", fail("unsafe_destination", "destination parent must be an existing real directory")
	}
	parent, err := os.OpenRoot(parentName)
	if err != nil {
		return nil, "", "", ioError()
	}
	opened, err := parent.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		parent.Close()
		return nil, "", "", fail("unsafe_destination", "destination parent changed while opening")
	}
	if _, err = parent.Lstat(name); err == nil {
		parent.Close()
		return nil, "", "", fail("destination_exists", "destination already exists; extraction never merges or overwrites")
	}
	if !errors.Is(err, os.ErrNotExist) {
		parent.Close()
		return nil, "", "", ioError()
	}
	return parent, name, abs, nil
}

func materialize(ctx context.Context, input io.Reader, destination, expected string) (result Extraction, resultErr error) {
	if err := contextError(ctx, nil); err != nil {
		return Extraction{}, err
	}
	parent, name, abs, err := destinationParent(destination)
	if err != nil {
		return Extraction{}, err
	}
	defer parent.Close()
	if err = parent.Mkdir(name, 0700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return Extraction{}, fail("destination_exists", "destination already exists; extraction never merges or overwrites")
		}
		return Extraction{}, ioError()
	}
	owned, err := parent.Lstat(name)
	if err != nil {
		return Extraction{}, fail("cleanup_failed", "cannot establish ownership of the new destination; inspect it before retrying")
	}
	keep := false
	defer func() {
		if !keep {
			if err := removeOwned(parent, name, owned); err != nil {
				result = Extraction{}
				resultErr = err
			}
		}
	}()
	root, err := parent.OpenRoot(name)
	if err != nil {
		return Extraction{}, ioError()
	}
	defer root.Close()
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(owned, opened) {
		return Extraction{}, fail("unsafe_destination", "new destination changed while opening")
	}
	r, err := verify(ctx, input, expected, func(path string) (io.WriteCloser, error) { return createExtractedFile(root, path) })
	if err = contextError(ctx, err); err != nil {
		return Extraction{}, err
	}
	keep = true
	return Extraction{Destination: abs, Snapshot: r}, nil
}

func removeOwned(parent *os.Root, name string, owned os.FileInfo) error {
	current, err := parent.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || current.Mode()&os.ModeSymlink != 0 || !current.IsDir() || !os.SameFile(current, owned) {
		return fail("cleanup_failed", "destination ownership changed; refused recursive cleanup")
	}
	// name is the single basename we exclusively created, anchored to its opened parent.
	if err = parent.RemoveAll(name); err != nil {
		return fail("cleanup_failed", "could not remove the partial destination; inspect it before retrying")
	}
	return nil
}

func createExtractedFile(root *os.Root, name string) (io.WriteCloser, error) {
	parts := strings.Split(name, "/")
	for i := 0; i < len(parts)-1; i++ {
		dir := strings.Join(parts[:i+1], "/")
		if err := root.Mkdir(dir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, ioError()
		}
		info, err := root.Lstat(dir)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fail("unsafe_destination", "extraction encountered a non-directory or link")
		}
	}
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, fail("unsafe_destination", "extraction refused an existing or inaccessible file")
	}
	return f, nil
}
