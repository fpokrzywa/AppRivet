package project

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/fpokrzywa/AppRivet/internal/runner"
)

const FlutterBuildTimeout = 15 * time.Minute

func FlutterAvailable() bool { return flutterExecutable() != "" }

// CreationFormats reports the formats to present on this host.
func CreationFormats(host string, flutter bool) []string {
	formats := []string{}
	if flutter {
		formats = append(formats, "flutter")
	}
	if host == "darwin" {
		formats = append(formats, "swiftui")
	}
	return formats
}

func flutterExecutable() string {
	file := "flutter"
	if runtime.GOOS == "windows" {
		file += ".bat"
	}
	if sdk := os.Getenv("APPRIVET_FLUTTER_SDK"); sdk != "" {
		if filepath.IsAbs(sdk) {
			p := filepath.Join(sdk, "bin", file)
			if flutterLauncher(p) {
				return p
			}
		}
		return ""
	}
	if p, err := exec.LookPath(file); err == nil && filepath.IsAbs(p) {
		return p
	}
	home, _ := os.UserHomeDir()
	cache, _ := os.UserCacheDir()
	var candidates []string
	if cache != "" {
		candidates = append(candidates, filepath.Join(cache, "AppRivet", "toolchains", "flutter", "bin", file))
	}
	if home != "" {
		candidates = append(candidates, filepath.Join(home, "develop", "flutter", "bin", file),
			filepath.Join(home, "development", "flutter", "bin", file), filepath.Join(home, "flutter", "bin", file))
	}
	if runtime.GOOS == "darwin" {
		candidates = append(candidates, "/opt/homebrew/bin/flutter", "/usr/local/bin/flutter")
	}
	for _, path := range candidates {
		if flutterLauncher(path) {
			return path
		}
	}
	return ""
}

func flutterLauncher(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && (runtime.GOOS == "windows" || info.Mode().Perm()&0111 != 0)
}

func regularFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

func flutterPlatforms(root string) []string {
	lib := filepath.Join(root, "lib")
	library, err := os.Lstat(lib)
	if err != nil || !library.IsDir() || !regularFile(filepath.Join(root, "pubspec.yaml")) || !regularFile(filepath.Join(lib, "main.dart")) {
		return nil
	}
	var platforms []string
	for _, target := range flutterTargets {
		if info, err := os.Lstat(filepath.Join(root, target)); err == nil && info.IsDir() {
			platforms = append(platforms, target)
		}
	}
	return platforms
}

// CreateFlutter asks the installed SDK for native scaffolds. Application code
// is shared in lib/main.dart, regardless of the chosen target set.
func CreateFlutter(ctx context.Context, parent string, draft Draft, source string) (Info, error) {
	return createFlutter(ctx, parent, draft, source, nil)
}

