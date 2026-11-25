package studio

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
