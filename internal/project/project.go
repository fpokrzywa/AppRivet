// Package project creates original Xcode projects and discovers project markers
// without executing user code or evaluating Swift package manifests.
package project

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

//go:embed templates/*
var templates embed.FS

type Info struct {
	Kind    string `json:"kind"`
	Path    string `json:"path"`
	Root    string `json:"root"`
	Scheme  string `json:"suggested_scheme,omitempty"`
	Starter string `json:"starter,omitempty"`
}

var namePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{0,49}$`)
var bundlePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*(\.[A-Za-z][A-Za-z0-9-]*)+$`)
var reserved = regexp.MustCompile(`(?i)^(CON|PRN|AUX|NUL|COM[0-9]|LPT[0-9])$`)

func Create(parent, name, bundleID string) (Info, error) {
	return CreateWithOptions(parent, name, bundleID, StarterOptions{})
}

type Draft struct {
	Name     string `json:"name"`
	BundleID string `json:"bundle_id"`
	Starter  string `json:"starter"`
	Brief    string `json:"brief"`
	Prompt   string `json:"prompt"`
}

// Review validates the same inputs used by project creation without writing files.
func Review(name, bundleID string, options StarterOptions) (Draft, error) {
	if !namePattern.MatchString(name) || reserved.MatchString(name) {
		return Draft{}, fmt.Errorf("project name must be 1-50 ASCII letters/digits, start with a letter, and not be a reserved Windows name")
	}
	if len(bundleID) > 255 || !bundlePattern.MatchString(bundleID) {
		return Draft{}, fmt.Errorf("bundle ID must use reverse-DNS labels starting with letters, such as com.example.myapp (maximum 255 characters)")
	}
	for _, label := range strings.Split(bundleID, ".") {
		if strings.HasSuffix(label, "-") || len(label) > 63 {
			return Draft{}, fmt.Errorf("bundle ID labels must be at most 63 characters and cannot end with a hyphen")
		}
	}
	starter, brief, err := resolveStarter(options)
	if err != nil {
		return Draft{}, err
	}
	prompt := fmt.Sprintf("Create an iOS 17+ SwiftUI app with these project settings.\n\nProject name: %s\nBundle ID: %s", name, bundleID)
	if brief == "" {
		prompt += "\n\nStart with a basic SwiftUI screen."
	} else {
		prompt += "\n\nApp brief:\n" + brief
	}
	return Draft{Name: name, BundleID: bundleID, Starter: starter, Brief: brief, Prompt: prompt}, nil
}

func CreateWithOptions(parent, name, bundleID string, options StarterOptions) (Info, error) {
	draft, err := Review(name, bundleID, options)
	if err != nil {
		return Info{}, err
	}
	return create(parent, draft, "", options.Starter != "" || draft.Brief != "")
}

func ReviewAI(name, bundleID, brief string) (Draft, error) {
	draft, err := Review(name, bundleID, StarterOptions{Brief: brief, Starter: "blank"})
	if err != nil {
		return Draft{}, err
	}
	if draft.Brief == "" {
		return Draft{}, fmt.Errorf("describe the app or import a brief before generating with AI")
	}
	draft.Starter = "ai"
	draft.Prompt = fmt.Sprintf(`Generate a working SwiftUI app for iPhone and iPad, targeting iOS 17 or later.
Return JSON with a single "content_view" string containing the complete ContentView.swift source.
Start with import SwiftUI and define struct ContentView: View with a default initializer.
Use Apple frameworks and self-contained source. The surrounding Xcode project, app entry point,
Greeting helper, and smoke tests already exist. Omit @main, package dependencies, entitlements, build settings,
commands, script files, and setup instructions. Implement the brief as fully as possible within one
Swift source file. Use local data and avoid credentials or external services.

Project name: %s
Bundle ID: %s

App brief:
%s`, draft.Name, draft.BundleID, draft.Brief)
	return draft, nil
}

// CreateGenerated writes generated text into a fixed source file. Xcode and the
// user must review and build the source separately.
func CreateGenerated(parent string, draft Draft, source string) (Info, error) {
	validated, err := ReviewAI(draft.Name, draft.BundleID, draft.Brief)
	if err != nil {
		return Info{}, err
	}
	return create(parent, validated, source, true)
}

