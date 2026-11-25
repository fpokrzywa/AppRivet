package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStarterSelectionAndBriefEscaping(t *testing.T) {
	for _, tc := range []struct{ brief, choice, want string }{
		{"A weekend to-do list", "auto", "checklist"},
		{"A garden journal", "auto", "notes"},
		{"A tally for reps", "auto", "counter"},
		{"Play a podcast", "auto", "blank"},
		{"A diary with a checklist", "auto", "blank"},
		{"A diary", "counter", "counter"},
	} {
		starter, brief, err := resolveStarter(StarterOptions{Brief: tc.brief, Starter: tc.choice})
		if err != nil || brief != tc.brief || starter != tc.want {
			t.Fatalf("%q / %q: got %q, %v", tc.brief, tc.choice, starter, err)
		}
	}
	// The brief belongs in the project documentation, separate from its Swift source.
	text := "Checklist \"\\(try! example())\"\n保存"
	root := t.TempDir()
	if _, err := CreateWithOptions(root, "BriefTest", "com.example.brief", StarterOptions{Brief: text}); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(root, "BriefTest", "BriefTest", "ContentView.swift"))
	if err != nil || strings.Contains(string(source), `try! example()`) {
		t.Fatalf("brief entered the Swift source: %v", err)
	}
	saved, err := os.ReadFile(filepath.Join(root, "BriefTest", "APP_BRIEF.md"))
	if err != nil || !strings.Contains(string(saved), text) {
		t.Fatalf("brief was not preserved in the project documentation: %v", err)
	}
}

func TestInvalidBriefLeavesNoProject(t *testing.T) {
	root := t.TempDir()
	for _, options := range []StarterOptions{{Brief: "line\x00"}, {Starter: "unsupported"}} {
		if _, err := CreateWithOptions(root, "Rejected", "com.example.rejected", options); err == nil {
			t.Fatalf("accepted invalid options: %+v", options)
		}
		if _, err := os.Stat(filepath.Join(root, "Rejected")); !os.IsNotExist(err) {
			t.Fatalf("invalid options left a project: %v", err)
		}
	}
}
