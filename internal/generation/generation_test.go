package generation

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fpokrzywa/AppRivet/internal/project"
)

const returnedStarter = `import SwiftUI
struct ContentView: View {
    var body: some View { Text("Your first iOS app is ready to build.") }
}`

func TestGenerationDoesNotSaveTheOfflineStarter(t *testing.T) {
	parent := t.TempDir()
	draft, err := Review("Watchmaker", "com.example.watchmaker", "Teach me to assemble a watch.", "anthropic", "test-model", project.TargetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = Create(t.Context(), parent, draft, draft.Approval, "fake-test-key", func(context.Context, string, string, string, string) (string, error) {
		return returnedStarter, nil
	})
	if err == nil || !strings.Contains(err.Error(), "offline starter") {
		t.Fatalf("expected a starter-specific error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(parent, draft.Name)); !os.IsNotExist(err) {
		t.Fatal("a project was created from the returned starter")
	}
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestProviderRequestSendsReviewedPrompt(t *testing.T) {
	const prompt = "Build a SwiftUI timer.\n\nA brief with \"quotes\" and 世界."
	const fakeKey = "fake-api-key-for-test"
	for _, tc := range []struct{ provider, model, endpoint, authHeader, auth, response string }{
		{"openai", "gpt-6.2-sol", "https://api.openai.com/v1/responses", "Authorization", "Bearer " + fakeKey,
			`{"status":"completed","output":[{"type":"reasoning"},{"type":"message","content":[{"type":"output_text","text":"{\"source\":\"source\"}"}]}]}`},
		{"anthropic", "claude-fable-5-2", "https://api.anthropic.com/v1/messages", "X-API-Key", fakeKey,
			`{"stop_reason":"end_turn","content":[{"type":"thinking"},{"type":"text","text":"{\"source\":\"source\"}"}]}`},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			httpClient := &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.String() != tc.endpoint || req.Method != http.MethodPost || req.Header.Get(tc.authHeader) != tc.auth {
					t.Fatal("provider request has the wrong endpoint, method, or credentials")
				}
				if tc.provider == "anthropic" && req.Header.Get("Anthropic-Version") != "2023-06-01" {
					t.Fatal("Anthropic API version missing")
				}
				var payload struct {
					Model           string
					Input           string
					Store           *bool
					Text            json.RawMessage
					Messages        []struct{ Role, Content string }
					Output          json.RawMessage `json:"output_config"`
					MaxTokens       int             `json:"max_tokens"`
					MaxOutputTokens int             `json:"max_output_tokens"`
				}
				if err := json.NewDecoder(req.Body).Decode(&payload); err != nil || payload.Model != tc.model {
					t.Fatalf("invalid generation request: %v", err)
				}
				if tc.provider == "openai" && (payload.Input != prompt || payload.Store == nil || *payload.Store || len(payload.Text) == 0) {
					t.Fatal("OpenAI call changed the reviewed prompt or omitted request settings")
				}
				if tc.provider == "anthropic" && (len(payload.Messages) != 1 || payload.Messages[0].Role != "user" ||
					payload.Messages[0].Content != prompt || len(payload.Output) == 0) {
					t.Fatal("Anthropic call changed the reviewed prompt or omitted structured output")
				}
				if tc.provider == "openai" && payload.MaxOutputTokens != 25000 ||
					tc.provider == "anthropic" && payload.MaxTokens != 20000 {
					t.Fatal("the built-in model did not receive its generation budget")
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tc.response))}, nil
			})}
			source, err := requestSource(context.Background(), httpClient, tc.provider, tc.model, fakeKey, prompt)
			if err != nil || source != "source" {
				t.Fatalf("did not decode source: %q, %v", source, err)
			}
		})
	}
}

func TestIncompleteProviderResponse(t *testing.T) {
	for _, tc := range []struct{ name, provider, response, want string }{
		{"OpenAI budget", "openai", `{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[{"type":"message","content":[{"type":"output_text","text":"{\"source\":\"partial\"}"}]}]}`, "OpenAI reached its output limit"},
		{"OpenAI filter", "openai", `{"status":"incomplete","incomplete_details":{"reason":"content_filter"}}`, "OpenAI stopped generation because of its content filter"},
		{"OpenAI other", "openai", `{"status":"incomplete","incomplete_details":{"reason":"private diagnostic"}}`, "OpenAI did not return complete app source"},
		{"Anthropic budget", "anthropic", `{"stop_reason":"max_tokens","content":[{"type":"text","text":"{\"source\":\"partial\"}"}]}`, "Anthropic reached its output limit"},
		{"Anthropic context", "anthropic", `{"stop_reason":"model_context_window_exceeded"}`, "Anthropic reached the model's context limit"},
		{"Anthropic refusal", "anthropic", `{"stop_reason":"refusal"}`, "Anthropic declined to generate source"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source, err := decodeSource(strings.NewReader(tc.response), tc.provider)
			if source != "" || err == nil || !strings.HasPrefix(err.Error(), tc.want) || strings.Contains(err.Error(), "private diagnostic") {
				t.Fatalf("expected %q without source or provider diagnostics, got %q, %v", tc.want, source, err)
			}
		})
	}
}

func TestProviderFailureDoesNotReturnRawProviderMessage(t *testing.T) {
	httpClient := &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader("provider diagnostic secret"))}, nil
	})}
	_, err := requestSource(context.Background(), httpClient, "openai", "gpt-6.2-sol", "fake-api-key", "brief")
	if err == nil || strings.Contains(err.Error(), "provider diagnostic secret") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPlanBudgetsReachProvider(t *testing.T) {
	for _, provider := range []string{"openai", "anthropic"} {
		t.Run(provider, func(t *testing.T) {
			client := &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				var body map[string]any
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				field := "max_output_tokens"
				if provider == "anthropic" {
					field = "max_tokens"
				}
				if body[field] != float64(summaryTokens) {
					t.Fatal("provider request did not use the approved summary budget")
				}
				return &http.Response{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader(""))}, nil
			})}
			if _, err := requestSourceWithBudget(t.Context(), client, provider, "test-model", "test-key", "brief", summaryTokens); err == nil {
				t.Fatal("expected a rate limit from the mock provider")
			}
		})
	}
}
