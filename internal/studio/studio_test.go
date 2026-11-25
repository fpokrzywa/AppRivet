package studio

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fpokrzywa/AppRivet/internal/generation"
	"github.com/fpokrzywa/AppRivet/internal/project"
)

func TestStudioCreatesAndListsProject(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "AppRivet Projects")
	var opened string
	var quit bool
	handler := Handler(Config{
		Host: "127.0.0.1:1234", Session: "local-test-session", Workspace: workspace,
		OpenFolder: func(path string) error { opened = path; return nil }, Quit: func() { quit = true },
	})
	request := func(method, path, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "http://127.0.0.1:1234"+path, strings.NewReader(body))
		req.Header.Set("X-AppRivet-Session", "local-test-session")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s: got %d, want %d: %s", method, path, res.Code, want, res.Body)
		}
		return res
	}
	page := request(http.MethodGet, "/", "", http.StatusOK)
	if !strings.Contains(page.Body.String(), "Describe your app") || !strings.Contains(page.Body.String(), "<style>") ||
		!strings.Contains(page.Body.String(), "<script>") || !strings.Contains(page.Header().Get("Content-Security-Policy"), "style-src 'sha256-") {
		t.Fatal("embedded UI or content restrictions are missing")
	}
	request(http.MethodPost, "/api/create", `{"name":"Sample"}`, http.StatusCreated)
	request(http.MethodPost, "/api/project", `{"name":"../outside"}`, http.StatusBadRequest)
	root := filepath.Join(workspace, "Sample")
	projectFile := filepath.Join(root, "Sample.xcodeproj", "project.pbxproj")
	before, err := os.ReadFile(projectFile)
	if err != nil || !strings.Contains(string(before), "com.example.sample") {
		t.Fatal("default bundle ID was not written into the generated project", err)
	}
	request(http.MethodPost, "/api/create", `{"name":"Sample","bundle_id":"org.example.replacement"}`, http.StatusBadRequest)
	after, err := os.ReadFile(projectFile)
	if err != nil || string(after) != string(before) {
		t.Fatal("a duplicate creation attempt changed the existing project", err)
	}
	request(http.MethodPost, "/api/create", `{"name":"../escaped"}`, http.StatusBadRequest)
	request(http.MethodPost, "/api/create", `{"name":"SampleTwo","path":"outside"}`, http.StatusBadRequest)
	res := request(http.MethodGet, "/api/state", "", http.StatusOK)
	var data state
	if err := json.NewDecoder(res.Body).Decode(&data); err != nil || data.Workspace != workspace || len(data.Projects) != 1 || data.Projects[0] != "Sample" {
		t.Fatalf("invalid project inventory: %+v, %v", data, err)
	}
	request(http.MethodPost, "/api/open-folder", "", http.StatusNoContent)
	request(http.MethodPost, "/api/quit", "", http.StatusNoContent)
	if opened != workspace || !quit {
		t.Fatalf("desktop actions did not use the configured workspace: %q", opened)
	}
}

func TestStudioRejectsOtherOriginsAndSessions(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "projects")
	handler := Handler(Config{Host: "127.0.0.1:1234", Session: "local-test-session", Workspace: workspace})
	for _, tc := range []struct{ name, host, origin, token string }{
		{"foreign host", "other.example:1234", "", "local-test-session"},
		{"foreign origin", "127.0.0.1:1234", "https://other.example", "local-test-session"},
		{"missing token", "127.0.0.1:1234", "", ""},
		{"wrong token", "127.0.0.1:1234", "", "wrong"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "http://"+tc.host+"/api/create", strings.NewReader(`{"name":"Unexpected"}`))
			req.Header.Set("X-AppRivet-Session", tc.token)
			req.Header.Set("Origin", tc.origin)
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			if res.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d", res.Code)
			}
		})
	}
	if _, err := os.Stat(workspace); !os.IsNotExist(err) {
		t.Fatalf("rejected requests changed the filesystem: %v", err)
	}
}

