package project

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestFlutterReviewBindsSelectedPlatforms(t *testing.T) {
	options := StarterOptions{Brief: "A travel planner that works on my laptop and phone."}
	selected := TargetOptions{Format: "flutter", Platforms: []string{"linux", "ios", "web"}}
	draft, err := ReviewTarget("Travel", "com.example.travel", options, selected, true)
	if err != nil || !slices.Equal(draft.Platforms, []string{"ios", "web", "linux"}) {
		t.Fatalf("wrong selected platforms: %v %v", draft.Platforms, err)
	}
	if !strings.Contains(draft.Prompt, "ios, web, linux") || !strings.Contains(draft.Prompt, "lib/main.dart") {
		t.Fatal("prompt does not describe the selected shared-code project")
	}
	for _, invalid := range []TargetOptions{
		{Format: "flutter"}, {Format: "flutter", Platforms: []string{"web", "web"}},
		{Format: "flutter", Platforms: []string{"../android"}}, {Format: "swiftui", Platforms: []string{"android"}},
	} {
		if _, err := ReviewTarget("Travel", "com.example.travel", options, invalid, false); err == nil {
			t.Fatalf("accepted invalid platform options: %+v", invalid)
		}
	}
	if got := FlutterBuildTargets(draft.Platforms, "linux"); !slices.Equal(got, []string{"web", "linux"}) {
		t.Fatalf("unexpected build targets on Linux: %v", got)
	}
	if got := FlutterBuildTargets(draft.Platforms, "darwin"); !slices.Equal(got, []string{"ios", "web"}) {
		t.Fatalf("unexpected build targets on macOS: %v", got)
	}
}

func TestFlutterDetectionAndBuildRefusals(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"lib", "web"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{"pubspec.yaml", "lib/main.dart"} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(file)), []byte("fixture"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	info, err := Detect(root)
	if err != nil || info.Kind != "flutter-project" || !slices.Equal(info.Platforms, []string{"web"}) {
		t.Fatalf("unexpected detection: %+v %v", info, err)
	}
	if _, err := BuildFlutter(context.Background(), root, "web", false); err == nil {
		t.Fatal("build started without trust")
	}
	if _, err := BuildFlutter(context.Background(), root, "android", true); err == nil {
		t.Fatal("build started for a target omitted from the project")
	}
}
