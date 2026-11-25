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
