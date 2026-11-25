package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const settingsFilename = ".apprivet.json"

type Settings struct {
	Name      string   `json:"name"`
	BundleID  string   `json:"bundle_id,omitempty"`
	Format    string   `json:"format"`
	Platforms []string `json:"platforms"`
	Starter   string   `json:"starter"`
	Provider  string   `json:"provider,omitempty"`
	Model     string   `json:"model,omitempty"`
	Brief     string   `json:"brief,omitempty"`
}

type Opened struct {
	Project  Info     `json:"project"`
	Settings Settings `json:"settings"`
}

type diskSettings struct {
	Version  int      `json:"version"`
	Settings Settings `json:"settings"`
}

func settingsData(draft Draft) []byte {
	settings := Settings{
		Name: draft.Name, Starter: draft.StarterChoice,
		Provider: draft.SourceProvider, Model: draft.SourceModel,
		Format: draft.Format, Platforms: draft.Platforms,
	}
	if settings.Format == "" {
		settings.Format = "swiftui"
		settings.Platforms = []string{"ios"}
		settings.BundleID = draft.BundleID
	}
	// The description lives in APP_BRIEF.md. Avoid duplicating it.
	body, _ := json.MarshalIndent(diskSettings{Version: 1, Settings: settings}, "", "  ")
	return append(body, '\n')
}

var legacyBundle = regexp.MustCompile(`PRODUCT_BUNDLE_IDENTIFIER\s*=\s*"?([A-Za-z0-9.-]+)"?\s*;`)

// OpenSettings reconstructs the project form. Older projects remain openable
// from their platform files, bundle identifier, and saved brief.
func OpenSettings(path string) (Opened, error) {
	info, err := Detect(path)
	if err != nil {
		return Opened{}, err
	}
	settings := Settings{Name: info.Scheme}
	switch info.Kind {
	case "flutter-project":
		settings.Format, settings.Platforms, settings.Starter = "flutter", info.Platforms, "blank"
	case "xcode-project":
		settings.Format, settings.Platforms, settings.Starter = "swiftui", []string{"ios"}, "auto"
	default:
		return Opened{}, fmt.Errorf("choose a Flutter or generated SwiftUI project")
	}
	root, err := os.OpenRoot(info.Root)
	if err != nil {
		return Opened{}, err
	}
	defer root.Close()
	if settings.Format == "swiftui" {
		config, err := readProjectFile(root, filepath.Join(filepath.Base(info.Path), "project.pbxproj"), 4<<20)
		if err != nil {
			return Opened{}, err
		}
		settings.BundleID = "com.example." + strings.ToLower(settings.Name)
		if match := legacyBundle.FindSubmatch(config); len(match) == 2 {
			settings.BundleID = string(match[1])
		}
	}
	brief, err := readProjectFile(root, "APP_BRIEF.md", 0)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Opened{}, err
	}
	payload, legacyChoice := readSavedBrief(string(brief), settings.Starter)
	settings.Brief = payload
	if validChoice(settings.Format, legacyChoice) {
		settings.Starter = legacyChoice
	}
	body, err := readProjectFile(root, settingsFilename, 32<<10)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Opened{}, err
	}
	if len(body) != 0 {
		var saved diskSettings
		if json.Unmarshal(body, &saved) == nil && saved.Version == 1 && saved.Settings.Format == settings.Format {
			if validChoice(settings.Format, saved.Settings.Starter) {
				settings.Starter = saved.Settings.Starter
			}
			if settings.Starter == "ai" && (saved.Settings.Provider == "openai" || saved.Settings.Provider == "anthropic") {
				settings.Provider = saved.Settings.Provider
				settings.Model = saved.Settings.Model
			}
		}
	}
	return Opened{Project: info, Settings: settings}, nil
}

func validChoice(format, choice string) bool {
	if format == "flutter" {
		return choice == "ai" || choice == "blank"
	}
	return slices.Contains([]string{"ai", "auto", "blank", "checklist", "notes", "counter"}, choice)
}

func readSavedBrief(raw, starter string) (string, string) {
	brief := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(raw, "\r\n", "\n"), "\r", "\n"))
	if !strings.HasPrefix(brief, "# App brief: ") {
		return brief, starter
	}
	blocks := strings.SplitN(brief, "\n\n", 3)
	if len(blocks) < 3 {
		return "", starter
	}
	header, payload := blocks[1], blocks[2]
	if choice, ok := strings.CutPrefix(header, "Starter: "); ok && validChoice("swiftui", choice) {
		starter = choice
	}
	return strings.TrimSpace(strings.TrimPrefix(payload, "## My idea\n\n")), starter
}

func readProjectFile(root *os.Root, name string, limit int64) ([]byte, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("the %s file must be a regular file", name)
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var reader io.Reader = file
	if limit > 0 {
		reader = io.LimitReader(file, limit+1)
	}
	body, err := io.ReadAll(reader)
	if err == nil && limit > 0 && int64(len(body)) > limit {
		return nil, fmt.Errorf("the %s settings file is too large", name)
	}
	return body, err
}
