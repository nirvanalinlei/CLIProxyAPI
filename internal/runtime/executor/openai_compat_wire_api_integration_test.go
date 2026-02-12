package executor

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	_ "github.com/router-for-me/CLIProxyAPI/v6/internal/translator"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
)

func TestOpenAICompatExecutor_WireResponses_StreamUsageChatShape(t *testing.T) {
	var gotPath string
	var gotBody []byte
	payload := strings.Join([]string{
		"data: {\"type\":\"response.output_item.added\",\"item\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\"},\"output_index\":0}",
		"data: {\"type\":\"response.output_text.delta\",\"item_id\":\"msg_1\",\"delta\":\"hi\",\"output_index\":0}",
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"created_at\":1,\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":2,\"total_tokens\":3}}}",
		"",
	}, "\n")
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(payload)),
		}, nil
	})

	cfg := &config.Config{OpenAICompatibility: []config.OpenAICompatibility{{Name: "p1", WireAPI: "responses"}}}
	executor := NewOpenAICompatExecutor("openai-compatibility", cfg)
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url":    "http://example.test/v1",
		"api_key":     "k",
		"compat_name": "p1",
	}}
	reqPayload := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hi"}],"stream":true,"stream_options":{"include_usage":true}}`)
	ctx := context.WithValue(context.Background(), "cliproxy.roundtripper", rt)
	stream, err := executor.ExecuteStream(ctx, auth, cliproxyexecutor.Request{
		Model:   "gpt-4",
		Payload: reqPayload,
	}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai"), Stream: true})
	if err != nil {
		t.Fatalf("ExecuteStream error: %v", err)
	}
	var chunks []string
	for c := range stream {
		if c.Err != nil {
			t.Fatalf("stream err: %v", c.Err)
		}
		chunks = append(chunks, string(c.Payload))
	}
	joined := strings.Join(chunks, "\n")
	if !strings.Contains(joined, "\"prompt_tokens\"") {
		t.Fatalf("expected chat usage shape: %s", joined)
	}
	if gotPath != "/v1/responses" {
		t.Fatalf("path = %q", gotPath)
	}
	if !strings.Contains(string(gotBody), "\"input\"") {
		t.Fatalf("expected responses payload: %s", string(gotBody))
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
