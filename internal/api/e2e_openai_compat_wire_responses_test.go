//go:build e2e

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/tidwall/gjson"
)

func TestE2E_OpenAICompat_WireResponses_Chat(t *testing.T) {
	baseURL, apiKey, model := requireE2EOpenAICompatEnv(t)
	server, alias := setupE2EOpenAICompatServer(t, baseURL, apiKey, model)

	wantTool := envFlag("E2E_TOOL_CALL")
	wantJSON := strings.EqualFold(strings.TrimSpace(os.Getenv("E2E_RESPONSE_FORMAT")), "json_object")

	req := map[string]any{
		"model": alias,
		"messages": []map[string]any{
			{"role": "user", "content": "Say hi."},
		},
	}

	if wantTool {
		req["messages"] = []map[string]any{
			{"role": "user", "content": "Call the tool do with {\"a\":1} and do not add extra text."},
		}
		req["tools"] = []map[string]any{
			{
				"type": "function",
				"function": map[string]any{
					"name":        "do",
					"description": "test",
					"parameters": map[string]any{
						"type":       "object",
						"properties": map[string]any{"a": map[string]any{"type": "integer"}},
						"required":   []string{"a"},
					},
				},
			},
		}
		req["tool_choice"] = map[string]any{"type": "function", "function": map[string]any{"name": "do"}}
	} else if wantJSON {
		req["messages"] = []map[string]any{
			{"role": "user", "content": "Return a JSON object with key ok and value true. Do not include extra text."},
		}
		req["response_format"] = map[string]any{"type": "json_object"}
	}

	body := mustJSON(t, req)
	resp := doAuthedRequest(t, server, http.MethodPost, "/v1/chat/completions", body)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}

	payload := resp.Body.Bytes()
	if wantTool {
		if !gjson.GetBytes(payload, "choices.0.message.tool_calls").Exists() {
			t.Fatalf("expected tool_calls: %s", resp.Body.String())
		}
		return
	}
	if wantJSON {
		content := gjson.GetBytes(payload, "choices.0.message.content").String()
		var parsed map[string]any
		if err := json.Unmarshal([]byte(content), &parsed); err != nil {
			t.Fatalf("expected json content, got %q (err=%v)", content, err)
		}
		if v, ok := parsed["ok"].(bool); !ok || !v {
			t.Fatalf("expected ok=true, got %v", parsed)
		}
		return
	}
	if gjson.GetBytes(payload, "choices.0.message.role").String() != "assistant" {
		t.Fatalf("missing assistant role: %s", resp.Body.String())
	}
}

func TestE2E_OpenAICompat_WireResponses_Claude(t *testing.T) {
	baseURL, apiKey, model := requireE2EOpenAICompatEnv(t)
	server, alias := setupE2EOpenAICompatServer(t, baseURL, apiKey, model)

	req := map[string]any{
		"model":      alias,
		"max_tokens": 64,
		"stream":     true,
		"messages": []map[string]any{
			{"role": "user", "content": []map[string]any{{"type": "text", "text": "hi"}}},
		},
	}
	body := mustJSON(t, req)
	resp := doAuthedRequest(t, server, http.MethodPost, "/v1/messages", body)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}

	payload := resp.Body.String()
	if !strings.Contains(payload, "event: message_start") {
		t.Fatalf("missing message_start: %s", payload)
	}
	if !strings.Contains(payload, "event: content_block_delta") {
		t.Fatalf("missing content_block_delta: %s", payload)
	}
	if !strings.Contains(payload, "event: message_stop") {
		t.Fatalf("missing message_stop: %s", payload)
	}
}

func TestE2E_OpenAICompat_WireResponses_DirectResponses(t *testing.T) {
	baseURL, apiKey, model := requireE2EOpenAICompatEnv(t)
	server, alias := setupE2EOpenAICompatServer(t, baseURL, apiKey, model)

	req := map[string]any{
		"model": alias,
		"input": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": "hi"},
				},
			},
		},
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("E2E_RESPONSE_FORMAT")), "json_object") {
		req["response_format"] = map[string]any{"type": "json_object"}
	}

	body := mustJSON(t, req)
	resp := doAuthedRequest(t, server, http.MethodPost, "/v1/responses", body)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}

	payload := resp.Body.Bytes()
	if gjson.GetBytes(payload, "id").String() == "" {
		t.Fatalf("missing id: %s", resp.Body.String())
	}
	if !gjson.GetBytes(payload, "output").Exists() && !gjson.GetBytes(payload, "output_text").Exists() {
		t.Fatalf("missing output: %s", resp.Body.String())
	}
}

func requireE2EOpenAICompatEnv(t *testing.T) (string, string, string) {
	t.Helper()
	baseURL := strings.TrimSpace(os.Getenv("E2E_OPENAI_COMPAT_BASE_URL"))
	apiKey := strings.TrimSpace(os.Getenv("E2E_OPENAI_COMPAT_API_KEY"))
	model := strings.TrimSpace(os.Getenv("E2E_MODEL"))
	if baseURL == "" || apiKey == "" || model == "" {
		t.Skip("missing E2E_OPENAI_COMPAT_BASE_URL, E2E_OPENAI_COMPAT_API_KEY, or E2E_MODEL")
	}
	return baseURL, apiKey, model
}

func setupE2EOpenAICompatServer(t *testing.T, baseURL, apiKey, model string) (*Server, string) {
	t.Helper()
	server := newTestServer(t)
	alias := "e2e-model"
	server.cfg.OpenAICompatibility = []config.OpenAICompatibility{{
		Name:    "e2e",
		BaseURL: baseURL,
		WireAPI: "responses",
		APIKeyEntries: []config.OpenAICompatibilityAPIKey{{
			APIKey: apiKey,
		}},
		Models: []config.OpenAICompatibilityModel{{
			Name:  model,
			Alias: alias,
		}},
	}}
	server.applyAccessConfig(nil, server.cfg)
	return server, alias
}

func doAuthedRequest(t *testing.T, server *Server, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	server.engine.ServeHTTP(rr, req)
	return rr
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	return out
}

func envFlag(key string) bool {
	val := strings.TrimSpace(os.Getenv(key))
	return val == "1" || strings.EqualFold(val, "true") || strings.EqualFold(val, "yes")
}
