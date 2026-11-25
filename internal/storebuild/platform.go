package storebuild

import (
	"archive/zip"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/fpokrzywa/AppRivet/internal/project"
	"github.com/fpokrzywa/AppRivet/internal/runner"
)

var releaseSchemePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,127}$`)

func (b builder) archiveIOS(ctx context.Context, info project.Info, team, work, destination string) (Result, error) {
	projectDir := info.Root
	if info.Kind == "flutter-project" {
		if _, err := b.flutter(ctx, info.Root, "ios"); err != nil {
			return Result{}, err
		}
		var err error
		info, err = project.Detect(filepath.Join(projectDir, "ios", "Runner.xcworkspace"))
		if err != nil {
			return Result{}, errors.New("Flutter did not prepare the iOS Runner workspace")
		}
	}
	if !releaseSchemePattern.MatchString(info.Scheme) {
		return Result{}, errors.New("use a shared iOS scheme with a valid name in Xcode")
	}
	selector := "-project"
	if info.Kind == "xcode-workspace" {
		selector = "-workspace"
	}
	archive := filepath.Join(work, "App.xcarchive")
	export := filepath.Join(work, "export")
	options := filepath.Join(work, "ExportOptions.plist")
	if err := os.WriteFile(options, []byte(fmt.Sprintf(exportOptions, team)), 0600); err != nil {
		return Result{}, err
	}
	const xcodebuild = "/usr/bin/xcodebuild"
	archiveArgs := []string{selector, info.Path, "-scheme", info.Scheme,
		"-configuration", "Release", "-sdk", "iphoneos", "-destination", "generic/platform=iOS", "-destination-timeout", "60",
		"-derivedDataPath", filepath.Join(work, "DerivedData"), "-clonedSourcePackagesDirPath", filepath.Join(work, "SourcePackages"),
		"-archivePath", archive, "DEVELOPMENT_TEAM=" + team, "archive"}
	if _, err := b.run.Run(ctx, runner.Spec{Path: xcodebuild, Args: archiveArgs, Dir: info.Root, Timeout: 20 * time.Minute}); err != nil {
		return Result{}, stageError(err, "Xcode could not archive the app. Check the shared scheme and the team's local signing configuration")
	}
	exportArgs := []string{"-exportArchive", "-archivePath", archive, "-exportOptionsPlist", options, "-exportPath", export}
	if _, err := b.run.Run(ctx, runner.Spec{Path: xcodebuild, Args: exportArgs, Dir: work, Timeout: 10 * time.Minute}); err != nil {
		return Result{}, stageError(err, "Xcode could not export the App Store Connect IPA. Check the Apple Distribution certificate and App Store provisioning profile in Xcode")
	}
	root, err := os.OpenRoot(work)
	if err != nil {
		return Result{}, err
	}
	defer root.Close()
	file, err := singleArtifact(root, "export", ".ipa")
	if err != nil {
		return Result{}, err
	}
	result, err := copyArtifact(root, file, destination)
	if err != nil {
		return Result{}, err
	}
	if err := inspectIPA(destination); err != nil {
		return Result{}, err
	}
	return result, nil
}

const exportOptions = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>method</key><string>app-store-connect</string>
<key>destination</key><string>export</string>
<key>teamID</key><string>%s</string>
<key>manageAppVersionAndBuildNumber</key><false/>
</dict></plist>
`

// Xcode performs the signing checks during export. Also confirm that the IPA
// contains both the metadata and signing resources for the exported app.
func inspectIPA(path string) error {
	invalid := errors.New("Xcode did not export a signed iOS app in the IPA")
	ipa, err := zip.OpenReader(path)
	if err != nil {
		return invalid
	}
	defer ipa.Close()
	apps := map[string]uint8{}
	for _, file := range ipa.File {
		if !file.Mode().IsRegular() || file.UncompressedSize64 == 0 {
			continue
		}
		name, ok := strings.CutPrefix(file.Name, "Payload/")
		if !ok {
			continue
		}
		app, component, ok := strings.Cut(name, "/")
		if !ok || !strings.HasSuffix(app, ".app") {
			continue
		}
		switch component {
		case "Info.plist":
			apps[app] |= 1
		case "_CodeSignature/CodeResources":
			apps[app] |= 2
		}
	}
	for _, fields := range apps {
		if len(apps) == 1 && fields == 3 {
			return nil
		}
	}
	return invalid
}

