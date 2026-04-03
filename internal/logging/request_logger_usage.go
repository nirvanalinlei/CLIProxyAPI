package logging

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/tidwall/gjson"
)

const usageSummaryTailReadBytes int64 = 4 * 1024 * 1024

type requestUsageSummary struct {
	Version                int        `json:"version"`
	GeneratedAt            time.Time  `json:"generated_at"`
	RequestTimestamp       *time.Time `json:"request_timestamp,omitempty"`
	APIResponseTimestamp   *time.Time `json:"api_response_timestamp,omitempty"`
	URL                    string     `json:"url,omitempty"`
	Method                 string     `json:"method,omitempty"`
	LogFile                string     `json:"log_file,omitempty"`
	StatusCode             int        `json:"status_code,omitempty"`
	Provider               string     `json:"provider,omitempty"`
	AuthID                 string     `json:"auth_id,omitempty"`
	AuthLabel              string     `json:"auth_label,omitempty"`
	AuthType               string     `json:"auth_type,omitempty"`
	Model                  string     `json:"model,omitempty"`
	PlanType               string     `json:"plan_type,omitempty"`
	ActiveLimit            string     `json:"active_limit,omitempty"`
	PrimaryUsedPercent     *int64     `json:"primary_used_percent,omitempty"`
	SecondaryUsedPercent   *int64     `json:"secondary_used_percent,omitempty"`
	PrimaryWindowMinutes   *int64     `json:"primary_window_minutes,omitempty"`
	SecondaryWindowMinutes *int64     `json:"secondary_window_minutes,omitempty"`
	PrimaryResetAt         string     `json:"primary_reset_at,omitempty"`
	SecondaryResetAt       string     `json:"secondary_reset_at,omitempty"`
	UsageFound             bool       `json:"usage_found"`
	UsageSource            string     `json:"usage_source,omitempty"`
	InputTokens            *int64     `json:"input_tokens,omitempty"`
	OutputTokens           *int64     `json:"output_tokens,omitempty"`
	TotalTokens            *int64     `json:"total_tokens,omitempty"`
	CachedInputTokens      *int64     `json:"cached_input_tokens,omitempty"`
	ReasoningTokens        *int64     `json:"reasoning_tokens,omitempty"`
}

type usageMetrics struct {
	InputTokens       *int64
	OutputTokens      *int64
	TotalTokens       *int64
	CachedInputTokens *int64
	ReasoningTokens   *int64
}

type requestAuthInfo struct {
	Provider  string
	AuthID    string
	AuthLabel string
	AuthType  string
}

func writeUsageSummaryFile(
	logFilePath, url, method string,
	requestBody []byte,
	requestBodyPath string,
	statusCode int,
	responseHeaders map[string][]string,
	responseBody []byte,
	responseBodyPath string,
	apiRequest []byte,
	apiResponse []byte,
	requestTimestamp, apiResponseTimestamp time.Time,
) error {
	if strings.TrimSpace(logFilePath) == "" {
		return nil
	}

	summary := buildUsageSummary(
		logFilePath,
		url,
		method,
		requestBody,
		requestBodyPath,
		statusCode,
		responseHeaders,
		responseBody,
		responseBodyPath,
		apiRequest,
		apiResponse,
		requestTimestamp,
		apiResponseTimestamp,
	)

	encoded, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	return os.WriteFile(usageSummaryPath(logFilePath), encoded, 0o644)
}

