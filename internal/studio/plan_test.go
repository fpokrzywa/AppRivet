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
		GenerateBudgeted: func(_ context.Context, _, _, _, prompt string, budget int) (string, error) {
			calls++
			switch calls {
			case 1:
				if budget != 4500 {
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
	input.Approval, input.APIKey = review.Approval, "fake-key-for-test"
	if res := post("draft", 200); json.Unmarshal(res.Body.Bytes(), &review) != nil || len(review.Prompts) != 2 || calls != 1 {
		t.Fatal("planning did not return the code prompts and a new approval")
	}
	input.Plan = review.Plan
	post("create", 400)
	if calls != 1 {
		t.Fatal("planning approval authorized code generation")
	}
	input.Approval = review.Approval
	post("create", 201)
	if calls != 3 {
		t.Fatal("code generation did not make exactly one request per approved component")
	}
}
