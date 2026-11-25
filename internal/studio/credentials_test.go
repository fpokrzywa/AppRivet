package studio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/fpokrzywa/AppRivet/internal/generation"
)

const fixtureKey = "sk-studio-fake-fixture"

type fixtureVault struct {
	keys map[string]string
	gets int
	err  error
}

func (v *fixtureVault) Get(provider string) (string, error) {
	v.gets++
	return v.keys[provider], v.err
}

func (v *fixtureVault) Save(provider, key string) error {
	if v.err == nil {
		v.keys[provider] = key
	}
	return v.err
}

func (v *fixtureVault) Remove(provider string) error {
	if v.err == nil {
		delete(v.keys, provider)
	}
	return v.err
}

func vaultRequest(t *testing.T, h http.Handler, method, path string, body any, want int) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(method, "http://127.0.0.1:1234"+path, bytes.NewReader(data))
	r.Header.Set("X-AppRivet-Session", "test-session")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, r)
	if res.Code != want || strings.Contains(res.Body.String(), fixtureKey) {
		t.Fatalf("%s %s returned %d or exposed a key, expected %d", method, path, res.Code, want)
	}
	return res
}

func TestStudioCredentialEndpointsReportProviderIDsOnly(t *testing.T) {
	vault := &fixtureVault{keys: map[string]string{}}
	config := Config{Host: "127.0.0.1:1234", Session: "test-session", Workspace: t.TempDir(), Vault: vault}
	h := Handler(config)
	keyRequest := map[string]string{"provider": "anthropic", "key": fixtureKey}
	unauthorized := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:1234/api/provider-key", strings.NewReader(`{"provider":"anthropic","key":"`+fixtureKey+`"}`))
	refusal := httptest.NewRecorder()
	h.ServeHTTP(refusal, unauthorized)
	if refusal.Code != http.StatusForbidden || len(vault.keys) != 0 {
		t.Fatal("the vault accepted an untrusted session")
	}
	vaultRequest(t, h, http.MethodPost, "/api/provider-key", map[string]string{"provider": "unknown", "key": fixtureKey}, 400)
	vaultRequest(t, h, http.MethodPost, "/api/provider-key", map[string]string{"provider": "anthropic", "key": "invalid\nkey"}, 400)
	vaultRequest(t, h, http.MethodPost, "/api/remove-provider-key", keyRequest, 400)
	if len(vault.keys) != 0 {
		t.Fatal("an invalid request changed the vault")
	}
	vaultRequest(t, h, http.MethodPost, "/api/provider-key", keyRequest, 204)
	if vault.keys["anthropic"] != fixtureKey {
		t.Fatal("the credential was not persisted")
	}
	h = Handler(config)
	read := func(wantStorage bool, wantKeys ...string) {
		t.Helper()
		var data state
		res := vaultRequest(t, h, http.MethodGet, "/api/state", nil, 200)
		if err := json.Unmarshal(res.Body.Bytes(), &data); err != nil || data.CredentialStorage != wantStorage || data.SavedKeys == nil || !slices.Equal(data.SavedKeys, wantKeys) {
			t.Fatal("state did not describe credential storage")
		}
	}
	read(true, "anthropic")
	vault.err = errors.New("fixture details must stay private: " + fixtureKey)
	read(true)
	vaultRequest(t, h, http.MethodPost, "/api/provider-key", keyRequest, 503)
	vault.err = nil
	vaultRequest(t, h, http.MethodPost, "/api/remove-provider-key", map[string]string{"provider": "anthropic"}, 204)
	read(true)
	config.Vault = nil
	h = Handler(config)
	read(false)
	vaultRequest(t, h, http.MethodPost, "/api/provider-key", keyRequest, 503)
}

