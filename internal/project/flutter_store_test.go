package project

import (
	"path/filepath"
	"testing"
)

func TestFlutterAppBundlePathUsesReportedBuildOutput(t *testing.T) {
	path := filepath.Join("custom output", "app", "outputs", "bundle", "release", "app-release.aab")
	output := "Running Gradle task 'bundleRelease'...\r\n✓ Built " + path + " (20.8MB)\r\n"
	if got, err := flutterAppBundlePath(output); err != nil || got != path {
		t.Fatalf("got %q %v, want %q", got, err, path)
	}
	for _, invalid := range []string{"Build success", "✓ Built ../app.aab (1.2MB)", "✓ Built /tmp/app.aab (1.2MB)", "✓ Built app.aab.exe"} {
		if got, err := flutterAppBundlePath(invalid); err == nil {
			t.Fatalf("unexpected path %q for %q", got, invalid)
		}
	}
}
