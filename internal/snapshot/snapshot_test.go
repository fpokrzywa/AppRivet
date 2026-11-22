package snapshot

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func source(t testing.TB) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "App"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"App/Main.swift": "print(\"hello\")\n", "README.md": "original source\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestCreateDeterministicVerifiedSnapshot(t *testing.T) {
	root := source(t)
	out := filepath.Join(t.TempDir(), "source.tar")
	r, err := Create(context.Background(), root, []string{"README.md", "App/Main.swift"}, out)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(out)
	h := sha256.Sum256(body)
	if r.SHA256 != hex.EncodeToString(h[:]) || r.ArchiveBytes != int64(len(body)) || len(r.Files) != 2 || r.Files[0].Path != "App/Main.swift" {
		t.Fatalf("bad report: %+v", r)
	}
	if strings.Contains(string(body), root) {
		t.Fatal("host path included")
	}
	second := filepath.Join(t.TempDir(), "again.tar")
	if err := os.Chtimes(filepath.Join(root, "README.md"), time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}
	r2, err := Create(context.Background(), root, []string{"App/Main.swift", "README.md"}, second)
	if err != nil || r2.SHA256 != r.SHA256 {
		t.Fatalf("not deterministic: %+v %v", r2, err)
	}
	verified, err := Inspect(context.Background(), second, r.SHA256)
	if err != nil || verified.SHA256 != r.SHA256 {
		t.Fatalf("verify: %+v %v", verified, err)
	}
	_, err = Inspect(context.Background(), second, strings.Repeat("0", 64))
	wantCode(t, err, "digest_mismatch")
	_, err = Inspect(context.Background(), second, "bad")
	wantCode(t, err, "invalid_arguments")
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	var detail *Error
	if !errors.As(err, &detail) || detail.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
}

