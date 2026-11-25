package project

import (
	"context"
	"fmt"
	"strings"
)

// CreateGeneratedComponents saves generated source in a new project. The first
// source is the main file, and subsequent sources are assigned fixed filenames.
// For Flutter, the caller supplies the matching Dart part directives.
func CreateGeneratedComponents(ctx context.Context, parent string, draft Draft, sources []string) (Info, error) {
	if err := ctx.Err(); err != nil {
		return Info{}, err
	}
	if len(sources) == 0 || len(sources) > 8 {
		return Info{}, fmt.Errorf("expected one to eight generated source files")
	}
	for i, source := range sources {
		if strings.TrimSpace(source) == "" {
			return Info{}, fmt.Errorf("generated source %d is empty", i+1)
		}
	}
	if draft.Format == "flutter" {
		return createFlutter(ctx, parent, draft, sources[0], sources[1:])
	}
	validated, err := ReviewTarget(draft.Name, draft.BundleID, StarterOptions{Brief: draft.Brief},
		TargetOptions{Format: draft.Format, Platforms: draft.Platforms}, true)
	if err != nil {
		return Info{}, err
	}
	validated.SourceProvider, validated.SourceModel = draft.SourceProvider, draft.SourceModel
	return create(parent, validated, sources[0], sources[1:], true)
}