func TestStudioUsesSavedCredentialOnlyForApprovedCalls(t *testing.T) {
	vault := &fixtureVault{keys: map[string]string{"anthropic": fixtureKey}}
	wantKey := fixtureKey
	var planLeaksKey bool
	var calls int
	h := Handler(Config{Host: "127.0.0.1:1234", Session: "test-session", Workspace: filepath.Join(t.TempDir(), "projects"), Vault: vault,
		Generate: func(_ context.Context, _, _, key, _ string) (string, error) {
			calls++
			if key != wantKey {
				t.Fatal("normal generation received the wrong credential")
			}
			if wantKey != fixtureKey {
				return "", errors.New("failed with " + wantKey)
			}
			return "import SwiftUI\nstruct ContentView: View { var body: some View { Text(\"Watch\") } }", nil
		},
		GenerateBudgeted: func(_ context.Context, _, _, key, prompt string, budget int) (string, error) {
			calls++
			if key != wantKey {
				t.Fatal("staged generation received the wrong credential")
			}
			if budget == 4500 {
				if planLeaksKey {
					return `{"architecture":"A provider accidentally returned ` + key + `.","components":[{"title":"Main","scope":"Display WatchView."},{"title":"Watch","scope":"Define WatchView."}]}`, nil
				}
				return `{"architecture":"ContentView uses WatchView.","components":[{"title":"Main","scope":"Display WatchView."},{"title":"Watch","scope":"Define WatchView."}]}`, nil
			}
			if strings.Contains(prompt, "AppRivetPart01.swift") && strings.Contains(prompt, "Component: Watch") {
				return "import SwiftUI\nstruct WatchView: View { var body: some View { Text(\"Watch\") } }", nil
			}
			return "import SwiftUI\nstruct ContentView: View { var body: some View { WatchView() } }", nil
		},
	})
	post := func(path string, body any, want int) *httptest.ResponseRecorder {
		t.Helper()
		return vaultRequest(t, h, http.MethodPost, path, body, want)
	}
	decode := func(res *httptest.ResponseRecorder, value any) {
		t.Helper()
		if err := json.Unmarshal(res.Body.Bytes(), value); err != nil {
			t.Fatal(err)
		}
	}
	req := createRequest{Name: "Watch", Brief: "Make a watch.", Provider: "anthropic", Model: "claude-opus-5-5"}
	var quick generation.Draft
	decode(post("/api/review", req, 200), &quick)
	post("/api/create", req, 400)
	stage := generation.PlanRequest{Name: "Watchmaker", Brief: req.Brief, Provider: req.Provider, Model: req.Model}
	var review generation.PlanReview
	decode(post("/api/plan/review", stage, 200), &review)
	post("/api/plan/draft", stage, 400)
	if calls != 0 || vault.gets != 0 {
		t.Fatal("an unapproved request accessed a vault or provider")
	}
	req.Approval = quick.Approval
	post("/api/create", req, 201)
	stage.Approval = review.Approval
	decode(post("/api/plan/draft", stage, 200), &review)
	stage.Plan = review.Plan
	post("/api/plan/create", stage, 400)
	if calls != 2 || vault.gets != 2 {
		t.Fatal("planning approval authorized loading a key for code generation")
	}
	stage.Approval = review.Approval
	post("/api/plan/create", stage, 201)
	if calls != 4 || vault.gets != 3 {
		t.Fatal("approved calls did not use the saved key")
	}
	wantKey = "sk-explicit-fake-fixture"
	req.Name, req.APIKey = "WatchOverride", wantKey
	decode(post("/api/review", req, 200), &quick)
	req.Approval = quick.Approval
	if response := post("/api/create", req, 400); strings.Contains(response.Body.String(), wantKey) || vault.gets != 3 || calls != 5 {
		t.Fatal("the explicit key did not take precedence or leaked in an error")
	}
	wantKey, planLeaksKey = fixtureKey, true
	stage.Plan = nil
	decode(post("/api/plan/review", stage, 200), &review)
	stage.Approval = review.Approval
	post("/api/plan/draft", stage, 400)
}
