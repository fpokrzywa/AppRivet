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

func TestPlanRetriesAnthropicOutputLimit(t *testing.T) {
	input := PlanRequest{Name: "Watchmaker", Brief: "Teach the player to build a working watch.",
		Provider: "anthropic", Model: "claude-opus-5-5", APIKey: "fake-key-for-test"}
	review, err := ReviewPlanning(input)
	if err != nil {
		t.Fatal(err)
	}
	input.Approval = review.Approval
	var budgets []int
	result, err := DraftComponents(t.Context(), input, func(_ context.Context, _, _, _ string, step PlanPrompt) (string, error) {
		budgets = append(budgets, step.MaxOutputTokens)
		if step.Prompt != review.Prompts[0].Prompt || step.File != "" {
			t.Fatal("the retry changed the approved prompt")
		}
		if len(budgets) == 1 {
			return decodeSource(strings.NewReader(`{"stop_reason":"max_tokens"}`), "anthropic")
		}
		return watchPlanJSON, nil
	})
	if err != nil || result.Plan == nil || !slices.Equal(budgets, []int{planTokens, planRetryTokens}) ||
		review.RequestCount != len(budgets) || review.PlanRetryOutputTokens != budgets[1] || review.MaxOutputTokens != budgets[0]+budgets[1] {
		t.Fatalf("expected a complete plan after one higher-budget retry, got %v with %v", err, budgets)
	}
	for _, tc := range []struct {
		name  string
		err   error
		calls int
	}{
		{"output limit", outputLimitError{provider: "Anthropic"}, 2},
		{"provider failure", errors.New("invalid credentials"), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			_, err := DraftComponents(t.Context(), input, func(context.Context, string, string, string, PlanPrompt) (string, error) {
				calls++
				return "", tc.err
			})
			if err == nil || calls != tc.calls || (tc.calls == 2 && !strings.Contains(err.Error(), "while drafting the component plan")) {
				t.Fatalf("unexpected retry count or error: %d, %v", calls, err)
			}
		})
	}
}

func TestComponentPlanBindsEveryCodeRequest(t *testing.T) {
	input := PlanRequest{Name: "Watchmaker", Format: "swiftui", Brief: "Teach assembly of a mechanical watch.",
		Provider: "anthropic", Model: "claude-opus-5-5", APIKey: "fake-key-for-test"}
	preparation, err := ReviewPlanning(input)
	if err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	planner := func(_ context.Context, provider, model, key string, step PlanPrompt) (string, error) {
		if provider != input.Provider || model != input.Model || key != input.APIKey || step != preparation.Prompts[0] {
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
	providerCall := func(_ context.Context, _, _, _ string, step PlanPrompt) (string, error) {
		i := len(sent)
		sent = append(sent, step)
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
	parent = t.TempDir()
	_, err = CreatePlanned(t.Context(), parent, input, func(context.Context, string, string, string, PlanPrompt) (string, error) {
		return returnedStarter, nil
	})
	if err == nil || !strings.Contains(err.Error(), "offline starter") {
		t.Fatalf("expected a starter-specific error from planned generation, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(parent, input.Name)); !os.IsNotExist(err) {
		t.Fatal("a planned project was created from the returned starter")
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
	generator := func(_ context.Context, _, _, _ string, step PlanPrompt) (string, error) {
		if len(step.Prompt) > briefChunkBytes+1800 {
			t.Fatalf("oversized request: %d bytes", len(step.Prompt))
		}
		if calls < len(preparation.Prompts) && step != preparation.Prompts[calls] {
			t.Fatal("sent unreviewed input from the original brief")
		}
		calls++
		tokens += step.MaxOutputTokens
		if step.MaxOutputTokens == planTokens {
			return "", outputLimitError{provider: "Anthropic"}
		}
		if step.MaxOutputTokens == planRetryTokens {
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
	review, err := DraftComponents(t.Context(), input, func(context.Context, string, string, string, PlanPrompt) (string, error) {
		return watchPlanJSON, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	input.Plan, input.Approval = review.Plan, review.Approval
	parent := t.TempDir()
	count := 0
	_, err = CreatePlanned(t.Context(), parent, input, func(context.Context, string, string, string, PlanPrompt) (string, error) {
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
