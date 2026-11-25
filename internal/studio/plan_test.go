package studio

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/fpokrzywa/AppRivet/internal/generation"
)

func TestStudioPlansThenGeneratesWithOneCodeApproval(t *testing.T) {
	calls := 0
	handler := Handler(Config{
		Host: "127.0.0.1:1234", Session: "test-session", Workspace: filepath.Join(t.TempDir(), "projects"),
		Drafts: generation.DraftStore{Directory: filepath.Join(t.TempDir(), "drafts")},
		GenerateBudgeted: func(_ context.Context, _, _, _ string, step generation.PlanPrompt) (string, error) {
			calls++
			switch calls {
			case 1:
				if step.MaxOutputTokens != 8000 || step.File != "" {
					t.Fatal("planning budget was not applied")
				}
				return `{"architecture":"ContentView uses WatchView.","components":[{"title":"Main","scope":"Display WatchView."},{"title":"Watch","scope":"Define WatchView."}]}`, nil
			case 2:
				return "import SwiftUI\nstruct ContentView: View { var body: some View { WatchView() } }", nil
			default:
				return "import SwiftUI\nstruct WatchView: View { var body: some View { Text(\"Watch\") } }", nil
			}
		},
	})
	input := generation.PlanRequest{Name: "Watchmaker", Brief: "Make a watch.", Provider: "anthropic", Model: "claude-opus-5-5"}
	post := func(path string, status int) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:1234/api/plan/"+path, bytes.NewReader(body))
		request.Header.Set("X-AppRivet-Session", "test-session")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, request)
		if res.Code != status {
			t.Fatalf("%s returned %d: %s", path, res.Code, res.Body)
		}
		return res
	}
	var review generation.PlanReview
	if res := post("review", 200); json.Unmarshal(res.Body.Bytes(), &review) != nil || len(review.Prompts) != 1 || calls != 0 {
		t.Fatal("local review made an unexpected provider call")
	}
	if review.PlanRetryOutputTokens != 16000 || review.RequestCount != 2 || review.MaxOutputTokens != 24000 {
		t.Fatal("local review did not include the optional planning retry")
	}
	input.Approval, input.APIKey = review.Approval, "fake-key-for-test"
	if res := post("draft", 200); json.Unmarshal(res.Body.Bytes(), &review) != nil || len(review.Prompts) != 2 || calls != 1 {
		t.Fatal("planning did not return the code prompts and a new approval")
	}
	if review.ComponentRetryOutputTokens != 16000 || review.RequestCount != 4 || review.MaxOutputTokens != 48000 {
		t.Fatal("code review did not include the optional retry for each component")
	}
	input.Plan = review.Plan
	post("advance", 400)
	if calls != 1 {
		t.Fatal("planning approval authorized code generation")
	}
	input.Approval = review.Approval
	for completed := 1; completed <= 2; completed++ {
		var progress generation.GenerationProgress
		res := post("advance", 200)
		if err := json.Unmarshal(res.Body.Bytes(), &progress); err != nil || progress.Completed != completed || progress.Project != nil {
			t.Fatalf("unexpected generation progress %+v: %v", progress, err)
		}
		if calls != 1+completed {
			t.Fatal("advance did not send exactly one component under the original code approval")
		}
	}
	var finished generation.GenerationProgress
	if res := post("advance", 200); json.Unmarshal(res.Body.Bytes(), &finished) != nil || finished.Project == nil || calls != 3 {
		t.Fatal("assembly did not reuse the completed components")
	}
}
