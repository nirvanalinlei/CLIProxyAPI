package test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/api/handlers/management"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/tidwall/gjson"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newOpenAICompatTestHandler(t *testing.T) (*management.Handler, string) {
	t.Helper()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	cfg := &config.Config{
		OpenAICompatibility: []config.OpenAICompatibility{
			{
				Name:    "test-provider",
				BaseURL: "https://example.com",
				APIKeyEntries: []config.OpenAICompatibilityAPIKey{
					{APIKey: "sk-test"},
				},
				Models: []config.OpenAICompatibilityModel{
					{Name: "gpt-4"},
				},
			},
		},
	}

	if err := os.WriteFile(configPath, []byte("port: 8080\n"), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	h := management.NewHandler(cfg, configPath, nil)
	return h, configPath
}

func setupOpenAICompatRouter(h *management.Handler) *gin.Engine {
	r := gin.New()
	mgmt := r.Group("/v0/management")
	{
		mgmt.GET("/openai-compatibility", h.GetOpenAICompat)
		mgmt.PATCH("/openai-compatibility", h.PatchOpenAICompat)
	}
	return r
}

func TestPatchOpenAICompatPersistsWireAPI(t *testing.T) {
	h, _ := newOpenAICompatTestHandler(t)
	r := setupOpenAICompatRouter(h)

	patchBody := map[string]any{
		"index": 0,
		"value": map[string]any{
			"wire-api": "responses",
		},
	}
	payload, err := json.Marshal(patchBody)
	if err != nil {
		t.Fatalf("failed to marshal patch body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPatch, "/v0/management/openai-compatibility", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v0/management/openai-compatibility", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	wireAPI := gjson.GetBytes(w.Body.Bytes(), "openai-compatibility.0.wire-api").String()
	if wireAPI != "responses" {
		t.Fatalf("wire-api = %q, want %q", wireAPI, "responses")
	}
}

func TestPatchOpenAICompatPersistsEnabled(t *testing.T) {
	h, _ := newOpenAICompatTestHandler(t)
	r := setupOpenAICompatRouter(h)

	patchBody := map[string]any{
		"index": 0,
		"value": map[string]any{
			"enabled": false,
		},
	}
	payload, err := json.Marshal(patchBody)
	if err != nil {
		t.Fatalf("failed to marshal patch body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPatch, "/v0/management/openai-compatibility", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v0/management/openai-compatibility", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	enabled := gjson.GetBytes(w.Body.Bytes(), "openai-compatibility.0.enabled")
	if !enabled.Exists() {
		t.Fatalf("expected enabled field to exist after disabling provider")
	}
	if enabled.Bool() {
		t.Fatalf("enabled = %t, want false", enabled.Bool())
	}
}