func buildUsageSummary(
	logFilePath, url, method string,
	requestBody []byte,
	requestBodyPath string,
	statusCode int,
	responseHeaders map[string][]string,
	responseBody []byte,
	responseBodyPath string,
	apiRequest []byte,
	apiResponse []byte,
	requestTimestamp, apiResponseTimestamp time.Time,
) requestUsageSummary {
	if len(requestBody) == 0 && requestBodyPath != "" {
		requestBody = readFileAllQuiet(requestBodyPath)
	}
	if len(responseBody) == 0 && responseBodyPath != "" {
		responseBody = readFileTailQuiet(responseBodyPath, usageSummaryTailReadBytes)
	}

	authInfo := extractAuthInfoFromAPIRequest(apiRequest)
	metrics, source := extractUsageMetrics(responseBody, apiResponse)
	summary := requestUsageSummary{
		Version:                1,
		GeneratedAt:            time.Now().UTC(),
		RequestTimestamp:       timePtr(requestTimestamp),
		APIResponseTimestamp:   timePtr(apiResponseTimestamp),
		URL:                    url,
		Method:                 method,
		LogFile:                filepath.Base(logFilePath),
		StatusCode:             statusCode,
		Provider:               authInfo.Provider,
		AuthID:                 authInfo.AuthID,
		AuthLabel:              authInfo.AuthLabel,
		AuthType:               authInfo.AuthType,
		Model:                  extractModelFromRequestBody(requestBody),
		PlanType:               getHeaderValue(responseHeaders, "plan_type", "plan-type", "x-plan-type", "x-cliproxy-plan-type", "x-cli-proxy-plan-type", "x-codex-plan-type"),
		ActiveLimit:            getHeaderValue(responseHeaders, "active_limit", "active-limit", "x-active-limit", "x-cliproxy-active-limit", "x-cli-proxy-active-limit"),
		PrimaryUsedPercent:     parseHeaderInt64(responseHeaders, "primary_used_percent", "primary-used-percent", "x-primary-used-percent", "x-cliproxy-primary-used-percent", "x-cli-proxy-primary-used-percent"),
		SecondaryUsedPercent:   parseHeaderInt64(responseHeaders, "secondary_used_percent", "secondary-used-percent", "x-secondary-used-percent", "x-cliproxy-secondary-used-percent", "x-cli-proxy-secondary-used-percent"),
		PrimaryWindowMinutes:   parseHeaderInt64(responseHeaders, "primary_window_minutes", "primary-window-minutes", "x-primary-window-minutes", "x-cliproxy-primary-window-minutes", "x-cli-proxy-primary-window-minutes"),
		SecondaryWindowMinutes: parseHeaderInt64(responseHeaders, "secondary_window_minutes", "secondary-window-minutes", "x-secondary-window-minutes", "x-cliproxy-secondary-window-minutes", "x-cli-proxy-secondary-window-minutes"),
		PrimaryResetAt:         normalizeResetAt(getHeaderValue(responseHeaders, "primary_reset_at", "primary-reset-at", "x-primary-reset-at", "x-cliproxy-primary-reset-at", "x-cli-proxy-primary-reset-at")),
		SecondaryResetAt:       normalizeResetAt(getHeaderValue(responseHeaders, "secondary_reset_at", "secondary-reset-at", "x-secondary-reset-at", "x-cliproxy-secondary-reset-at", "x-cli-proxy-secondary-reset-at")),
		UsageFound:             metrics != nil,
		UsageSource:            source,
	}
	if metrics != nil {
		summary.InputTokens = metrics.InputTokens
		summary.OutputTokens = metrics.OutputTokens
		summary.TotalTokens = metrics.TotalTokens
		summary.CachedInputTokens = metrics.CachedInputTokens
		summary.ReasoningTokens = metrics.ReasoningTokens
	}

	return summary
}

func usageSummaryPath(logFilePath string) string {
	if strings.HasSuffix(strings.ToLower(logFilePath), ".log") {
		return logFilePath[:len(logFilePath)-len(".log")] + ".usage.json"
	}
	return logFilePath + ".usage.json"
}

func extractUsageMetrics(responseBody, apiResponse []byte) (*usageMetrics, string) {
	if metrics := extractUsageMetricsFromPayload(responseBody); metrics != nil {
		return metrics, "response"
	}
	if metrics := extractUsageMetricsFromAPIResponse(apiResponse); metrics != nil {
		return metrics, "api_response"
	}
	return nil, ""
}

func extractUsageMetricsFromPayload(payload []byte) *usageMetrics {
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 {
		return nil
	}
	if metrics := extractUsageMetricsFromJSON(payload); metrics != nil {
		return metrics
	}
	return extractUsageMetricsFromSSE(payload)
}

func extractUsageMetricsFromSSE(payload []byte) *usageMetrics {
	var last *usageMetrics
	for _, line := range bytes.Split(payload, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		data := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(data) == 0 || bytes.Equal(data, []byte("[DONE]")) {
			continue
		}
		if metrics := extractUsageMetricsFromJSON(data); metrics != nil {
			last = metrics
		}
	}
	return last
}

func extractUsageMetricsFromJSON(payload []byte) *usageMetrics {
	if !gjson.ValidBytes(payload) {
		return nil
	}

	root := gjson.ParseBytes(payload)
	candidates := []gjson.Result{
		root.Get("usage"),
		root.Get("response.usage"),
		root.Get("item.usage"),
		root.Get("usageMetadata"),
		root.Get("response.usageMetadata"),
		root,
	}
	for _, candidate := range candidates {
		if metrics := extractOpenAIUsageMetrics(candidate); metrics != nil {
			return metrics
		}
		if metrics := extractGeminiUsageMetrics(candidate); metrics != nil {
			return metrics
		}
	}
	return nil
}

