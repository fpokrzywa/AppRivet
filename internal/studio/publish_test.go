package studio

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fpokrzywa/AppRivet/internal/project"
	"github.com/fpokrzywa/AppRivet/internal/storeconnect"
)

func TestPublishRoutesRequireSessionAndTrust(t *testing.T) {
	workspace := t.TempDir()
	_, err := project.Create(workspace, "StoreFixture", "org.example.storefixture")
	if err != nil {
		t.Fatal(err)
	}
	settings := storeconnect.Store{Directory: filepath.Join(t.TempDir(), "settings")}
	handler := Handler(Config{Host: "127.0.0.1:1234", Session: "local-session", Workspace: workspace, PublishStore: &settings})
	send := func(method, path, key, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, "http://127.0.0.1:1234/api/publish/"+path, strings.NewReader(body))
		request.Header.Set("X-AppRivet-Session", key)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != want {
			t.Fatalf("%s %s returned %d: %s", method, path, response.Code, response.Body)
		}
		return response
	}
	send("GET", "settings", "wrong-session", "", 403)
	send("POST", "settings", "local-session", `{"apple_team_id":"not a team"}`, 400)
	send("POST", "settings", "local-session", `{"apple_team_id":"A123456789"}`, 200)
	response := send("GET", "settings", "local-session", "", 200)
	var saved storeconnect.Status
	if err := json.Unmarshal(response.Body.Bytes(), &saved); err != nil || saved.AppleTeamID != "A123456789" ||
		saved.CredentialStorage || saved.HasAppleKey || saved.HasGoogleKey || strings.Contains(response.Body.String(), "private_key") {
		t.Fatal("publishing settings did not return safe metadata")
	}
	send("POST", "connection", "local-session", `{"store":"apple","apple_private_key":"inline-key"}`, 400)
	send("POST", "connection", "local-session", `{"store":"apple"}`, 400)
	send("POST", "history", "local-session", `{"name":"../outside"}`, 400)
	if res := send("POST", "history", "local-session", `{"name":"StoreFixture"}`, 200); strings.TrimSpace(res.Body.String()) != "[]" {
		t.Fatal("unexpected release records for a fresh project")
	}
	if res := send("POST", "build", "local-session", `{"name":"StoreFixture","platform":"ios","trusted":false}`, 400); !strings.Contains(res.Body.String(), "trust") {
		t.Fatal("an untrusted store build was not rejected")
	}
	send("POST", "open-artifact", "local-session", `{"name":"StoreFixture","sha256":"unknown"}`, 400)
}
