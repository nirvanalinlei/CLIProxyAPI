package logging

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tidwall/gjson"
)

func TestExtractUsageMetricsFromStreamingPayload(t *testing.T) {
	payload := []byte("event: response.completed\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":40,\"output_tokens\":60,\"total_tokens\":100,\"input_tokens_details\":{\"cached_tokens\":8},\"output_tokens_details\":{\"reasoning_tokens\":5}}}}\n\n")

	metrics, source := extractUsageMetrics(payload, nil)
	if source != "response" {
		t.Fatalf("expected response source, got %q", source)
	}
	if metrics == nil {
		t.Fatal("expected usage metrics, got nil")
	}
	assertMetricValue(t, metrics.InputTokens, 40, "input_tokens")
	assertMetricValue(t, metrics.OutputTokens, 60, "output_tokens")
	assertMetricValue(t, metrics.TotalTokens, 100, "total_tokens")
	assertMetricValue(t, metrics.CachedInputTokens, 8, "cached_input_tokens")
	assertMetricValue(t, metrics.ReasoningTokens, 5, "reasoning_tokens")
}

func TestExtractUsageMetricsFallsBackToAPIResponse(t *testing.T) {
	apiResponse := []byte("=== API RESPONSE 1 ===\n" +
		"Timestamp: 2026-04-02T00:00:00Z\n\n" +
		"Headers:\n" +
		"Content-Type: application/json\n\n" +
		"Body:\n" +
		"{\"response\":{\"usage\":{\"input_tokens\":30,\"output_tokens\":70,\"total_tokens\":100}}}\n")

	metrics, source := extractUsageMetrics(nil, apiResponse)
	if source != "api_response" {
		t.Fatalf("expected api_response source, got %q", source)
	}
	if metrics == nil {
		t.Fatal("expected usage metrics, got nil")
	}
	assertMetricValue(t, metrics.TotalTokens, 100, "total_tokens")
}

func TestExtractAuthInfoFromAPIRequest(t *testing.T) {
	apiRequest := []byte("=== API REQUEST 1 ===\nAuth: provider=codex, auth_id=acc-1, label=primary plus, type=oauth\n\n")
	info := extractAuthInfoFromAPIRequest(apiRequest)
	if info.Provider != "codex" {
		t.Fatalf("expected provider=codex, got %q", info.Provider)
	}
	if info.AuthID != "acc-1" {
		t.Fatalf("expected auth_id=acc-1, got %q", info.AuthID)
	}
	if info.AuthLabel != "primary plus" {
		t.Fatalf("expected auth_label='primary plus', got %q", info.AuthLabel)
	}
	if info.AuthType != "oauth" {
		t.Fatalf("expected auth_type=oauth, got %q", info.AuthType)
	}
}

func TestFileRequestLoggerWritesUsageSummarySidecar(t *testing.T) {
	dir := t.TempDir()
	logger := NewFileRequestLogger(true, dir, "", 0)

	responseHeaders := map[string][]string{
		"X-CLIProxy-Plan-Type":                {"plus"},
		"X-CLIProxy-Active-Limit":             {"5h"},
		"X-CLIProxy-Primary-Used-Percent":     {"74"},
		"X-CLIProxy-Secondary-Used-Percent":   {"23"},
		"X-CLIProxy-Primary-Window-Minutes":   {"300"},
		"X-CLIProxy-Secondary-Window-Minutes": {"10080"},
	}
	requestBody := []byte(`{"model":"gpt-5.4"}`)
	responseBody := []byte(`{"id":"resp_1","usage":{"input_tokens":40,"output_tokens":60,"total_tokens":100}}`)
	apiRequest := []byte("=== API REQUEST 1 ===\nAuth: provider=codex, auth_id=acc-plus, label=plus account, type=oauth\n\n")

	err := logger.LogRequest(
		"/v1/responses",
		"POST",
		nil,
		requestBody,
		200,
		responseHeaders,
		responseBody,
		apiRequest,
		nil,
		nil,
		"req-sync",
		time.Unix(100, 0),
		time.Unix(101, 0),
	)
	if err != nil {
		t.Fatalf("LogRequest returned error: %v", err)
	}

	usageFile := mustFindUsageFile(t, dir, "*req-sync.usage.json")
	content, err := os.ReadFile(usageFile)
	if err != nil {
		t.Fatalf("read usage file: %v", err)
	}
	assertJSONInt(t, content, "total_tokens", 100)
	assertJSONString(t, content, "model", "gpt-5.4")
	assertJSONString(t, content, "plan_type", "plus")
	assertJSONString(t, content, "provider", "codex")
	assertJSONString(t, content, "auth_id", "acc-plus")
	assertJSONString(t, content, "auth_label", "plus account")
	assertJSONString(t, content, "auth_type", "oauth")
	assertJSONString(t, content, "usage_source", "response")
}