//go:embed VerifyBundle.java
var verifyBundleSource []byte

var androidBuildGate = make(chan struct{}, 1)

func (b builder) buildAndroid(ctx context.Context, rootPath, java, fingerprint, work, destination string) (Result, error) {
	select {
	case androidBuildGate <- struct{}{}:
		defer func() { <-androidBuildGate }()
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
	file, err := b.flutter(ctx, rootPath, "android")
	if err != nil {
		return Result{}, err
	}
	if !filepath.IsLocal(file) || filepath.Ext(file) != ".aab" {
		return Result{}, errors.New("Flutter did not report a release AAB inside the project")
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return Result{}, err
	}
	defer root.Close()
	if err := requireArtifactDir(root, filepath.Dir(file), ".aab"); err != nil {
		return Result{}, err
	}
	result, err := copyArtifact(root, file, destination)
	if err != nil {
		return Result{}, err
	}
	verifier := filepath.Join(work, "VerifyBundle.java")
	if err := os.WriteFile(verifier, verifyBundleSource, 0600); err != nil {
		return Result{}, err
	}
	if _, err := b.run.Run(ctx, runner.Spec{Path: java, Args: []string{verifier, destination, fingerprint}, Dir: work, Timeout: 3 * time.Minute}); err != nil {
		return Result{}, stageError(err, "could not verify the AAB against the Android upload certificate. Check the release signing configuration, SHA-256 fingerprint, and Java 17+ JDK")
	}
	return result, nil
}

func stageError(err error, message string) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%s: %w", message, err)
	}
	return errors.New(message)
}

func singleArtifact(root *os.Root, dir, extension string) (string, error) {
	if err := requireArtifactDir(root, dir, extension); err != nil {
		return "", err
	}
	entries, err := readDirectory(root, dir)
	if err != nil {
		return "", err
	}
	var name string
	for _, entry := range entries {
		if !strings.EqualFold(filepath.Ext(entry.Name()), extension) {
			continue
		}
		if name != "" || !entry.Type().IsRegular() {
			return "", fmt.Errorf("expected one regular %s artifact in the release directory", extension)
		}
		name = filepath.Join(dir, entry.Name())
	}
	if name == "" {
		return "", fmt.Errorf("the build tool did not produce a %s release artifact", extension)
	}
	return name, nil
}

func requireArtifactDir(root *os.Root, dir, extension string) error {
	for path := dir; path != "."; path = filepath.Dir(path) {
		info, err := root.Lstat(path)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("the build tool did not produce a release directory for the %s artifact", extension)
		}
	}
	return nil
}

func findJava() string {
	java := "java"
	var jbr string
	switch runtime.GOOS {
	case "windows":
		java += ".exe"
		jbr = filepath.Join(os.Getenv("ProgramFiles"), "Android", "Android Studio", "jbr")
	case "darwin":
		jbr = "/Applications/Android Studio.app/Contents/jbr/Contents/Home"
	case "linux":
		jbr = "/opt/android-studio/jbr"
	}
	for _, home := range []string{os.Getenv("JAVA_HOME"), jbr} {
		if !filepath.IsAbs(home) {
			continue
		}
		path := filepath.Join(home, "bin", java)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && (runtime.GOOS == "windows" || info.Mode().Perm()&0111 != 0) {
			return path
		}
	}
	if path, err := exec.LookPath(java); err == nil && filepath.IsAbs(path) {
		return path
	}
	return ""
}