func createFlutter(ctx context.Context, parent string, draft Draft, source string, parts []string) (Info, error) {
	validated, err := ReviewTarget(draft.Name, draft.BundleID, StarterOptions{Brief: draft.Brief},
		TargetOptions{Format: "flutter", Platforms: draft.Platforms}, source != "")
	if err != nil {
		return Info{}, err
	}
	validated.SourceProvider, validated.SourceModel = draft.SourceProvider, draft.SourceModel
	executable := flutterExecutable()
	if executable == "" {
		return Info{}, errors.New("install the Flutter SDK before creating a Flutter project")
	}
	root, err := filepath.Abs(filepath.Join(parent, validated.Name))
	if err != nil {
		return Info{}, err
	}
	if err := os.Mkdir(root, 0755); err != nil {
		return Info{}, fmt.Errorf("create Flutter project destination (must not already exist): %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if _, err := runFlutter(ctx, executable, root, "create", "--empty", "--no-pub",
		"--project-name=apprivet_"+strings.ToLower(validated.Name),
		"--org=com.example", "--platforms="+strings.Join(validated.Platforms, ","), "."); err != nil {
		return Info{}, fmt.Errorf("Flutter setup failed; a partial project may remain at %s: %w", root, err)
	}
	if got := flutterPlatforms(root); !slices.Equal(got, validated.Platforms) {
		return Info{}, fmt.Errorf("the Flutter SDK did not create the requested platforms at %s", root)
	}
	for i, part := range parts {
		name := fmt.Sprintf("apprivet_part_%02d.dart", i+1)
		file, err := os.OpenFile(filepath.Join(root, "lib", name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return Info{}, fmt.Errorf("partial project remains at %s: %w", root, err)
		}
		_, writeErr := file.WriteString(part + "\n")
		if err := errors.Join(writeErr, file.Close()); err != nil {
			return Info{}, fmt.Errorf("partial project remains at %s: %w", root, err)
		}
	}
	if source != "" {
		if err := os.WriteFile(filepath.Join(root, "lib", "main.dart"), []byte(source+"\n"), 0644); err != nil {
			return Info{}, err
		}
	}
	if validated.Brief != "" {
		content := "# App brief: " + validated.Name + "\n\nPlatforms: " + strings.Join(validated.Platforms, ", ") + "\n\n## My idea\n\n" + validated.Brief + "\n"
		if err := os.WriteFile(filepath.Join(root, "APP_BRIEF.md"), []byte(content), 0644); err != nil {
			return Info{}, err
		}
	}
	if err := os.WriteFile(filepath.Join(root, settingsFilename), settingsData(validated), 0644); err != nil {
		return Info{}, err
	}
	return Info{Kind: "flutter-project", Path: root, Root: root, Scheme: validated.Name, Starter: validated.Starter, Platforms: validated.Platforms}, nil
}

type FlutterBuild struct {
	Target   string `json:"target"`
	Artifact string `json:"artifact"`
	Output   string `json:"output,omitempty"`
}

func FlutterBuildTargets(platforms []string, host string) []string {
	result := []string{}
	for _, platform := range platforms {
		switch platform {
		case "android", "web":
			result = append(result, platform)
		case "ios", "macos":
			if host == "darwin" {
				result = append(result, platform)
			}
		case "windows", "linux":
			if host == platform {
				result = append(result, platform)
			}
		}
	}
	return result
}

func BuildFlutter(ctx context.Context, path, target string, trusted bool) (FlutterBuild, error) {
	if !trusted {
		return FlutterBuild{}, errors.New("review the project and confirm you trust its build scripts")
	}
	info, err := Detect(path)
	if err != nil || info.Kind != "flutter-project" {
		return FlutterBuild{}, errors.New("choose a Flutter project folder")
	}
	if !slices.Contains(FlutterBuildTargets(info.Platforms, runtime.GOOS), target) {
		return FlutterBuild{}, errors.New("this Flutter target is outside the project or requires another operating system")
	}
	executable := flutterExecutable()
	if executable == "" {
		return FlutterBuild{}, errors.New("install the Flutter SDK to build this project")
	}
	var args []string
	var artifact string
	switch target {
	case "web":
		args, artifact = []string{"build", "web"}, "build/web"
	case "android":
		args, artifact = []string{"build", "apk", "--debug"}, "build/app/outputs/flutter-apk/app-debug.apk"
	case "ios":
		args, artifact = []string{"build", "ios", "--simulator", "--debug"}, "build/ios/iphonesimulator/Runner.app"
	case "windows", "linux", "macos":
		args, artifact = []string{"build", target, "--debug"}, "build/"+target
	}
	ctx, cancel := context.WithTimeout(ctx, FlutterBuildTimeout)
	defer cancel()
	output, err := runFlutter(ctx, executable, info.Root, args...)
	if err != nil {
		return FlutterBuild{Target: target, Output: output}, err
	}
	product := filepath.Join(info.Root, filepath.FromSlash(artifact))
	if _, err := os.Stat(product); err != nil {
		return FlutterBuild{Target: target, Output: output}, errors.New("Flutter finished without the expected build artifact")
	}
	return FlutterBuild{Target: target, Artifact: product, Output: output}, nil
}

// RunFlutterStore runs the fixed Flutter release build or Xcode preparation
// step and returns the project-relative AAB path on Android. The caller must
// have confirmed project trust and validated the target.
func RunFlutterStore(ctx context.Context, root, target string) (string, error) {
	var args []string
	switch target {
	case "android":
		args = []string{"build", "appbundle", "--release"}
	case "ios":
		if runtime.GOOS != "darwin" {
			return "", errors.New("Flutter iOS preparation requires macOS with full Xcode")
		}
		args = []string{"build", "ios", "--release", "--no-codesign", "--config-only"}
	default:
		return "", errors.New("unsupported Flutter store platform")
	}
	executable := flutterExecutable()
	if executable == "" {
		return "", errors.New("install the Flutter SDK to build this project")
	}
	ctx, cancel := context.WithTimeout(ctx, FlutterBuildTimeout)
	defer cancel()
	output, err := runFlutter(ctx, executable, root, args...)
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("Flutter release command did not finish: %w", ctx.Err())
		}
		if target == "android" {
			return "", errors.New("Flutter could not build the release AAB. Check the Android toolchain and the project's release signing configuration")
		}
		return "", errors.New("Flutter could not prepare the iOS Release configuration. Check the Flutter, CocoaPods, and Xcode setup")
	}
	if target == "android" {
		return flutterAppBundlePath(output)
	}
	return "", nil
}

func flutterAppBundlePath(output string) (string, error) {
	// Flutter reports the successful bundle path at the end of the command.
	// Use that path so a custom build directory cannot pick up an older AAB.
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		_, reported, ok := strings.Cut(strings.TrimSpace(lines[i]), "Built ")
		if !ok {
			continue
		}
		end := strings.LastIndex(reported, ".aab")
		if end == -1 {
			continue
		}
		end += len(".aab")
		if suffix := reported[end:]; suffix != "" && !strings.HasPrefix(suffix, " (") {
			continue
		}
		if path := filepath.FromSlash(reported[:end]); filepath.IsLocal(path) {
			return filepath.Clean(path), nil
		}
		break
	}
	return "", errors.New("Flutter did not report a release AAB inside the project")
}

func runFlutter(ctx context.Context, executable, dir string, args ...string) (string, error) {
	cmd, err := flutterCommand(ctx, executable, args)
	if err != nil {
		return "", err
	}
	cmd.Dir = dir
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(key) {
		case "APPRIVET_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "FLUTTER_SUPPRESS_ANALYTICS", "CI":
			continue
		}
		cmd.Env = append(cmd.Env, entry)
	}
	cmd.Env = append(cmd.Env, "CI=true", "FLUTTER_SUPPRESS_ANALYTICS=true")
	var output flutterOutput
	cmd.Stdout, cmd.Stderr = &output, &output
	runErr := cmd.Run()
	log := runner.Redact(strings.TrimSpace(output.String()))
	if ctx.Err() != nil {
		return log, fmt.Errorf("Flutter command did not finish: %w", ctx.Err())
	}
	if runErr != nil {
		return log, fmt.Errorf("Flutter command failed. Check the Flutter SDK and toolchain for the selected platform.\n%s", log)
	}
	return log, nil
}

type flutterOutput struct{ bytes.Buffer }

func (o *flutterOutput) Write(p []byte) (int, error) {
	const limit = 64 * 1024
	size := len(p)
	if size >= limit {
		o.Reset()
		p = p[size-limit:]
	} else if extra := o.Len() + size - limit; extra > 0 {
		o.Next(extra)
	}
	_, _ = o.Buffer.Write(p)
	return size, nil
}
