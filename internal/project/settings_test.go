package project

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestOpenSettingsRestoresSwiftUIProject(t *testing.T) {
	brief := strings.Repeat("Keep a list of trail gear.\n", 3500) + "# Final note"
	created, err := CreateWithOptions(t.TempDir(), "Trip", "org.example.trips", StarterOptions{Brief: brief, Starter: "auto"})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := OpenSettings(created.Path)
	if err != nil {
		t.Fatal(err)
	}
	if settings := opened.Settings; settings.Name != "Trip" || settings.Format != "swiftui" ||
		settings.BundleID != "org.example.trips" || settings.Starter != "auto" || settings.Brief != brief ||
		!slices.Equal(settings.Platforms, []string{"ios"}) {
		t.Fatal("reopening did not restore the original SwiftUI form")
	}
	body, err := os.ReadFile(filepath.Join(created.Root, settingsFilename))
	if err != nil || strings.Contains(string(body), "Keep a list") {
		t.Fatal("the saved settings unnecessarily repeated the full brief")
	}
}

func TestOpenSettingsRestoresLegacyFlutterProject(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Classic")
	for _, dir := range []string{"lib", "web", "windows"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	const brief = "A list app for the web.\n\n## More details\n\nSave notes."
	for file, content := range map[string]string{
		"pubspec.yaml": "name: apprivet_classic", "lib/main.dart": "void main() {}",
		"APP_BRIEF.md": "# App brief: Classic\n\nPlatforms: web, windows\n\n" + brief + "\n",
	} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(file)), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	opened, err := OpenSettings(root)
	if err != nil {
		t.Fatal(err)
	}
	if settings := opened.Settings; settings.Name != "Classic" || settings.Format != "flutter" ||
		settings.Brief != brief || !slices.Equal(settings.Platforms, []string{"web", "windows"}) {
		t.Fatalf("incorrect legacy settings: %+v", settings)
	}
	if _, err := os.Stat(filepath.Join(root, settingsFilename)); !os.IsNotExist(err) {
		t.Fatal("reopening modified an existing project")
	}
}

func TestOpenSettingsRejectsSymlinkedBrief(t *testing.T) {
	parent := t.TempDir()
	created, err := Create(parent, "Linked", "org.example.linked")
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "outside.txt")
	if err := os.WriteFile(outside, []byte("Unrelated private file"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(created.Root, "APP_BRIEF.md")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := OpenSettings(created.Root); err == nil {
		t.Fatal("opened a brief linked outside its project folder")
	}
}