func create(parent string, draft Draft, swiftSource string, includeStarter bool) (Info, error) {
	name := draft.Name
	root, err := filepath.Abs(filepath.Join(parent, name))
	if err != nil {
		return Info{}, err
	}
	files := []struct{ src, dst string }{
		{"App.swift.tmpl", name + "/App.swift"},
		{"ContentView.swift.tmpl", name + "/ContentView.swift"},
		{"Tests.swift.tmpl", name + "Tests/" + name + "Tests.swift"},
		{"project.pbxproj.tmpl", name + ".xcodeproj/project.pbxproj"},
		{"scheme.xml.tmpl", name + ".xcodeproj/xcshareddata/xcschemes/" + name + ".xcscheme"},
		{"gitignore.tmpl", ".gitignore"},
		{"README.md.tmpl", "README.md"},
	}
	if draft.Brief != "" {
		files = append(files, struct{ src, dst string }{"APP_BRIEF.md.tmpl", "APP_BRIEF.md"})
	}
	// Render everything before writing. The destination is exclusively created;
	// failures preserve any partial project for inspection rather than deleting it.
	rendered := make([][]byte, len(files))
	for i, file := range files {
		if swiftSource != "" && file.src == "ContentView.swift.tmpl" {
			rendered[i] = []byte(swiftSource + "\n\nenum Greeting {\n    static func message(for name: String) -> String {\n        \"Hello, \\(name)!\"\n    }\n}\n")
			continue
		}
		raw, err := templates.ReadFile("templates/" + file.src)
		if err != nil {
			return Info{}, err
		}
		tmpl, err := template.New(file.src).Option("missingkey=error").Parse(string(raw))
		if err != nil {
			return Info{}, err
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, draft); err != nil {
			return Info{}, err
		}
		rendered[i] = buf.Bytes()
	}
	if err := os.Mkdir(root, 0755); err != nil {
		return Info{}, fmt.Errorf("create project destination (must not already exist): %w", err)
	}
	for i, file := range files {
		dest := filepath.Join(root, filepath.FromSlash(file.dst))
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return Info{}, fmt.Errorf("partial project remains at %s: %w", root, err)
		}
		f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return Info{}, fmt.Errorf("partial project remains at %s: %w", root, err)
		}
		_, writeErr := f.Write(rendered[i])
		closeErr := f.Close()
		if writeErr != nil {
			return Info{}, writeErr
		}
		if closeErr != nil {
			return Info{}, closeErr
		}
	}
	info := Info{Kind: "xcode-project", Path: filepath.Join(root, name+".xcodeproj"), Root: root, Scheme: name}
	if includeStarter {
		info.Starter = draft.Starter
	}
	return info, nil
}

func Detect(path string) (Info, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return Info{}, err
	}
	stat, err := os.Lstat(absolute)
	if err != nil {
		return Info{}, fmt.Errorf("inspect project: %w", err)
	}
	if stat.Mode()&os.ModeSymlink != 0 {
		return Info{}, fmt.Errorf("project path cannot be a symlink")
	}
	if !stat.IsDir() {
		return Info{}, fmt.Errorf("project path must be a directory, .xcodeproj, or .xcworkspace")
	}
	if kind := xcodeKind(absolute); kind != "" {
		return xcodeInfo(absolute, kind)
	}
	entries, err := os.ReadDir(absolute)
	if err != nil {
		return Info{}, err
	}
	var projects, workspaces []string
	for _, entry := range entries {
		kind := xcodeKind(entry.Name())
		if kind == "" {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return Info{}, fmt.Errorf("project marker %s is a symlink; select a real project explicitly", entry.Name())
		}
		if !entry.IsDir() {
			continue
		}
		if kind == "xcode-workspace" {
			workspaces = append(workspaces, entry.Name())
		} else {
			projects = append(projects, entry.Name())
		}
	}
	candidates := projects
	if len(workspaces) != 0 {
		candidates = workspaces
	}
	if len(candidates) > 1 {
		return Info{}, fmt.Errorf("multiple projects or workspaces found; use --project with one explicit path")
	}
	if len(candidates) == 1 {
		p := filepath.Join(absolute, candidates[0])
		return xcodeInfo(p, xcodeKind(p))
	}
	pkg := filepath.Join(absolute, "Package.swift")
	if stat, err := os.Lstat(pkg); err == nil {
		if !stat.Mode().IsRegular() {
			return Info{}, fmt.Errorf("Package.swift must be a regular file, not a symlink")
		}
		return Info{Kind: "swift-package", Path: pkg, Root: absolute}, nil
	} else if !os.IsNotExist(err) {
		return Info{}, err
	}
	return Info{}, fmt.Errorf("no .xcodeproj, .xcworkspace, or Package.swift found at project root")
}

func xcodeKind(path string) string {
	switch filepath.Ext(path) {
	case ".xcodeproj":
		return "xcode-project"
	case ".xcworkspace":
		return "xcode-workspace"
	}
	return ""
}

func xcodeInfo(path, kind string) (Info, error) {
	marker := "project.pbxproj"
	if kind == "xcode-workspace" {
		marker = "contents.xcworkspacedata"
	}
	stat, err := os.Lstat(filepath.Join(path, marker))
	if err != nil {
		return Info{}, fmt.Errorf("missing %s: %w", marker, err)
	}
	if !stat.Mode().IsRegular() || stat.Mode()&fs.ModeSymlink != 0 {
		return Info{}, fmt.Errorf("%s must be a regular file", marker)
	}
	return Info{Kind: kind, Path: path, Root: filepath.Dir(path), Scheme: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))}, nil
}
