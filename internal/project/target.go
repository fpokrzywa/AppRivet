package project

import (
	"fmt"
	"slices"
	"strings"
)

var flutterTargets = []string{"ios", "android", "web", "windows", "macos", "linux"}

type TargetOptions struct {
	Format    string
	Platforms []string
}

func ReviewTarget(name, bundleID string, starter StarterOptions, target TargetOptions, ai bool) (Draft, error) {
	switch target.Format {
	case "", "swiftui":
		if len(target.Platforms) != 0 && !slices.Equal(target.Platforms, []string{"ios"}) {
			return Draft{}, fmt.Errorf("SwiftUI projects support the iOS target. Select Flutter for other platforms")
		}
		if ai {
			return ReviewAI(name, bundleID, starter.Brief)
		}
		return Review(name, bundleID, starter)
	case "flutter":
	default:
		return Draft{}, fmt.Errorf("choose Flutter or SwiftUI as the project format")
	}

	starter.Starter = "blank"
	draft, err := Review(name, bundleID, starter)
	if err != nil {
		return Draft{}, err
	}
	if len(target.Platforms) == 0 {
		return Draft{}, fmt.Errorf("check at least one Flutter platform")
	}
	picked := make(map[string]bool, len(target.Platforms))
	for _, platform := range target.Platforms {
		if !slices.Contains(flutterTargets, platform) || picked[platform] {
			return Draft{}, fmt.Errorf("choose each Flutter target once from iOS, Android, web, Windows, macOS and Linux")
		}
		picked[platform] = true
	}
	draft.Format = "flutter"
	for _, platform := range flutterTargets {
		if picked[platform] {
			draft.Platforms = append(draft.Platforms, platform)
		}
	}
	draft.Prompt = fmt.Sprintf("Create a Flutter starter for %s with shared code in lib/main.dart.\n\nPlatforms: %s", name, strings.Join(draft.Platforms, ", "))
	if ai {
		if draft.Brief == "" {
			return Draft{}, fmt.Errorf("describe the app or import a brief before generating with AI")
		}
		draft.Starter = "ai"
		draft.StarterChoice = "ai"
		draft.Prompt = fmt.Sprintf("Build a working Flutter app named %s for these targets: %s.\n\n"+
			"Return JSON with one \"source\" string containing all of lib/main.dart. Start with import 'package:flutter/material.dart'; and define void main(). "+
			"Use Flutter and Dart SDK libraries only. Keep core behavior in the shared file and adapt the layout to narrow and wide screens. "+
			"Use APIs supported on all selected targets. Omit dependencies, build scripts, shell commands, secrets and platform-specific files. "+
			"The Flutter platform scaffolds and pubspec already exist.", name, strings.Join(draft.Platforms, ", "))
	}
	if draft.Brief != "" {
		draft.Prompt += "\n\nApp brief:\n" + draft.Brief
	}
	return draft, nil
}