func extractUsageMetricsFromAPIResponse(apiResponse []byte) *usageMetrics {
	apiResponse = bytes.TrimSpace(apiResponse)
	if len(apiResponse) == 0 {
		return nil
	}
	if metrics := extractUsageMetricsFromPayload(apiResponse); metrics != nil {
		return metrics
	}

	var last *usageMetrics
	for _, body := range splitAPIResponseBodies(apiResponse) {
		if metrics := extractUsageMetricsFromPayload(body); metrics != nil {
			last = metrics
		}
	}
	return last
}

func splitAPIResponseBodies(apiResponse []byte) [][]byte {
	var (
		bodies    [][]byte
		current   bytes.Buffer
		capturing bool
	)
	flush := func() {
		body := bytes.TrimSpace(current.Bytes())
		if len(body) > 0 {
			bodies = append(bodies, bytes.Clone(body))
		}
		current.Reset()
	}

	for _, rawLine := range bytes.Split(apiResponse, []byte("\n")) {
		line := strings.TrimSpace(string(bytes.TrimRight(rawLine, "\r")))
		switch {
		case strings.HasPrefix(line, "=== API RESPONSE") || strings.HasPrefix(line, "=== API REQUEST") || strings.HasPrefix(line, "=== API ERROR RESPONSE"):
			if capturing {
				flush()
				capturing = false
			}
		case line == "Body:":
			if capturing {
				flush()
			}
			capturing = true
		default:
			if !capturing {
				continue
			}
			if current.Len() > 0 {
				current.WriteByte('\n')
			}
			current.Write(bytes.TrimRight(rawLine, "\r"))
		}
	}
	if capturing {
		flush()
	}
	return bodies
}

func extractAuthInfoFromAPIRequest(apiRequest []byte) requestAuthInfo {
	for _, rawLine := range bytes.Split(apiRequest, []byte("\n")) {
		line := strings.TrimSpace(string(bytes.TrimRight(rawLine, "\r")))
		if !strings.HasPrefix(line, "Auth:") {
			continue
		}
		return parseAuthInfoLine(strings.TrimSpace(strings.TrimPrefix(line, "Auth:")))
	}
	return requestAuthInfo{}
}

func parseAuthInfoLine(line string) requestAuthInfo {
	return requestAuthInfo{
		Provider:  extractDelimitedField(line, "provider", false),
		AuthID:    extractDelimitedField(line, "auth_id", false),
		AuthLabel: extractDelimitedField(line, "label", false),
		AuthType:  extractDelimitedField(line, "type", true),
	}
}

func extractDelimitedField(line, key string, stopAtWhitespace bool) string {
	lowerLine := strings.ToLower(line)
	token := strings.ToLower(key) + "="
	start := strings.Index(lowerLine, token)
	if start == -1 {
		return ""
	}
	start += len(token)
	end := len(line)
	if comma := strings.Index(lowerLine[start:], ","); comma != -1 {
		end = start + comma
	}
	if stopAtWhitespace {
		if ws := indexWhitespace(line[start:end]); ws != -1 {
			end = start + ws
		}
	}
	return strings.TrimSpace(line[start:end])
}

func indexWhitespace(value string) int {
	for idx, r := range value {
		if unicode.IsSpace(r) {
			return idx
		}
	}
	return -1
}

func extractOpenAIUsageMetrics(result gjson.Result) *usageMetrics {
	if !result.Exists() {
		return nil
	}
	input := tokenCountPtrFromResult(result.Get("input_tokens"))
	if input == nil {
		input = tokenCountPtrFromResult(result.Get("prompt_tokens"))
	}
	output := tokenCountPtrFromResult(result.Get("output_tokens"))
	if output == nil {
		output = tokenCountPtrFromResult(result.Get("completion_tokens"))
	}
	total := tokenCountPtrFromResult(result.Get("total_tokens"))
	cached := tokenCountPtrFromResult(result.Get("input_tokens_details.cached_tokens"))
	if cached == nil {
		cached = tokenCountPtrFromResult(result.Get("prompt_tokens_details.cached_tokens"))
	}
	reasoning := tokenCountPtrFromResult(result.Get("output_tokens_details.reasoning_tokens"))
	if reasoning == nil {
		reasoning = tokenCountPtrFromResult(result.Get("completion_tokens_details.reasoning_tokens"))
	}
	return metricsFromCounts(input, output, total, cached, reasoning)
}

