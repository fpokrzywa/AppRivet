package project

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// StarterOptions describes a locally generated starting point for a project.
// The brief is treated as text and never executed or inserted as Swift code.
type StarterOptions struct {
	Brief   string
	Starter string
}

func resolveStarter(options StarterOptions) (kind, brief string, err error) {
	brief = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(options.Brief, "\r\n", "\n"), "\r", "\n"))
	if !utf8.ValidString(brief) || utf8.RuneCountInString(brief) > 2000 || strings.IndexFunc(brief, func(r rune) bool {
		return unicode.IsControl(r) && r != '\n' && r != '\t'
	}) >= 0 {
		return "", "", fmt.Errorf("describe the app in up to 2,000 characters of plain text")
	}
	switch options.Starter {
	case "", "auto":
		kind = inferStarter(brief)
	case "blank", "checklist", "notes", "counter":
		kind = options.Starter
	default:
		return "", "", fmt.Errorf("choose Automatic, Blank, Checklist, Notes or Counter")
	}
	return kind, brief, nil
}

func inferStarter(brief string) string {
	match := ""
	words := strings.ReplaceAll(strings.ToLower(brief), "to-do", "todo")
	for _, word := range strings.FieldsFunc(words, func(r rune) bool { return !unicode.IsLetter(r) }) {
		var candidate string
		switch word {
		case "checklist", "checklists", "task", "tasks", "todo", "todos":
			candidate = "checklist"
		case "notes", "note", "journal", "diary", "notepad":
			candidate = "notes"
		case "counter", "count", "counting", "tally", "tallies":
			candidate = "counter"
		default:
			continue
		}
		if match != "" && match != candidate {
			return "blank"
		}
		match = candidate
	}
	if match == "" {
		return "blank"
	}
	return match
}