func TestFileStreamingLogWriterWritesUsageSummarySidecar(t *testing.T) {
	dir := t.TempDir()
	logger := NewFileRequestLogger(true, dir, "", 0)

	writer, err := logger.LogStreamingRequest(
		"/v1/responses",
		"POST",
		nil,
		[]byte(`{"model":"gpt-5.4","stream":true}`),
		"req-stream",
	)
	if err != nil {
		t.Fatalf("LogStreamingRequest returned error: %v", err)
	}

	if err := writer.WriteAPIRequest([]byte("=== API REQUEST 1 ===\nAuth: provider=codex, auth_id=acc-pro, label=pro account, type=oauth\n\n")); err != nil {
		t.Fatalf("WriteAPIRequest returned error: %v", err)
	}

	err = writer.WriteStatus(200, map[string][]string{
		"X-CLIProxy-Plan-Type": {"pro"},
	})
	if err != nil {
		t.Fatalf("WriteStatus returned error: %v", err)
	}
	writer.WriteChunkAsync([]byte("event: response.completed\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":100,\"output_tokens\":150,\"total_tokens\":250}}}\n\n"))
	writer.SetFirstChunkTimestamp(time.Unix(201, 0))

	if err := writer.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	usageFile := mustFindUsageFile(t, dir, "*req-stream.usage.json")
	content, err := os.ReadFile(usageFile)
	if err != nil {
		t.Fatalf("read usage file: %v", err)
	}
	assertJSONInt(t, content, "total_tokens", 250)
	assertJSONString(t, content, "plan_type", "pro")
	assertJSONString(t, content, "provider", "codex")
	assertJSONString(t, content, "auth_id", "acc-pro")
	assertJSONString(t, content, "auth_label", "pro account")
	assertJSONString(t, content, "auth_type", "oauth")
	assertJSONString(t, content, "model", "gpt-5.4")
}

func mustFindUsageFile(t *testing.T, dir string, pattern string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		t.Fatalf("glob usage file: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected exactly one usage file for pattern %q, got %d", pattern, len(matches))
	}
	return matches[0]
}

func assertMetricValue(t *testing.T, value *int64, want int64, name string) {
	t.Helper()
	if value == nil {
		t.Fatalf("expected %s, got nil", name)
	}
	if *value != want {
		t.Fatalf("expected %s=%d, got %d", name, want, *value)
	}
}

func assertJSONInt(t *testing.T, content []byte, path string, want int64) {
	t.Helper()
	got := gjson.GetBytes(content, path)
	if !got.Exists() {
		t.Fatalf("expected json path %q to exist", path)
	}
	if got.Int() != want {
		t.Fatalf("expected %s=%d, got %d", path, want, got.Int())
	}
}

func assertJSONString(t *testing.T, content []byte, path string, want string) {
	t.Helper()
	got := gjson.GetBytes(content, path)
	if !got.Exists() {
		t.Fatalf("expected json path %q to exist", path)
	}
	if got.String() != want {
		t.Fatalf("expected %s=%q, got %q", path, want, got.String())
	}
}
