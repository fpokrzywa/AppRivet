package project

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateProject(t *testing.T) {
	root := t.TempDir()
	info, err := Create(root, "HelloWorld", "org.example.hello")
	if err != nil {
		t.Fatal(err)
	}
	if info.Kind != "xcode-project" || info.Scheme != "HelloWorld" {
		t.Fatalf("unexpected project: %+v", info)
	}
	for _, file := range []string{"HelloWorld/App.swift", "HelloWorld/ContentView.swift", "HelloWorldTests/HelloWorldTests.swift", "HelloWorld.xcodeproj/project.pbxproj", "HelloWorld.xcodeproj/xcshareddata/xcschemes/HelloWorld.xcscheme"} {
		body, err := os.ReadFile(filepath.Join(root, "HelloWorld", file))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "{{") {
			t.Fatalf("unexpanded template in %s", file)
		}
	}
	body, _ := os.ReadFile(filepath.Join(info.Path, "xcshareddata/xcschemes/HelloWorld.xcscheme"))
	var scheme struct {
		XMLName xml.Name
		Build   struct {
			Entries []struct {
				Ref struct {
					Name string `xml:"BlueprintName,attr"`
				} `xml:"BuildableReference"`
			} `xml:"BuildActionEntries>BuildActionEntry"`
		} `xml:"BuildAction"`
	}
	if err := xml.Unmarshal(body, &scheme); err != nil {
		t.Fatal(err)
	}
	if scheme.XMLName.Local != "Scheme" || len(scheme.Build.Entries) != 1 || scheme.Build.Entries[0].Ref.Name != "HelloWorld" {
		t.Fatalf("invalid buildable scheme: %s", body)
	}
	detected, err := Detect(filepath.Join(root, "HelloWorld"))
	if err != nil || detected.Path != info.Path {
		t.Fatalf("discovery mismatch: %+v, %v", detected, err)
	}
}

func TestProjectNamesCannotCollideWithSwiftSyntaxOrFrameworks(t *testing.T) {
	for _, name := range []string{"class", "import", "where", "SwiftUI", "XCTest", "Foundation"} {
		t.Run(name, func(t *testing.T) {
			info, err := Create(t.TempDir(), name, "org.example.app")
			if err != nil {
				t.Fatal(err)
			}
			pbx, err := os.ReadFile(filepath.Join(info.Path, "project.pbxproj"))
			if err != nil {
				t.Fatal(err)
			}
			tests, err := os.ReadFile(filepath.Join(info.Root, name+"Tests", name+"Tests.swift"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(pbx), `PRODUCT_MODULE_NAME = "AppRivet_`+name+`";`) != 2 {
				t.Fatal("app configurations lack a safe, explicit module name")
			}
			if !strings.Contains(string(tests), "@testable import AppRivet_"+name+"\n") {
				t.Fatal("test import does not use safe app module name")
			}
		})
	}
}

func TestCreateRejectsUnsafeNamesWithoutWrites(t *testing.T) {
	for _, name := range []string{"", "../Escape", `..\Escape`, "a/b", "a b", "a\"b", "-App", "1App", "CON", "nul", "LPT1", "COM9", "App.", strings.Repeat("A", 51)} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if _, err := Create(root, name, "org.example.app"); err == nil {
				t.Fatalf("accepted %q", name)
			}
			entries, _ := os.ReadDir(root)
			if len(entries) != 0 {
				t.Fatal("invalid input wrote files")
			}
		})
	}
}

func TestCreateRejectsInvalidBundleIDs(t *testing.T) {
	for _, id := range []string{"", "app", "com..app", "com.1app", "com.app;evil", "com.*", "com.app\nEVIL", "com.-app", "com.app-", strings.Repeat("a", 256) + ".app"} {
		if _, err := Create(t.TempDir(), "App", id); err == nil {
			t.Errorf("accepted bundle ID %q", id)
		}
	}
}

func TestCreateDoesNotOverwrite(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "App")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(root, "App", "org.example.app"); err == nil {
		t.Fatal("overwrote existing directory")
	}
	body, err := os.ReadFile(marker)
	if err != nil || string(body) != "keep" {
		t.Fatal("existing data changed")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatal("modified existing project")
	}
}

func TestDetectLayouts(t *testing.T) {
	for _, tc := range []struct {
		name    string
		dirs    []string
		pkg     bool
		kind    string
		wantErr bool
	}{
		{"project", []string{"App.xcodeproj"}, false, "xcode-project", false},
		{"workspace", []string{"App.xcodeproj", "App.xcworkspace"}, true, "xcode-workspace", false},
		{"package", nil, true, "swift-package", false},
		{"ambiguous", []string{"A.xcodeproj", "B.xcodeproj"}, false, "", true},
		{"ambiguous workspaces", []string{"A.xcworkspace", "B.xcworkspace"}, false, "", true},
		{"empty", nil, false, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for _, dir := range tc.dirs {
				makeProjectMarker(t, root, dir)
			}
			if tc.pkg {
				if err := os.WriteFile(filepath.Join(root, "Package.swift"), []byte("THIS IS NOT EXECUTABLE SWIFT"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			info, err := Detect(root)
			if (err != nil) != tc.wantErr {
				t.Fatalf("got %+v %v", info, err)
			}
			if err == nil && info.Kind != tc.kind {
				t.Fatalf("kind = %q", info.Kind)
			}
		})
	}
}

func TestDetectExplicitProjectAndBrokenMarkers(t *testing.T) {
	root := t.TempDir()
	makeProjectMarker(t, root, "A.xcodeproj")
	makeProjectMarker(t, root, "B.xcodeproj")
	if info, err := Detect(filepath.Join(root, "B.xcodeproj")); err != nil || info.Scheme != "B" {
		t.Fatalf("%+v %v", info, err)
	}
	broken := filepath.Join(t.TempDir(), "Broken.xcodeproj")
	if err := os.Mkdir(broken, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := Detect(broken); err == nil {
		t.Fatal("accepted missing project.pbxproj")
	}
}

func TestRejectSymlinkDestinationAndMarker(t *testing.T) {
	root, target := t.TempDir(), t.TempDir()
	if err := os.Symlink(target, filepath.Join(root, "App")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Create(root, "App", "org.example.app"); err == nil {
		t.Fatal("accepted symlink destination")
	}
	if err := os.Symlink(target, filepath.Join(root, "App.xcodeproj")); err != nil {
		t.Fatal(err)
	}
	if _, err := Detect(filepath.Join(root, "App.xcodeproj")); err == nil {
		t.Fatal("accepted symlink project")
	}
}

func makeProjectMarker(t *testing.T, root, name string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	marker := "project.pbxproj"
	if strings.HasSuffix(name, ".xcworkspace") {
		marker = "contents.xcworkspacedata"
	}
	if err := os.WriteFile(filepath.Join(path, marker), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
}
