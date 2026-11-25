// Package generation requests Swift source from an AI provider and saves it as an
// editable project for separate review and building.
package generation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fpokrzywa/AppRivet/internal/project"
)

var (
	apiKeyPattern = regexp.MustCompile("^[A-Za-z0-9_-]{8,512}$")
	modelPattern  = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$")
	contentView   = regexp.MustCompile(`(?m)^\s*struct\s+ContentView\s*:\s*(?:SwiftUI\.)?View\b`)
	client        = &http.Client{
		Timeout: 180 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
)

type Draft struct {
	project.Draft
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Approval string `json:"approval"`
}

func Review(name, bundle, brief, provider, model string) (Draft, error) {
	if provider != "openai" && provider != "anthropic" {
		return Draft{}, errors.New("choose OpenAI or Anthropic in Admin")
	}
	model = strings.TrimSpace(model)
	if !modelPattern.MatchString(model) {
		return Draft{}, errors.New("enter a valid API model ID in Admin")
	}
	draft, err := project.ReviewAI(name, bundle, brief)
	if err != nil {
		return Draft{}, err
	}
	approval := fmt.Sprintf("%x", sha256.Sum256([]byte(provider+"\n"+model+"\n"+draft.Prompt)))
	return Draft{Draft: draft, Provider: provider, Model: model, Approval: approval}, nil
}

type SourceGenerator func(ctx context.Context, provider, model, key, prompt string) (string, error)

func Create(ctx context.Context, parent string, draft Draft, approval, key string, generate SourceGenerator) (project.Info, error) {
	if approval == "" || approval != draft.Approval {
		return project.Info{}, errors.New("review and approve the current prompt, provider and model before sending")
	}
	key = strings.TrimSpace(key)
	if !apiKeyPattern.MatchString(key) {
		return project.Info{}, errors.New("add a valid API key for the selected provider in Admin")
	}
	if _, err := os.Lstat(filepath.Join(parent, draft.Name)); !errors.Is(err, os.ErrNotExist) {
		return project.Info{}, errors.New("choose a new project name or folder before generating")
	}
	if generate == nil {
		return project.Info{}, errors.New("AI generation is unavailable")
	}
	source, err := generate(ctx, draft.Provider, draft.Model, key, draft.Prompt)
	if err != nil {
		return project.Info{}, err
	}
	source = strings.TrimSpace(source)
	if !utf8.ValidString(source) || len(source) > 128*1024 || strings.ContainsRune(source, 0) ||
		!strings.HasPrefix(source, "import SwiftUI\n") && !strings.HasPrefix(source, "import SwiftUI\r\n") ||
		!contentView.MatchString(source) {
		return project.Info{}, errors.New("the model did not return a complete SwiftUI ContentView. No project was created")
	}
	return project.CreateGenerated(parent, draft.Draft, source)
}

func Generate(ctx context.Context, provider, model, key, prompt string) (string, error) {
	return requestSource(ctx, client, provider, model, key, prompt)
}

func requestSource(ctx context.Context, httpClient *http.Client, provider, model, key, prompt string) (string, error) {
	schema := map[string]any{
		"type": "object", "properties": map[string]any{"content_view": map[string]string{"type": "string"}},
		"required": []string{"content_view"}, "additionalProperties": false,
	}
	var endpoint, label string
	var payload map[string]any
	switch provider {
	case "openai":
		endpoint, label = "https://api.openai.com/v1/responses", "OpenAI"
		payload = map[string]any{
			"model": model, "input": prompt, "store": false, "max_output_tokens": 10000,
			"text": map[string]any{"format": map[string]any{"type": "json_schema", "name": "swiftui_source", "strict": true, "schema": schema}},
		}
	case "anthropic":
		endpoint, label = "https://api.anthropic.com/v1/messages", "Anthropic"
		payload = map[string]any{
			"model": model, "max_tokens": 10000,
			"messages":      []map[string]string{{"role": "user", "content": prompt}},
			"output_config": map[string]any{"format": map[string]any{"type": "json_schema", "schema": schema}},
		}
	default:
		return "", errors.New("unsupported AI provider")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if provider == "openai" {
		req.Header.Set("Authorization", "Bearer "+key)
	} else {
		req.Header.Set("X-API-Key", key)
		req.Header.Set("Anthropic-Version", "2023-06-01")
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("could not reach %s. Check your connection and try again", label)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return "", fmt.Errorf("%s rejected the API key or project permissions", label)
	case http.StatusRequestEntityTooLarge:
		return "", fmt.Errorf("%s rejected the prompt because it exceeds the provider's request limit", label)
	case http.StatusBadRequest, http.StatusNotFound:
		return "", fmt.Errorf("%s rejected the request. Check model access, structured output support, and the model's input limit", label)
	case http.StatusTooManyRequests:
		return "", fmt.Errorf("%s reported a rate or usage limit. Check your API project", label)
	default:
		return "", fmt.Errorf("%s could not complete the request (HTTP %d)", label, resp.StatusCode)
	}
	return decodeSource(resp.Body, provider)
}

func decodeSource(body io.Reader, provider string) (string, error) {
	type part struct{ Type, Text string }
	var result struct {
		Status     string
		StopReason string `json:"stop_reason"`
		Content    []part
		Output     []struct {
			Type    string
			Content []part
		}
	}
	decoder := json.NewDecoder(io.LimitReader(body, 512*1024))
	if err := decoder.Decode(&result); err != nil {
		return "", errors.New("the provider returned an unreadable response")
	}
	parts := result.Content
	if provider == "openai" {
		if result.Status != "completed" {
			return "", errors.New("the provider returned an incomplete response. Try a smaller app brief")
		}
		parts = nil
		for _, item := range result.Output {
			if item.Type == "message" {
				parts = append(parts, item.Content...)
			}
		}
	} else if result.StopReason == "refusal" {
		return "", errors.New("the provider declined to generate source for this prompt")
	} else if result.StopReason != "end_turn" {
		return "", errors.New("the provider returned an incomplete response. Try a smaller app brief")
	}
	var responseText string
	for _, part := range parts {
		switch part.Type {
		case "refusal":
			return "", errors.New("the provider declined to generate source for this prompt")
		case "text", "output_text":
			if responseText != "" {
				return "", errors.New("the provider returned more than one source result")
			}
			responseText = part.Text
		}
	}
	var output struct {
		ContentView string `json:"content_view"`
	}
	structured := json.NewDecoder(strings.NewReader(responseText))
	structured.DisallowUnknownFields()
	if err := structured.Decode(&output); err != nil || structured.Decode(new(any)) != io.EOF || output.ContentView == "" {
		return "", errors.New("the provider returned an invalid SwiftUI source result")
	}
	return output.ContentView, nil
}
