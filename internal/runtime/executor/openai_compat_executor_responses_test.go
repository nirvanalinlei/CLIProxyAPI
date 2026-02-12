package executor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	_ "github.com/router-for-me/CLIProxyAPI/v6/internal/translator"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
	"github.com/tidwall/gjson"
)

func TestOpenAICompatExecutor_WireResponses(t *testing.T) {
	var gotPath string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_1","object":"response","usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`))
	}))
	defer srv.Close()

	cfg := &config.Config{OpenAICompatibility: []config.OpenAICompatibility{{Name: "p1", WireAPI: "responses"}}}
	executor := NewOpenAICompatExecutor("openai-compatibility", cfg)
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url":    srv.URL + "/v1",
		"api_key":     "k",
		"compat_name": "p1",
	}}
	_, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-4",
		Payload: []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`),
	}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai")})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if gotPath != "/v1/responses" {
		t.Fatalf("path = %q", gotPath)
	}
	if !gjson.GetBytes(gotBody, "input").Exists() || gjson.GetBytes(gotBody, "messages").Exists() {
		t.Fatalf("expected responses payload: %s", string(gotBody))
	}
}

func TestOpenAICompatExecutor_WireResponses_ErrorNoFallback(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := &config.Config{OpenAICompatibility: []config.OpenAICompatibility{{Name: "p1", WireAPI: "responses"}}}
	executor := NewOpenAICompatExecutor("openai-compatibility", cfg)
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url":    srv.URL + "/v1",
		"api_key":     "k",
		"compat_name": "p1",
	}}
	_, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-4",
		Payload: []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`),
	}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai")})
	if err == nil {
		t.Fatalf("expected error")
	}
	if hits != 1 {
		t.Fatalf("expected single upstream hit, got %d", hits)
	}
}
