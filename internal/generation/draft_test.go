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

func TestDraftResumesApprovedPlanAfterComponentFailure(t *testing.T) {
	parent, storage := t.TempDir(), filepath.Join(t.TempDir(), "drafts")
	input := PlanRequest{Name: "Watchmaker", Brief: "Build a working watch.", Provider: "anthropic", Model: "test-model",
		APIKey: "fake-key-for-test", Clarifications: []string{"Use metric units."},
		Plan: &ComponentPlan{Architecture: "ContentView shows the assembled watch.",
			Components: []Component{{"Workbench", "Build the watch."}, {"Watch", "Display the watch."}},
			Stages:     []PlanStage{{"Workshop", 1}, {"Finished watch", 1}}, OpenItems: []string{"Should progress survive a restart?"}}}
	review, err := ReviewComponents(input)
	if err != nil || len(review.Stages) != 2 {
		t.Fatalf("review: %+v, %v", review, err)
	}
	input.Approval = review.Approval
	store := DraftStore{Directory: storage}
	var sent []string
	fail := true
	generate := func(_ context.Context, _, _, _ string, step PlanPrompt) (string, error) {
		sent = append(sent, step.Title)
		if step.File == "ContentView.swift" {
			return "import SwiftUI\nstruct ContentView: View { var body: some View { Watch() } }", nil
		}
		if fail {
			return "", errors.New("provider unavailable")
		}
		return "import SwiftUI\nstruct Watch: View { var body: some View { Text(\"Watch\") } }", nil
	}
	if progress, err := store.Advance(t.Context(), parent, input, generate); err != nil || progress.Completed != 1 || progress.Project != nil {
		t.Fatalf("first component: %+v, %v", progress, err)
	}
	if _, err := store.Advance(t.Context(), parent, input, generate); err == nil || !strings.Contains(err.Error(), "component 2 of 2 (Watch)") {
		t.Fatalf("expected the failing component, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(parent, input.Name)); !os.IsNotExist(err) {
		t.Fatal("generation exposed an incomplete project")
	}
	// Reopen through the local review entry point. No key or previous UI state.
	reopened := DraftStore{Directory: storage}
	local := input
	local.Approval, local.APIKey, local.Plan, local.Clarifications = "", "", nil, nil
	saved, err := reopened.Review(parent, local)
	if err != nil || !saved.Resuming || saved.Completed != 1 || saved.Approval != input.Approval ||
		!slices.Equal(saved.Clarifications, input.Clarifications) {
		t.Fatalf("saved review: %+v, %v", saved, err)
	}
	entries, err := os.ReadDir(storage)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if body, err := os.ReadFile(filepath.Join(storage, entry.Name())); err != nil || strings.Contains(string(body), input.APIKey) {
			t.Fatal("could not verify saved contents are free of the API key")
		}
	}
	changed := input
	changed.Clarifications = []string{"Use inches."}
	changedReview, err := ReviewComponents(changed)
	if err != nil || changedReview.Approval == input.Approval {
		t.Fatal("changed clarifications reused the approved plan")
	}
	changed.Approval = changedReview.Approval
	if _, err := reopened.Advance(t.Context(), parent, changed, generate); err == nil || len(sent) != 2 {
		t.Fatal("another plan reused or overwrote a saved draft")
	}
	fail = false
	if progress, err := reopened.Advance(t.Context(), parent, input, generate); err != nil || progress.Completed != 2 || progress.Project != nil {
		t.Fatalf("resumed the wrong result: %+v, %v", progress, err)
	}
	if progress, err := reopened.Advance(t.Context(), parent, input, generate); err != nil || progress.Project == nil {
		t.Fatalf("assemble result: %+v, %v", progress, err)
	}
	if !slices.Equal(sent, []string{"Workbench", "Watch", "Watch"}) {
		t.Fatalf("unexpected provider calls: %v", sent)
	}
	if _, err := os.Stat(filepath.Join(parent, input.Name, input.Name, "AppRivetPart01.swift")); err != nil {
		t.Fatal(err)
	}
	if review, err := reopened.Review(parent, local); err != nil || review.Resuming {
		t.Fatalf("completion left a resumable draft: %+v, %v", review, err)
	}
}

func TestPlanningClarificationRequiresFreshReview(t *testing.T) {
	request := PlanRequest{Name: "Watchmaker", Brief: "Build a watch.", Provider: "openai", Model: "test-model", APIKey: "fake-key-for-test"}
	initial, err := ReviewPlanning(request)
	if err != nil {
		t.Fatal(err)
	}
	request.Approval, request.Clarifications = initial.Approval, []string{"Save progress locally."}
	calls := 0
	provider := func(_ context.Context, _, _, _ string, step PlanPrompt) (string, error) {
		calls++
		if !strings.Contains(step.Prompt, request.Clarifications[0]) || !strings.Contains(step.Prompt, request.Brief) {
			t.Fatal("the request lost the brief or clarification")
		}
		return `{"architecture":"ContentView renders Watch.","stages":[{"title":"Workbench","count":1},{"title":"Watch features","count":1}],"components":[{"title":"Main","scope":"Present Watch."},{"title":"Watch","scope":"Save progress locally."}],"open_items":["Which watch movement should be modeled?"]}`, nil
	}
	if _, err := DraftComponents(t.Context(), request, provider); err == nil || calls != 0 {
		t.Fatal("changed clarification was sent under an old approval")
	}
	revised, err := ReviewPlanning(request)
	if err != nil {
		t.Fatal(err)
	}
	request.Approval = revised.Approval
	result, err := DraftComponents(t.Context(), request, provider)
	if err != nil || calls != 1 || len(result.Stages) != 2 || len(result.Plan.OpenItems) != 1 {
		t.Fatalf("revised plan: %+v, %v", result, err)
	}
	request.Plan = result.Plan
	request.Plan.Stages[1].Count = 2
	if _, err := ReviewComponents(request); err == nil {
		t.Fatal("stage counts assigned a component more than once")
	}
}

func TestConcurrentAdvanceDoesNotRepeatProviderRequest(t *testing.T) {
	parent := t.TempDir()
	store := DraftStore{Directory: filepath.Join(t.TempDir(), "drafts")}
	input := PlanRequest{Name: "Watchmaker", Brief: "Create a watch.", Provider: "anthropic", Model: "test-model", APIKey: "fake-key-for-test",
		Plan: &ComponentPlan{Architecture: "ContentView renders Watch.", Components: []Component{{"Main", "Show Watch."}, {"Watch", "Show time."}}}}
	review, err := ReviewComponents(input)
	if err != nil {
		t.Fatal(err)
	}
	input.Approval = review.Approval
	started, release := make(chan struct{}), make(chan struct{})
	first := make(chan error, 1)
	go func() {
		_, err := store.Advance(t.Context(), parent, input, func(context.Context, string, string, string, PlanPrompt) (string, error) {
			close(started)
			<-release
			return "import SwiftUI\nstruct ContentView: View { var body: some View { Watch() } }", nil
		})
		first <- err
	}()
	select {
	case <-started:
	case err := <-first:
		t.Fatalf("first advance did not reach the provider: %v", err)
	}
	called := false
	_, concurrentErr := store.Advance(t.Context(), parent, input, func(context.Context, string, string, string, PlanPrompt) (string, error) {
		called = true
		return "", errors.New("unexpected duplicate request")
	})
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if called || concurrentErr == nil || !strings.Contains(concurrentErr.Error(), "lock this draft") {
		t.Fatalf("concurrent advance reached the provider or did not report the lock: %v", concurrentErr)
	}
}
