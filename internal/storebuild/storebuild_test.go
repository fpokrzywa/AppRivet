package storebuild

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/fpokrzywa/AppRivet/internal/runner"
)

type testRunner func(context.Context, runner.Spec) (runner.Result, error)

func (f testRunner) Run(ctx context.Context, spec runner.Spec) (runner.Result, error) {
	return f(ctx, spec)
}

func writeFixture(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func flutterFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "pubspec.yaml"), "name: fixture")
	writeFixture(t, filepath.Join(root, "lib", "main.dart"), "void main() {}")
	writeFixture(t, filepath.Join(root, "android", "settings.gradle"), "")
	writeFixture(t, filepath.Join(root, "ios", "Runner.xcworkspace", "contents.xcworkspacedata"), "")
	return root
}

func privateFixture(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	private := filepath.Join(root, "releases")
	if err := os.Mkdir(private, 0700); err != nil {
		t.Fatal(err)
	}
	return private
}

func zipFixture(t *testing.T, path string, names ...string) {
	t.Helper()
	writeFixture(t, path, "")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	for _, name := range names {
		entry, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(entry, "fixture"); err != nil {
			t.Fatal(err)
		}
	}
	if err := errors.Join(z.Close(), f.Close()); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseRejectsInvalidRequestsBeforeRunningTools(t *testing.T) {
	app := flutterFixture(t)
	b := builder{host: "windows", releaseRoot: t.TempDir()}
	for _, tc := range []struct {
		request Request
		message string
	}{
		{Request{Project: app, Platform: "android"}, "trust"},
		{Request{Project: app, Platform: "ios", Trusted: true}, "macOS"},
		{Request{Project: app, Platform: "android", UploadCertificateSHA256: "not-a-digest", Trusted: true}, "fingerprint"},
		{Request{Project: app, Platform: "web", Trusted: true}, "platform"},
		{Request{Platform: "android", Trusted: true}, "project"},
	} {
		if _, err := b.build(t.Context(), tc.request); err == nil || !strings.Contains(err.Error(), tc.message) {
			t.Fatalf("build(%+v) got %v, want error containing %q", tc.request, err, tc.message)
		}
	}
}

func TestIOSArchiveAndExportUsePrivatePaths(t *testing.T) {
	for _, flutter := range []bool{false, true} {
		app := flutterFixture(t)
		if !flutter {
			app = t.TempDir()
			writeFixture(t, filepath.Join(app, "Catalog.xcodeproj", "project.pbxproj"), "")
		}
		var calls []runner.Spec
		var flutterPrepared bool
		b := builder{host: "darwin", releaseRoot: privateFixture(t), flutter: func(_ context.Context, root, target string) (string, error) {
			flutterPrepared = true
			if root != app || target != "ios" {
				t.Fatalf("unexpected Flutter preparation %s %s", root, target)
			}
			return "", nil
		}}
		b.run = testRunner(func(_ context.Context, spec runner.Spec) (runner.Result, error) {
			calls = append(calls, spec)
			if spec.Path != "/usr/bin/xcodebuild" || slices.Contains(spec.Args, "-allowProvisioningUpdates") {
				t.Fatalf("unexpected Xcode command: %+v", spec)
			}
			if slices.Contains(spec.Args, "-exportArchive") {
				options, err := os.ReadFile(argAfter(t, spec.Args, "-exportOptionsPlist"))
				if err != nil || !strings.Contains(string(options), "<string>app-store-connect</string>") ||
					!strings.Contains(string(options), "<string>export</string>") || !strings.Contains(string(options), "<string>F00BARBAZ1</string>") {
					t.Fatalf("invalid App Store export options: %s %v", options, err)
				}
				zipFixture(t, filepath.Join(argAfter(t, spec.Args, "-exportPath"), "Catalog.ipa"),
					"Payload/Catalog.app/Info.plist", "Payload/Catalog.app/_CodeSignature/CodeResources")
			}
			return runner.Result{}, nil
		})
		got, err := b.build(t.Context(), Request{Project: app, Platform: "ios", TeamID: "F00BARBAZ1", Trusted: true})
		if err != nil || got.Platform != "ios" || len(got.SHA256) != 64 || got.Size == 0 {
			t.Fatalf("build failed: %+v %v", got, err)
		}
		if len(calls) != 2 || flutterPrepared != flutter || argAfter(t, calls[0].Args, "-destination") != "generic/platform=iOS" ||
			!slices.Contains(calls[0].Args, "archive") || !slices.Contains(calls[0].Args, "DEVELOPMENT_TEAM=F00BARBAZ1") {
			t.Fatalf("unexpected commands: %+v", calls)
		}
		for _, path := range []string{got.Artifact, argAfter(t, calls[0].Args, "-archivePath"), argAfter(t, calls[0].Args, "-derivedDataPath")} {
			relative, err := filepath.Rel(b.releaseRoot, path)
			if err != nil || !filepath.IsLocal(relative) {
				t.Fatalf("path outside release root: %s", path)
			}
		}
		listed, err := b.list(app)
		if err != nil || !slices.Equal(listed, []Result{got}) {
			t.Fatalf("unexpected history: %+v %v", listed, err)
		}
	}
}

func TestAndroidPublishesOnlyAfterPinnedSignatureVerification(t *testing.T) {
	app := flutterFixture(t)
	pin := strings.Repeat("AB:", 31) + "AB"
	var verified bool
	b := builder{host: "windows", releaseRoot: privateFixture(t), java: func() string { return "java-fixture" }}
	b.flutter = func(_ context.Context, root, target string) (string, error) {
		if root != app || target != "android" {
			t.Fatalf("unexpected Flutter invocation: %s %s", root, target)
		}
		bundle := filepath.Join("custom output", "app", "outputs", "bundle", "release", "app-release.aab")
		writeFixture(t, filepath.Join(app, "build", "app", "outputs", "bundle", "release", "app-release.aab"), "stale default-path artifact")
		writeFixture(t, filepath.Join(app, bundle), "aab fixture")
		return bundle, nil
	}
	b.run = testRunner(func(_ context.Context, spec runner.Spec) (runner.Result, error) {
		if spec.Path != "java-fixture" || len(spec.Args) != 3 || spec.Args[2] != strings.Repeat("ab", 32) {
			t.Fatalf("unexpected signing verification: %+v", spec)
		}
		if data, err := os.ReadFile(spec.Args[1]); err != nil || string(data) != "aab fixture" || !strings.HasPrefix(spec.Args[1], b.releaseRoot+string(os.PathSeparator)) {
			t.Fatalf("verification did not inspect the private artifact: %s %s %v", spec.Args[1], data, err)
		}
		if !verified {
			return runner.Result{}, errors.New("signature mismatch")
		}
		return runner.Result{}, nil
	})
	req := Request{Project: app, Platform: "android", UploadCertificateSHA256: pin, Trusted: true}
	if result, err := b.build(t.Context(), req); err == nil || result.Artifact != "" {
		t.Fatalf("unverified artifact was accepted: %+v %v", result, err)
	}
	if listed, err := b.list(app); err != nil || len(listed) != 0 {
		t.Fatalf("unverified artifact appeared in history: %+v %v", listed, err)
	}
	verified = true
	got, err := b.build(t.Context(), req)
	if err != nil || filepath.Ext(got.Artifact) != ".aab" {
		t.Fatalf("verified artifact was rejected: %+v %v", got, err)
	}
	listed, err := b.list(app)
	if err != nil || !slices.Equal(listed, []Result{got}) {
		t.Fatalf("unexpected history: %+v %v", listed, err)
	}
}

func TestReleaseStorageRejectsOverlapAndUncommittedArtifacts(t *testing.T) {
	app := flutterFixture(t)
	if _, err := releaseBucket(filepath.Join(app, "releases"), app, true); err == nil {
		t.Fatal("release storage accepted a directory under the project")
	}
	if _, err := os.Stat(filepath.Join(app, "releases")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("release storage changed the project: %v", err)
	}
	b := builder{releaseRoot: privateFixture(t)}
	bucket, err := releaseBucket(b.releaseRoot, app, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, folder := range []string{".pending-fixture", "release-invalid"} {
		writeFixture(t, filepath.Join(bucket, folder, "app.aab"), "uncommitted")
		writeFixture(t, filepath.Join(bucket, folder, "release.json"), `{"platform":"../outside","size":11}`)
	}
	if result, err := b.list(app); err != nil || len(result) != 0 {
		t.Fatalf("unexpected uncommitted releases in history: %+v %v", result, err)
	}
}

func TestSingleArtifactRejectsLinkedArtifactAndDirectory(t *testing.T) {
	path := t.TempDir()
	outside := t.TempDir()
	writeFixture(t, filepath.Join(outside, "app.aab"), "other file")
	if err := os.Symlink(filepath.Join(outside, "app.aab"), filepath.Join(path, "app.aab")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(path, "linked")); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for _, dir := range []string{".", "linked"} {
		if _, err := singleArtifact(root, dir, ".aab"); err == nil {
			t.Fatalf("linked artifact accepted in %s", dir)
		}
	}
}

func argAfter(t *testing.T, args []string, key string) string {
	t.Helper()
	i := slices.Index(args, key)
	if i == -1 || i+1 == len(args) {
		t.Fatalf("missing %s argument in %v", key, args)
	}
	return args[i+1]
}
