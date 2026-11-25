package generation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const watchPlanJSON = `{"architecture":"ContentView renders the WatchMovement view.", "components":[{"title":"Workbench","scope":"Display the WatchMovement view."},{"title":"Movement","scope":"Define a WatchMovement SwiftUI view."}]}`

func TestComponentPlanBindsEveryCodeRequest(t *testing.T) {
	input := PlanRequest{Name: "Watchmaker", Format: "swiftui", Brief: "Teach assembly of a mechanical watch.",
		Provider: "anthropic", Model: "claude-opus-5-5", APIKey: "fake-key-for-test"}
	preparation, err := ReviewPlanning(input)
	if err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	planner := func(_ context.Context, provider, model, key, prompt string, budget int) (string, error) {
		if provider != input.Provider || model != input.Model || key != input.APIKey ||
			prompt != preparation.Prompts[0].Prompt || budget != preparation.Prompts[0].MaxOutputTokens {
			t.Fatal("sent a planning request that was absent from the review")
		}
		return watchPlanJSON, nil
	}
	input.Approval = preparation.Approval
	review, err := DraftComponents(t.Context(), input, planner)
	if err != nil {
		t.Fatal(err)
	}
	input.Plan = review.Plan
	if review.Approval == input.Approval || review.RequestCount != 2 || review.MaxOutputTokens != 2*componentTokens {
		t.Fatal("planning approval was reused or the reviewed budget is wrong")
	}
	var sent []PlanPrompt
	providerCall := func(_ context.Context, _, _, _, prompt string, budget int) (string, error) {
		i := len(sent)
		sent = append(sent, PlanPrompt{Title: review.Prompts[i].Title, File: review.Prompts[i].File,
			Prompt: prompt, MaxOutputTokens: budget})
		if i == 0 {
			return "import SwiftUI\nstruct ContentView: View { var body: some View { WatchMovement() } }", nil
		}
		return "import SwiftUI\nstruct WatchMovement: View { var body: some View { Text(\"Movement\") } }", nil
	}
	if _, err := CreatePlanned(t.Context(), parent, input, providerCall); err == nil || len(sent) != 0 {
		t.Fatal("the planning approval incorrectly authorized code generation")
	}
	input.Approval = review.Approval
	changed := input
	changed.Brief += " Add a second watch."
	if _, err := CreatePlanned(t.Context(), parent, changed, providerCall); err == nil || len(sent) != 0 {
		t.Fatal("a changed brief reused the approval")
	}
	tampered := *input.Plan
	tampered.Components = slices.Clone(tampered.Components)
	tampered.Components[1].Scope = "Different implementation"
	changed = input
	changed.Plan = &tampered
	if _, err := CreatePlanned(t.Context(), parent, changed, providerCall); err == nil || len(sent) != 0 {
		t.Fatal("a changed component reused the approval")
	}
	info, err := CreatePlanned(t.Context(), parent, input, providerCall)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(sent, review.Prompts) {
		t.Fatal("the actual requests differed from the approved prompts and budgets")
	}
	for _, step := range review.Prompts {
		if _, err := os.Stat(filepath.Join(info.Root, input.Name, step.File)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLongBriefPlanningBoundsInputAndAccountsForEveryCall(t *testing.T) {
	input := PlanRequest{Name: "Watchmaker", Format: "swiftui",
		Brief:    strings.Repeat("Timepiece measurements 世界 and assembly notes.\n\n", 1800),
		Provider: "anthropic", Model: "claude-opus-5-5", APIKey: "fake-key-for-test"}
	chunks := splitBrief(input.Brief)
	if len(chunks) <= summaryFanIn || strings.Join(chunks, "") != input.Brief {
		t.Fatal("the brief was dropped or the test failed to exercise the synthesis cascade")
	}
	preparation, err := ReviewPlanning(input)
	if err != nil || !preparation.UsesSynthesis || len(preparation.Prompts) != len(chunks) {
		t.Fatalf("invalid planning review: %v", err)
	}
	calls, tokens := 0, 0
	generator := func(_ context.Context, _, _, _, prompt string, budget int) (string, error) {
		if len(prompt) > briefChunkBytes+1800 {
			t.Fatalf("oversized request: %d bytes", len(prompt))
		}
		if calls < len(preparation.Prompts) && prompt != preparation.Prompts[calls].Prompt {
			t.Fatal("sent unreviewed input from the original brief")
		}
		calls++
		tokens += budget
		if budget == planTokens {
			return watchPlanJSON, nil
		}
		return "Keep the assembly order and timepiece measurements.", nil
	}
	if _, err := DraftComponents(t.Context(), input, generator); err == nil || calls != 0 {
		t.Fatal("unapproved brief was sent to the provider")
	}
	input.Approval = preparation.Approval
	if _, err := DraftComponents(t.Context(), input, generator); err != nil ||
		calls != preparation.RequestCount || tokens != preparation.MaxOutputTokens {
		t.Fatalf("planned %d requests and %d tokens; used %d and %d: %v",
			preparation.RequestCount, preparation.MaxOutputTokens, calls, tokens, err)
	}
}

func TestComponentFailureLeavesNoProject(t *testing.T) {
	input := PlanRequest{Name: "Watchmaker", Format: "swiftui", Brief: "Build a watch.",
		Provider: "anthropic", Model: "claude-opus-5-5", APIKey: "fake-key-for-test"}
	preparation, err := ReviewPlanning(input)
	if err != nil {
		t.Fatal(err)
	}
	input.Approval = preparation.Approval
	review, err := DraftComponents(t.Context(), input, func(context.Context, string, string, string, string, int) (string, error) {
		return watchPlanJSON, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	input.Plan, input.Approval = review.Plan, review.Approval
	parent := t.TempDir()
	count := 0
	_, err = CreatePlanned(t.Context(), parent, input, func(context.Context, string, string, string, string, int) (string, error) {
		count++
		if count == 1 {
			return "import SwiftUI\nstruct ContentView: View { var body: some View { WatchMovement() } }", nil
		}
		return "", errors.New("output limit")
	})
	if err == nil || !strings.Contains(err.Error(), "component 2 of 2 (Movement)") || count != 2 {
		t.Fatalf("did not identify the failed component: %v", err)
	}
	if _, err := os.Stat(filepath.Join(parent, input.Name)); !os.IsNotExist(err) {
		t.Fatal("an incomplete plan created a project")
	}
}