func TestStudioBuildsChecklistStarterFromBrief(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "projects")
	h := Handler(Config{Host: "127.0.0.1:1234", Session: "local-test-session", Workspace: workspace})
	brief := `A packing checklist for weekend trips. Keep the list when I reopen the app.`
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:1234/api/create", strings.NewReader(`{"name":"TripPack","starter":"auto","brief":"`+brief+`"}`))
	req.Header.Set("X-AppRivet-Session", "local-test-session")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusCreated || !strings.Contains(res.Body.String(), `"starter":"checklist"`) {
		t.Fatalf("brief did not select a checklist: %d %s", res.Code, res.Body)
	}
	root := filepath.Join(workspace, "TripPack")
	source, err := os.ReadFile(filepath.Join(root, "TripPack", "ContentView.swift"))
	if err != nil || !strings.Contains(string(source), "struct ChecklistItem") || !strings.Contains(string(source), "@AppStorage") {
		t.Fatalf("starter does not implement a saved checklist: %v", err)
	}
	saved, err := os.ReadFile(filepath.Join(root, "APP_BRIEF.md"))
	if err != nil || !strings.Contains(string(saved), brief) {
		t.Fatalf("project did not save the brief: %v", err)
	}
}

func TestStudioReviewsAIRequestBeforeCallingProvider(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "projects")
	brief := strings.Repeat("Keep the latest lap.\n", 5000) + "A timer that saves the latest lap."
	const key = "sk-ant-example-fake"
	const source = "import SwiftUI\n\nstruct ContentView: View {\n    var body: some View { Text(\"Lap timer\") }\n}"
	var calls int
	var draft generation.Draft
	h := Handler(Config{
		Host: "127.0.0.1:1234", Session: "local-test-session", Workspace: workspace,
		Generate: func(_ context.Context, provider, model, apiKey, prompt string) (string, error) {
			calls++
			if provider != draft.Provider || model != draft.Model || apiKey != key || prompt != draft.Prompt {
				t.Fatal("provider, credentials or prompt did not match the approved request")
			}
			return source, nil
		},
	})
	send := func(path string, data any, want int) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:1234"+path, strings.NewReader(string(body)))
		req.Header.Set("X-AppRivet-Session", "local-test-session")
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s returned %d, want %d: %s", path, res.Code, want, res.Body.String())
		}
		return res
	}
	input := createRequest{Name: "LapTimer", Brief: brief, Provider: "anthropic", Model: "claude-fable-5-2"}
	if err := json.NewDecoder(send("/api/review", input, http.StatusOK).Body).Decode(&draft); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(draft.Prompt, brief) || draft.BundleID != "com.example.laptimer" || draft.Starter != "ai" {
		t.Fatalf("incorrect prompt review: %+v", draft)
	}
	input.APIKey = key
	send("/api/create", input, http.StatusBadRequest)
	input.Approval = draft.Approval
	changed := input
	changed.Brief += " Now add a chart."
	send("/api/create", changed, http.StatusBadRequest)
	changed = input
	changed.Model = "claude-opus-5-5"
	send("/api/create", changed, http.StatusBadRequest)
	changed = input
	changed.Provider = "openai"
	send("/api/create", changed, http.StatusBadRequest)
	changed = input
	changed.Format = "flutter"
	changed.Platforms = []string{"web", "linux"}
	send("/api/create", changed, http.StatusBadRequest)
	if _, err := os.Stat(workspace); !os.IsNotExist(err) || calls != 0 {
		t.Fatalf("review or unapproved requests had side effects: calls=%d, stat=%v", calls, err)
	}

	res := send("/api/create", input, http.StatusCreated)
	if strings.Contains(res.Body.String(), key) || !strings.Contains(res.Body.String(), "\"starter\":\"ai\"") {
		t.Fatal("created project response returned the wrong output")
	}
	var reopened project.Opened
	res = send("/api/project", map[string]string{"name": "LapTimer"}, http.StatusOK)
	if strings.Contains(res.Body.String(), key) {
		t.Fatal("the project form exposed the API key")
	}
	if err := json.NewDecoder(res.Body).Decode(&reopened); err != nil || reopened.Settings.Brief != brief ||
		reopened.Settings.Model != input.Model || reopened.Settings.Provider != input.Provider || reopened.Settings.Starter != "ai" {
		t.Fatal("reopening did not restore AI project settings safely")
	}
	root := filepath.Join(workspace, "LapTimer")
	for file, want := range map[string]string{"LapTimer/ContentView.swift": source, "APP_BRIEF.md": brief} {
		got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
		if err != nil || !strings.Contains(string(got), want) || strings.Contains(string(got), key) {
			t.Fatalf("wrong generated content in %s: %v", file, err)
		}
	}
	send("/api/create", input, http.StatusBadRequest)
	if calls != 1 {
		t.Fatalf("provider called for a duplicate project: %d calls", calls)
	}
}
