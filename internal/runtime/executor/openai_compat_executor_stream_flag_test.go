package executor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	_ "github.com/router-for-me/CLIProxyAPI/v6/internal/translator"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
	"github.com/tidwall/gjson"
)

func TestOpenAICompatExecutor_ChatCompletionsExecuteSetsStreamFalse(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_1","object":"chat.completion","created":1,"model":"gpt-4","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer srv.Close()

	executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": srv.URL + "/v1",
		"api_key":  "k",
	}}

	_, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-4",
		Payload: []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`),
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
		Stream:       false,
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !gjson.GetBytes(gotBody, "stream").Exists() {
		t.Fatalf("expected stream field in upstream payload: %s", string(gotBody))
	}
	if gjson.GetBytes(gotBody, "stream").Bool() {
		t.Fatalf("expected stream=false in upstream payload: %s", string(gotBody))
	}
}

func TestOpenAICompatExecutor_ChatCompletionsExecuteStreamSetsStreamTrue(t *testing.T) {
	var gotBody []byte
	payload := strings.Join([]string{
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		"data: [DONE]",
		"",
	}, "\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": srv.URL + "/v1",
		"api_key":  "k",
	}}

	stream, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-4",
		Payload: []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`),
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
		Stream:       true,
	})
	if err != nil {
		t.Fatalf("ExecuteStream error: %v", err)
	}
	for c := range stream {
		if c.Err != nil {
			t.Fatalf("stream err: %v", c.Err)
		}
	}
	if !gjson.GetBytes(gotBody, "stream").Exists() {
		t.Fatalf("expected stream field in upstream payload: %s", string(gotBody))
	}
	if !gjson.GetBytes(gotBody, "stream").Bool() {
		t.Fatalf("expected stream=true in upstream payload: %s", string(gotBody))
	}
}
