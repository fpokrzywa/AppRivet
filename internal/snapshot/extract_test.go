package snapshot

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func hashBytes(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func archiveFile(t *testing.T, b []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "input.tar")
	if err := os.WriteFile(p, b, 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExtractVerifiedBytesAndPrivateModes(t *testing.T) {
	b := validBytes(t)
	dest := filepath.Join(t.TempDir(), "source")
	r, err := Extract(context.Background(), archiveFile(t, b), dest, hashBytes(b))
	if err != nil {
		t.Fatal(err)
	}
	if r.Destination != dest || r.Snapshot.SHA256 != hashBytes(b) {
		t.Fatalf("bad extraction: %+v", r)
	}
	for _, e := range r.Snapshot.Files {
		body, err := os.ReadFile(filepath.Join(dest, filepath.FromSlash(e.Path)))
		if err != nil || hashBytes(body) != e.SHA256 {
			t.Fatalf("bad materialization %s: %v", e.Path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dest, "manifest.json")); !os.IsNotExist(err) {
		t.Fatal("internal manifest extracted")
	}
	if runtime.GOOS != "windows" {
		for _, name := range []string{dest, filepath.Join(dest, "App"), filepath.Join(dest, "App/Main.swift")} {
			info, err := os.Stat(name)
			if err != nil {
				t.Fatal(err)
			}
			want := os.FileMode(0600)
			if info.IsDir() {
				want = 0700
			}
			if info.Mode().Perm() != want {
				t.Fatalf("mode %s %v", name, info.Mode())
			}
		}
	}
}

func TestExtractFailsClosedWithoutPublishing(t *testing.T) {
	good := validBytes(t)
	bad := bytes.Replace(good, []byte("hello"), []byte("HELLO"), 1)
	for _, tc := range []struct {
		b            []byte
		digest, code string
	}{{good, "", "invalid_arguments"}, {good, "bad", "invalid_arguments"}, {good, strings.Repeat("0", 64), "digest_mismatch"}, {bad, hashBytes(bad), "invalid_snapshot"}, {good[:512], hashBytes(good[:512]), "invalid_snapshot"}} {
		dest := filepath.Join(t.TempDir(), "target")
		r, err := Extract(context.Background(), archiveFile(t, tc.b), dest, tc.digest)
		wantCode(t, err, tc.code)
		if r.Destination != "" {
			t.Fatal("partial result")
		}
		if _, err = os.Lstat(dest); !os.IsNotExist(err) {
			t.Fatal("invalid snapshot published")
		}
	}
	for _, item := range []archiveItem{{"source/../escape", tar.TypeReg, []byte("x"), 0644}, {"source/link", tar.TypeSymlink, nil, 0644}, {"source/link", tar.TypeLink, nil, 0644}, {"source/a", tar.TypeDir, nil, 0755}} {
		b := forged(t, []archiveItem{item})
		parent := t.TempDir()
		dest := filepath.Join(parent, "target")
		if _, err := Extract(context.Background(), archiveFile(t, b), dest, hashBytes(b)); err == nil {
			t.Fatal("unsafe archive accepted")
		}
		entries, _ := os.ReadDir(parent)
		if len(entries) != 0 {
			t.Fatal("unsafe archive wrote outside staging")
		}
	}
}

func TestExtractNeverMergesOrOverwrites(t *testing.T) {
	b := validBytes(t)
	input := archiveFile(t, b)
	for _, directory := range []bool{false, true} {
		dest := filepath.Join(t.TempDir(), "occupied")
		if directory {
			if err := os.Mkdir(dest, 0700); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := os.WriteFile(dest, []byte("keep"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		_, err := Extract(context.Background(), input, dest, hashBytes(b))
		wantCode(t, err, "destination_exists")
		info, err := os.Stat(dest)
		if err != nil || info.IsDir() != directory {
			t.Fatal("existing destination changed")
		}
		if !directory {
			body, _ := os.ReadFile(dest)
			if string(body) != "keep" {
				t.Fatal("existing file overwritten")
			}
		}
	}
	outside := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("symlinks unavailable")
	}
	_, err := Extract(context.Background(), input, link, hashBytes(b))
	wantCode(t, err, "destination_exists")
	_, err = Extract(context.Background(), input, filepath.Join(link, "child"), hashBytes(b))
	wantCode(t, err, "unsafe_destination")
	entries, _ := os.ReadDir(outside)
	if len(entries) != 0 {
		t.Fatal("symlink destination escaped")
	}
}

type mutateAtEOF struct {
	io.Reader
	action func()
}

func (r mutateAtEOF) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		r.action()
	}
	return n, err
}
func TestExtractionUsesStableVerifiedCopy(t *testing.T) {
	b := validBytes(t)
	original := archiveFile(t, b)
	dest := filepath.Join(t.TempDir(), "target")
	f, err := os.Open(original)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r, err := extractFrom(context.Background(), mutateAtEOF{f, func() {
		if err := os.WriteFile(original, []byte("replaced"), 0600); err != nil {
			t.Fatal(err)
		}
	}}, dest, hashBytes(b))
	if err != nil || r.Snapshot.SHA256 != hashBytes(b) {
		t.Fatalf("%+v %v", r, err)
	}
	data, _ := os.ReadFile(filepath.Join(dest, "App/Main.swift"))
	if !bytes.Contains(data, []byte("hello")) {
		t.Fatal("re-read replaced input")
	}
}

func TestCleanupRefusesReplacedDestination(t *testing.T) {
	parentName := t.TempDir()
	parent, err := os.OpenRoot(parentName)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	if err = parent.Mkdir("target", 0700); err != nil {
		t.Fatal(err)
	}
	owned, err := parent.Lstat("target")
	if err != nil {
		t.Fatal(err)
	}
	if err = parent.Rename("target", "original"); err != nil {
		t.Fatal(err)
	}
	if err = parent.Mkdir("target", 0700); err != nil {
		t.Fatal(err)
	}
	if err = parent.WriteFile("target/keep", []byte("foreign"), 0600); err != nil {
		t.Fatal(err)
	}
	codeErr := removeOwned(parent, "target", owned)
	wantCode(t, codeErr, "cleanup_failed")
	if b, err := parent.ReadFile("target/keep"); err != nil || string(b) != "foreign" {
		t.Fatal("deleted foreign destination")
	}
}

func TestExtractionSinkRefusesPreplantedFilesAndLinks(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err = root.WriteFile("keep", []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = createExtractedFile(root, "keep"); err == nil {
		t.Fatal("overwrote existing file")
	}
	outside := t.TempDir()
	if err = os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Skip("symlinks unavailable")
	}
	if _, err = createExtractedFile(root, "link/escape"); err == nil {
		t.Fatal("followed preplanted directory link")
	}
	if _, err = os.Stat(filepath.Join(outside, "escape")); !os.IsNotExist(err) {
		t.Fatal("wrote outside destination")
	}
}

func TestMaterializationFailureAndCancellationCleanOwnedTree(t *testing.T) {
	good := validBytes(t)
	for _, cancelRead := range []bool{false, true} {
		dest := filepath.Join(t.TempDir(), "target")
		ctx, cancel := context.WithCancel(context.Background())
		var input io.Reader = bytes.NewReader(good)
		if cancelRead {
			input = cancelAtEOFReader{input, cancel}
		} else {
			input = bytes.NewReader(bytes.Replace(good, []byte("hello"), []byte("HELLO"), 1))
		}
		r, err := materialize(ctx, input, dest, hashBytes(good))
		cancel()
		if err == nil || r.Destination != "" {
			t.Fatal("failed extraction accepted")
		}
		if _, err = os.Stat(dest); !os.IsNotExist(err) {
			t.Fatal("partial destination retained")
		}
	}
	staging := t.TempDir()
	t.Setenv("TMPDIR", staging)
	t.Setenv("TMP", staging)
	ctx, cancel := context.WithCancel(context.Background())
	dest := filepath.Join(t.TempDir(), "target")
	_, err := extractFrom(ctx, cancelAtEOFReader{bytes.NewReader(good), cancel}, dest, hashBytes(good))
	wantCode(t, err, "cancelled")
	entries, _ := os.ReadDir(staging)
	if len(entries) != 0 {
		t.Fatal("private archive retained on cancellation")
	}
}

func TestConcurrentExtractionHasOneOwner(t *testing.T) {
	b := validBytes(t)
	input := archiveFile(t, b)
	dest := filepath.Join(t.TempDir(), "target")
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := Extract(context.Background(), input, dest, hashBytes(b))
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else {
			wantCode(t, err, "destination_exists")
		}
	}
	if success != 1 {
		t.Fatalf("owners %d", success)
	}
	if _, err := os.Stat(filepath.Join(dest, "App/Main.swift")); err != nil {
		t.Fatal("loser removed winning output")
	}
}
