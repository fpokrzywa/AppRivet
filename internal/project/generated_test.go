package project

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestCreateGeneratedComponentsIncludesSwiftHelpersInAppTarget(t *testing.T) {
	draft := Draft{Name: "ComponentApp", BundleID: "com.example.app", Brief: "Show a dashboard.",
		Format: "swiftui", Platforms: []string{"ios"}, SourceProvider: "openai", SourceModel: "example-model"}
	sources := []string{"import SwiftUI\nstruct ContentView: View { var body: some View { Text(Part1.title) } }"}
	for i := 1; i <= 7; i++ {
		sources = append(sources, fmt.Sprintf("enum Part%d { static let title = \"Dashboard\" }", i))
	}
	info, err := CreateGeneratedComponents(t.Context(), t.TempDir(), draft, sources)
	if err != nil {
		t.Fatal(err)
	}
	read := func(path string) string {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	app := filepath.Join(info.Root, "ComponentApp")
	main := read(filepath.Join(app, "ContentView.swift"))
	if !strings.HasPrefix(main, sources[0]+"\n") || !strings.Contains(main, "enum Greeting {") {
		t.Fatalf("main source and scaffold helper were not preserved: %s", main)
	}
	pbx := read(filepath.Join(info.Path, "project.pbxproj"))
	appGroup := regexp.MustCompile(`(?m)^ *[0-9A-F]{24} = \{isa = PBXGroup; children = \([^\n]+\); path = ComponentApp;`).FindString(pbx)
	appSources := regexp.MustCompile(`(?m)^ *A00000000000000000000020 = \{isa = PBXSourcesBuildPhase;[^\n]+`).FindString(pbx)
	testSources := regexp.MustCompile(`(?m)^ *A00000000000000000000023 = \{isa = PBXSourcesBuildPhase;[^\n]+`).FindString(pbx)
	if appGroup == "" || appSources == "" || testSources == "" {
		t.Fatal("missing scaffold group or source phase")
	}
	if !strings.Contains(pbx, "buildPhases = (A00000000000000000000020,") {
		t.Fatal("missing app target source phase association")
	}
	for i := 1; i <= 7; i++ {
		name := fmt.Sprintf("AppRivetPart%02d.swift", i)
		if body := read(filepath.Join(app, name)); body != sources[i]+"\n" {
			t.Errorf("contents of %s are wrong: %q", name, body)
		}
		fileRef := regexp.MustCompile(`(?m)^ *([0-9A-F]{24}) = \{isa = PBXFileReference; lastKnownFileType = sourcecode.swift; path = ` + regexp.QuoteMeta(name) + `;`).FindStringSubmatch(pbx)
		if len(fileRef) != 2 || !strings.Contains(appGroup, fileRef[1]) {
			t.Fatalf("file reference for %s missing from app group", name)
		}
		buildRef := regexp.MustCompile(`(?m)^ *([0-9A-F]{24}) = \{isa = PBXBuildFile; fileRef = ` + fileRef[1] + `;`).FindStringSubmatch(pbx)
		if len(buildRef) != 2 || !strings.Contains(appSources, buildRef[1]) || strings.Contains(testSources, buildRef[1]) {
			t.Fatalf("build reference for %s must be in the app source phase", name)
		}
	}
	ids := make(map[string]bool)
	for _, object := range regexp.MustCompile(`(?m)^  ([0-9A-F]{24}) = \{`).FindAllStringSubmatch(pbx, -1) {
		id := object[1]
		if ids[id] {
			t.Errorf("duplicate Xcode object ID %s", id)
		}
		ids[id] = true
	}
	opened, err := OpenSettings(info.Root)
	if err != nil || opened.Settings.Starter != "ai" || opened.Settings.Provider != draft.SourceProvider || opened.Settings.Model != draft.SourceModel {
		t.Fatalf("generated settings were not preserved: %+v, %v", opened, err)
	}
}

func TestCreateGeneratedComponentsWritesFlutterParts(t *testing.T) {
	sdk := t.TempDir()
	bin := filepath.Join(sdk, "bin")
	if err := os.Mkdir(bin, 0755); err != nil {
		t.Fatal(err)
	}
	name := "flutter"
	body := "#!/bin/sh\nset -e\nmkdir lib web\nprintf fixture > pubspec.yaml\nprintf fixture > lib/main.dart\n"
	if runtime.GOOS == "windows" {
		name = "flutter.bat"
		body = "@echo off\r\nmkdir lib web\r\necho fixture>pubspec.yaml\r\necho fixture>lib\\main.dart\r\n"
	}
	if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APPRIVET_FLUTTER_SDK", sdk)
	draft := Draft{Name: "ComponentApp", BundleID: "com.example.app", Brief: "Show a dashboard.",
		Format: "flutter", Platforms: []string{"web"}}
	main := "import 'package:flutter/material.dart';\npart 'apprivet_part_01.dart';\npart 'apprivet_part_02.dart';\nvoid main() {}"
	first := "part of 'main.dart';\nclass Dashboard {}"
	second := "part of 'main.dart';\nclass Model {}"
	info, err := CreateGeneratedComponents(t.Context(), t.TempDir(), draft, []string{main, first, second})
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []struct{ name, source string }{
		{"main.dart", main}, {"apprivet_part_01.dart", first}, {"apprivet_part_02.dart", second},
	} {
		body, err := os.ReadFile(filepath.Join(info.Root, "lib", file.name))
		if err != nil || string(body) != file.source+"\n" {
			t.Fatalf("unexpected source in %s: %q, %v", file.name, body, err)
		}
	}
}

func TestCreateGeneratedComponentsRejectsInvalidSourcesBeforeCreatingProject(t *testing.T) {
	draft := Draft{Name: "ComponentApp", BundleID: "com.example.app", Brief: "Show a dashboard."}
	for _, sources := range [][]string{nil, {"main", " \n"}, strings.Fields(strings.Repeat("source ", 9))} {
		parent := t.TempDir()
		if _, err := CreateGeneratedComponents(t.Context(), parent, draft, sources); err == nil {
			t.Errorf("accepted invalid source set %q", sources)
		}
		if _, err := os.Stat(filepath.Join(parent, draft.Name)); !os.IsNotExist(err) {
			t.Errorf("invalid source set created a project: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	parent := t.TempDir()
	if _, err := CreateGeneratedComponents(ctx, parent, draft, []string{"main"}); err == nil {
		t.Fatal("accepted a canceled generation")
	}
	if _, err := os.Stat(filepath.Join(parent, draft.Name)); !os.IsNotExist(err) {
		t.Errorf("canceled save created a project: %v", err)
	}
}
