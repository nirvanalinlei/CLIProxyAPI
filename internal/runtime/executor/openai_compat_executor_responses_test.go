package executor

import (
	"bytes"
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

func TestOpenAICompatExecutorUsesResponsesWhenWireAPIConfigured(t *testing.T) {
	var gotPath string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		gotBody = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_1","object":"response","output":[{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]}]}`))
	}))
	defer server.Close()

	cfg := &config.Config{
		OpenAICompatibility: []config.OpenAICompatibility{
			{
				Name:    "test-provider",
				WireAPI: config.OpenAIWireAPIResponses,
			},
		},
	}

	executor := NewOpenAICompatExecutor("openai-compatibility", cfg)
	auth := &cliproxyauth.Auth{
		Provider: "test-provider",
		Attributes: map[string]string{
			"base_url": server.URL + "/v1",
			"api_key":  "test",
		},
	}
	payload := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`)
	resp, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-4",
		Payload: payload,
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
		Stream:       false,
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if gotPath != "/v1/responses" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/responses")
	}
	if !gjson.GetBytes(gotBody, "input").Exists() {
		t.Fatalf("expected input in body")
	}
	if gjson.GetBytes(gotBody, "messages").Exists() {
		t.Fatalf("unexpected messages in body")
	}
	if !gjson.GetBytes(resp.Payload, "choices").Exists() {
		t.Fatalf("expected chat completion response")
	}
	if gjson.GetBytes(resp.Payload, "output").Exists() {
		t.Fatalf("unexpected responses output in payload")
	}
}

func TestOpenAICompatExecutorResponsesFallsBackViaOpenAIForClaudeSource(t *testing.T) {
	var gotPath string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		gotBody = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_1","object":"response","output":[{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]}]}`))
	}))
	defer server.Close()

	cfg := &config.Config{
		OpenAICompatibility: []config.OpenAICompatibility{
			{
				Name:    "test-provider",
				WireAPI: config.OpenAIWireAPIResponses,
			},
		},
	}

	executor := NewOpenAICompatExecutor("openai-compatibility", cfg)
	auth := &cliproxyauth.Auth{
		Provider: "test-provider",
		Attributes: map[string]string{
			"base_url": server.URL + "/v1",
			"api_key":  "test",
		},
	}
	claudePayload := []byte(`{"model":"gpt-4","max_tokens":128,"messages":[{"role":"user","content":"hello"}]}`)
	resp, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-4",
		Payload: claudePayload,
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("claude"),
		Stream:       false,
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if gotPath != "/v1/responses" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/responses")
	}
	if !gjson.GetBytes(gotBody, "input").Exists() {
		t.Fatalf("expected input in body")
	}
	if gjson.GetBytes(gotBody, "messages").Exists() {
		t.Fatalf("unexpected messages in body")
	}
	if got := gjson.GetBytes(gotBody, "input.0.role").String(); got != "user" {
		t.Fatalf("input.0.role = %q, want %q", got, "user")
	}
	if got := gjson.GetBytes(gotBody, "input.0.content.0.type").String(); got != "input_text" {
		t.Fatalf("input.0.content.0.type = %q, want %q", got, "input_text")
	}
	if !gjson.GetBytes(resp.Payload, "content.0.text").Exists() {
		t.Fatalf("expected claude response payload")
	}
}

func TestOpenAICompatExecutorResponsesStreamFallsBackViaOpenAIForClaudeSource(t *testing.T) {
	var gotPath string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		gotBody = body
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"response_id\":\"resp_1\",\"delta\":\"hi\"}\n\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\"}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	cfg := &config.Config{
		OpenAICompatibility: []config.OpenAICompatibility{
			{
				Name:    "test-provider",
				WireAPI: config.OpenAIWireAPIResponses,
			},
		},
	}

	executor := NewOpenAICompatExecutor("openai-compatibility", cfg)
	auth := &cliproxyauth.Auth{
		Provider: "test-provider",
		Attributes: map[string]string{
			"base_url": server.URL + "/v1",
			"api_key":  "test",
		},
	}
	claudePayload := []byte(`{"model":"gpt-4","max_tokens":128,"stream":true,"messages":[{"role":"user","content":"hello"}]}`)
	stream, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-4",
		Payload: claudePayload,
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("claude"),
		Stream:       true,
	})
	if err != nil {
		t.Fatalf("ExecuteStream error: %v", err)
	}

	var all bytes.Buffer
	for chunk := range stream.Chunks {
		if chunk.Err != nil {
			t.Fatalf("stream chunk error: %v", chunk.Err)
		}
		all.Write(chunk.Payload)
	}

	if gotPath != "/v1/responses" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/responses")
	}
	if !gjson.GetBytes(gotBody, "input").Exists() {
		t.Fatalf("expected input in body")
	}
	if gjson.GetBytes(gotBody, "messages").Exists() {
		t.Fatalf("unexpected messages in body")
	}
	if !bytes.Contains(all.Bytes(), []byte("event: message_stop")) {
		t.Fatalf("expected claude stream message_stop event, got: %s", all.String())
	}
	if bytes.Contains(all.Bytes(), []byte("response.output_text.delta")) {
		t.Fatalf("expected translated claude stream, got raw responses events: %s", all.String())
	}
}

func TestOpenAICompatExecutorResponsesFallbackConvertsFunctionToolsForClaudeSource(t *testing.T) {
	var gotPath string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		gotBody = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_1","object":"response","output":[{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]}]}`))
	}))
	defer server.Close()

	cfg := &config.Config{
		OpenAICompatibility: []config.OpenAICompatibility{
			{
				Name:    "test-provider",
				WireAPI: config.OpenAIWireAPIResponses,
			},
		},
	}

	executor := NewOpenAICompatExecutor("openai-compatibility", cfg)
	auth := &cliproxyauth.Auth{
		Provider: "test-provider",
		Attributes: map[string]string{
			"base_url": server.URL + "/v1",
			"api_key":  "test",
		},
	}
	claudePayload := []byte(`{
		"model":"gpt-4",
		"max_tokens":128,
		"messages":[{"role":"user","content":"hello"}],
		"tools":[{"name":"Task","description":"run task","input_schema":{"type":"object","properties":{"prompt":{"type":"string"}}}}]
	}`)
	_, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-4",
		Payload: claudePayload,
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("claude"),
		Stream:       false,
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if gotPath != "/v1/responses" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/responses")
	}
	if got := gjson.GetBytes(gotBody, "tools.0.type").String(); got != "function" {
		t.Fatalf("tools.0.type = %q, want %q", got, "function")
	}
	if got := gjson.GetBytes(gotBody, "tools.0.name").String(); got != "Task" {
		t.Fatalf("tools.0.name = %q, want %q", got, "Task")
	}
	if gjson.GetBytes(gotBody, "tools.0.function").Exists() {
		t.Fatalf("unexpected tools.0.function in responses payload")
	}
	if !gjson.GetBytes(gotBody, "tools.0.parameters").Exists() {
		t.Fatalf("expected tools.0.parameters to exist")
	}
}
