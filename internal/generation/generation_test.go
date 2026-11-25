package generation

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestProviderRequestSendsReviewedPrompt(t *testing.T) {
	const prompt = "Build a SwiftUI timer.\n\nA brief with \"quotes\" and 世界."
	const fakeKey = "fake-api-key-for-test"
	for _, tc := range []struct{ provider, model, endpoint, authHeader, auth, response string }{
		{"openai", "gpt-6.2-sol", "https://api.openai.com/v1/responses", "Authorization", "Bearer " + fakeKey,
			`{"status":"completed","output":[{"type":"reasoning"},{"type":"message","content":[{"type":"output_text","text":"{\"content_view\":\"source\"}"}]}]}`},
		{"anthropic", "claude-fable-5-2", "https://api.anthropic.com/v1/messages", "X-API-Key", fakeKey,
			`{"stop_reason":"end_turn","content":[{"type":"thinking"},{"type":"text","text":"{\"content_view\":\"source\"}"}]}`},
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
					Model    string
					Input    string
					Store    *bool
					Text     json.RawMessage
					Messages []struct{ Role, Content string }
					Output   json.RawMessage `json:"output_config"`
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
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tc.response))}, nil
			})}
			source, err := requestSource(context.Background(), httpClient, tc.provider, tc.model, fakeKey, prompt)
			if err != nil || source != "source" {
				t.Fatalf("did not decode source: %q, %v", source, err)
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