func extractGeminiUsageMetrics(result gjson.Result) *usageMetrics {
	if !result.Exists() {
		return nil
	}
	input := tokenCountPtrFromResult(result.Get("promptTokenCount"))
	output := tokenCountPtrFromResult(result.Get("candidatesTokenCount"))
	total := tokenCountPtrFromResult(result.Get("totalTokenCount"))
	cached := tokenCountPtrFromResult(result.Get("cachedContentTokenCount"))
	reasoning := tokenCountPtrFromResult(result.Get("thoughtsTokenCount"))
	return metricsFromCounts(input, output, total, cached, reasoning)
}

func metricsFromCounts(input, output, total, cached, reasoning *int64) *usageMetrics {
	if total == nil && input != nil && output != nil {
		sum := *input + *output
		total = &sum
	}
	metrics := &usageMetrics{
		InputTokens:       input,
		OutputTokens:      output,
		TotalTokens:       total,
		CachedInputTokens: cached,
		ReasoningTokens:   reasoning,
	}
	if !hasUsageMetrics(metrics) {
		return nil
	}
	return metrics
}

func hasUsageMetrics(metrics *usageMetrics) bool {
	if metrics == nil {
		return false
	}
	return metrics.InputTokens != nil || metrics.OutputTokens != nil || metrics.TotalTokens != nil || metrics.CachedInputTokens != nil || metrics.ReasoningTokens != nil
}

func tokenCountPtrFromResult(result gjson.Result) *int64 {
	if !result.Exists() {
		return nil
	}
	value := strings.TrimSpace(result.String())
	if value == "" && result.Type != gjson.Number {
		return nil
	}
	return parseOptionalInt64(value)
}

func extractModelFromRequestBody(body []byte) string {
	body = bytes.TrimSpace(body)
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return ""
	}
	for _, path := range []string{"model", "request.model"} {
		if value := strings.TrimSpace(gjson.GetBytes(body, path).String()); value != "" {
			return value
		}
	}
	return ""
}

func getHeaderValue(headers map[string][]string, aliases ...string) string {
	if len(headers) == 0 {
		return ""
	}
	lookup := make(map[string]string, len(headers))
	for key, values := range headers {
		if len(values) == 0 {
			continue
		}
		value := strings.TrimSpace(values[0])
		if value == "" {
			continue
		}
		lookup[normalizeHeaderKey(key)] = value
	}
	for _, alias := range aliases {
		if value := lookup[normalizeHeaderKey(alias)]; value != "" {
			return value
		}
	}
	return ""
}

func normalizeHeaderKey(key string) string {
	var builder strings.Builder
	builder.Grow(len(key))
	for _, r := range strings.ToLower(key) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func parseHeaderInt64(headers map[string][]string, aliases ...string) *int64 {
	return parseOptionalInt64(getHeaderValue(headers, aliases...))
}

func parseOptionalInt64(value string) *int64 {
	value = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), "%"))
	if value == "" {
		return nil
	}
	if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
		return &parsed
	}
	if parsed, err := strconv.ParseFloat(value, 64); err == nil {
		rounded := int64(parsed)
		return &rounded
	}
	return nil
}

func normalizeResetAt(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
		switch {
		case parsed > 1_000_000_000_000:
			return time.UnixMilli(parsed).UTC().Format(time.RFC3339)
		case parsed > 1_000_000_000:
			return time.Unix(parsed, 0).UTC().Format(time.RFC3339)
		}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC().Format(time.RFC3339)
		}
	}
	return value
}

func readFileAllQuiet(path string) []byte {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return content
}

func readFileTailQuiet(path string, maxBytes int64) []byte {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if maxBytes <= 0 {
		return readFileAllQuiet(path)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() {
		_ = file.Close()
	}()

	info, err := file.Stat()
	if err != nil {
		return nil
	}
	if info.Size() <= maxBytes {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		return content
	}

	start := info.Size() - maxBytes
	if _, err := file.Seek(start, 0); err != nil {
		return nil
	}
	content, err := io.ReadAll(file)
	if err != nil {
		return nil
	}
	return content
}

func timePtr(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	copied := value.UTC()
	return &copied
}