func TestSelectionAndPortablePaths(t *testing.T) {
	for _, name := range []string{"", "../escape", "/absolute", "a/../b", "a//b", "./a", `a\b`, "C:/file", "a:b", "a/.", "NUL.txt", "com1", "aux/file", "ends.", " ends", "x/ends ", "bad\nname", "caf\u00e9.swift", "a?b", "a*b", strings.Repeat("x", 81)} {
		t.Run(name, func(t *testing.T) {
			_, err := Create(context.Background(), t.TempDir(), []string{name}, filepath.Join(t.TempDir(), "out.tar"))
			wantCode(t, err, "unsafe_path")
		})
	}
	for _, names := range [][]string{{"a", "A"}, {"A/x", "a/y"}, {"a", "a/b"}, {"a/b", "a"}} {
		_, err := Create(context.Background(), t.TempDir(), names, filepath.Join(t.TempDir(), "out.tar"))
		wantCode(t, err, "unsafe_path")
	}
	for _, name := range []string{".git/config", ".env", "dir/.env.production", "auth.p8", "KEY.PEM", "secret.key", "profile.mobileprovision", ".ssh/id_ed25519", "id_rsa", ".aws/config"} {
		_, err := Create(context.Background(), t.TempDir(), []string{name}, filepath.Join(t.TempDir(), "out.tar"))
		wantCode(t, err, "sensitive_path")
	}
	list := filepath.Join(t.TempDir(), "files.txt")
	if err := os.WriteFile(list, []byte("\xef\xbb\xbfREADME.md\r\n\r\nApp/Main.swift\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	names, err := ReadSelection(list)
	if err != nil || len(names) != 2 || names[0] != "App/Main.swift" {
		t.Fatalf("selection: %v %v", names, err)
	}
	if err := os.WriteFile(list, []byte(strings.Repeat("x", MaxManifestBytes+1)), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = ReadSelection(list)
	wantCode(t, err, "limit_exceeded")
}

func TestCreatePreservesExistingOutputAndCleansFailure(t *testing.T) {
	root := source(t)
	out := filepath.Join(t.TempDir(), "out.tar")
	if err := os.WriteFile(out, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := Create(context.Background(), root, []string{"README.md"}, out)
	wantCode(t, err, "archive_exists")
	b, _ := os.ReadFile(out)
	if string(b) != "keep" {
		t.Fatal("existing output changed")
	}
	out = filepath.Join(t.TempDir(), "out.tar")
	_, err = Create(context.Background(), root, []string{"App/Main.swift", "missing.txt"}, out)
	if err == nil {
		t.Fatal("missing input accepted")
	}
	if _, err = os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("partial output retained")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = Create(ctx, root, []string{"README.md"}, out)
	wantCode(t, err, "cancelled")
	if _, err = os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("cancelled output retained")
	}
	_, err = Verify(ctx, bytes.NewReader(nil), "")
	wantCode(t, err, "cancelled")
}

func TestSourceRejectsSymlinksAndDirectories(t *testing.T) {
	root := source(t)
	_, err := Create(context.Background(), root, []string{"App"}, filepath.Join(t.TempDir(), "out.tar"))
	wantCode(t, err, "unsafe_source")
	if err := os.Symlink(filepath.Join(root, "README.md"), filepath.Join(root, "link")); err != nil {
		t.Skip("symlink privilege unavailable")
	}
	_, err = Create(context.Background(), root, []string{"link"}, filepath.Join(t.TempDir(), "out.tar"))
	wantCode(t, err, "unsafe_source")
	if err := os.Symlink(filepath.Join(root, "App"), filepath.Join(root, "dirlink")); err != nil {
		t.Fatal(err)
	}
	_, err = Create(context.Background(), root, []string{"dirlink/Main.swift"}, filepath.Join(t.TempDir(), "out.tar"))
	wantCode(t, err, "unsafe_source")
}

func validBytes(t testing.TB) []byte {
	t.Helper()
	out := filepath.Join(t.TempDir(), "out.tar")
	if _, err := Create(context.Background(), source(t), []string{"App/Main.swift", "README.md"}, out); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestVerifyRejectsTamperingTruncationAndTrailingBytes(t *testing.T) {
	good := validBytes(t)
	badData := bytes.Replace(good, []byte("hello"), []byte("HELLO"), 1)
	for _, b := range [][]byte{nil, good[:511], good[:len(good)-1], badData, append(append([]byte{}, good...), 0), append(append([]byte{}, good...), good...)} {
		r, err := Verify(context.Background(), bytes.NewReader(b), "")
		if err == nil || len(r.Files) != 0 {
			t.Fatalf("accepted invalid archive: %d %+v %v", len(b), r, err)
		}
	}
}

type archiveItem struct {
	name string
	kind byte
	body []byte
	mode int64
}

func forged(t *testing.T, items []archiveItem) []byte {
	t.Helper()
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	for _, item := range items {
		h := &tar.Header{Name: item.name, Typeflag: item.kind, Size: int64(len(item.body)), Mode: item.mode, Format: tar.FormatUSTAR}
		if item.kind == tar.TypeSymlink || item.kind == tar.TypeLink {
			h.Linkname = "outside"
			h.Size = 0
		}
		if err := w.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if len(item.body) > 0 {
			if _, err := w.Write(item.body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestVerifyRejectsArchiveLinksPathsMetadataAndManifestAmbiguity(t *testing.T) {
	for _, item := range []archiveItem{{"source/../escape", tar.TypeReg, []byte("x"), 0644}, {"source/a", tar.TypeSymlink, nil, 0644}, {"source/a", tar.TypeLink, nil, 0644}, {"source/a", tar.TypeDir, nil, 0755}, {"source/.env", tar.TypeReg, []byte("secret"), 0644}} {
		_, err := Verify(context.Background(), bytes.NewReader(forged(t, []archiveItem{item})), "")
		if err == nil {
			t.Fatalf("accepted %+v", item)
		}
	}
	good := validBytes(t)
	for _, transform := range []func(*tar.Header, []byte) []byte{
		func(h *tar.Header, b []byte) []byte { h.Mode = 0777; return b },
		func(h *tar.Header, b []byte) []byte { h.Uid = 123; return b },
		func(h *tar.Header, b []byte) []byte { h.ModTime = time.Unix(100, 0); return b },
		func(h *tar.Header, b []byte) []byte {
			if h.Name == "manifest.json" {
				return append([]byte(" "), b...)
			}
			return b
		},
		func(h *tar.Header, b []byte) []byte {
			if h.Name == "manifest.json" {
				return bytes.Replace(b, []byte(`"schema_version":1`), []byte(`"schema_version":2,"schema_version":1`), 1)
			}
			return b
		},
	} {
		var mutated bytes.Buffer
		tw := tar.NewWriter(&mutated)
		tr := tar.NewReader(bytes.NewReader(good))
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(tr)
			b = transform(h, b)
			h.Size = int64(len(b))
			if err = tw.WriteHeader(h); err != nil {
				t.Fatal(err)
			}
			if _, err = tw.Write(b); err != nil {
				t.Fatal(err)
			}
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := Verify(context.Background(), bytes.NewReader(mutated.Bytes()), ""); err == nil {
			t.Fatal("accepted altered metadata/manifest")
		}
	}
}

func TestLimits(t *testing.T) {
	root := source(t)
	p := filepath.Join(root, "large.bin")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(MaxFileBytes + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	_, err = Create(context.Background(), root, []string{"large.bin"}, filepath.Join(t.TempDir(), "out.tar"))
	wantCode(t, err, "limit_exceeded")
	_, err = Create(context.Background(), root, nil, filepath.Join(t.TempDir(), "out.tar"))
	wantCode(t, err, "invalid_arguments")
	names := make([]string, MaxFiles+1)
	_, err = Create(context.Background(), root, names, filepath.Join(t.TempDir(), "out.tar"))
	wantCode(t, err, "limit_exceeded")
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	if err = tw.WriteHeader(&tar.Header{Name: "source/large", Size: MaxFileBytes + 1, Typeflag: tar.TypeReg, Mode: 0644, Format: tar.FormatUSTAR}); err != nil {
		t.Fatal(err)
	}
	_, err = Verify(context.Background(), bytes.NewReader(b.Bytes()), "")
	wantCode(t, err, "limit_exceeded")
}

func FuzzVerify(f *testing.F) {
	f.Add(validBytes(f))
	f.Add([]byte(""))
	f.Add(make([]byte, 1024))
	f.Fuzz(func(t *testing.T, b []byte) {
		r, err := Verify(context.Background(), bytes.NewReader(b), "")
		if err != nil {
			if len(r.Files) > 0 {
				t.Fatal("partial report")
			}
			return
		}
		if len(r.Files) == 0 || len(r.Files) > MaxFiles || r.TotalBytes > MaxTotalBytes {
			t.Fatal("invalid accepted report")
		}
		if _, err := json.Marshal(r); err != nil {
			t.Fatal(err)
		}
	})
}

func TestConsoleDeviceNamesAreNotPortable(t *testing.T) {
	for _, name := range []string{"CONIN$", "conout$", "CON .txt"} {
		_, err := Create(context.Background(), t.TempDir(), []string{name}, filepath.Join(t.TempDir(), "out.tar"))
		wantCode(t, err, "unsafe_path")
	}
}

func TestArchiveCaseAndPrefixCollisions(t *testing.T) {
	for _, names := range [][]string{{"source/A", "source/a"}, {"source/A/x", "source/a/y"}, {"source/a", "source/a/b"}} {
		var items []archiveItem
		for _, name := range names {
			items = append(items, archiveItem{name, tar.TypeReg, []byte("x"), 0644})
		}
		_, err := Verify(context.Background(), bytes.NewReader(forged(t, items)), "")
		wantCode(t, err, "unsafe_path")
	}
}

type cancellingReader struct {
	io.Reader
	cancel context.CancelFunc
}

func (r cancellingReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.cancel()
	return n, err
}
func TestCancellationDuringArchiveRead(t *testing.T) {
	b := validBytes(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := Verify(ctx, cancellingReader{bytes.NewReader(b), cancel}, "")
	wantCode(t, err, "cancelled")
}

func TestTotalAndArchiveLimitsAndOutputSelfReference(t *testing.T) {
	root := t.TempDir()
	var names []string
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		f, err := os.Create(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if err = f.Truncate(MaxFileBytes); err != nil {
			t.Fatal(err)
		}
		f.Close()
		names = append(names, name)
	}
	out := filepath.Join(t.TempDir(), "out.tar")
	_, err := Create(context.Background(), root, names, out)
	wantCode(t, err, "limit_exceeded")
	if _, err = os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("over-limit output retained")
	}
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(MaxArchiveBytes + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	_, err = Inspect(context.Background(), out, "")
	wantCode(t, err, "limit_exceeded")
	_, err = Create(context.Background(), root, []string{"new.tar"}, filepath.Join(root, "new.tar"))
	wantCode(t, err, "invalid_arguments")
}
