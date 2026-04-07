package management

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

func TestPutOpenAICompatRejectsInvalidConcurrencyWithoutMutatingConfig(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	manager := coreauth.NewManager(nil, nil, nil)
	h := NewHandlerWithoutConfigFilePath(&config.Config{
		OpenAICompatibility: []config.OpenAICompatibility{
			{Name: "keep", BaseURL: "https://keep.example.com", Concurrency: 1},
		},
	}, manager)

	body := bytes.NewBufferString(`[
		{"name":"dup","base-url":"https://compat.example.com/v1","concurrency":2},
		{"name":" DUP ","base-url":"https://compat.example.com:443/v1/","concurrency":3}
	]`)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/v0/management/openai-compatibility", body)
	ctx.Request.Header.Set("Content-Type", "application/json")

	h.PutOpenAICompat(ctx)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(h.cfg.OpenAICompatibility) != 1 || h.cfg.OpenAICompatibility[0].Name != "keep" {
		t.Fatalf("config mutated on failed PUT: %#v", h.cfg.OpenAICompatibility)
	}
}

func TestPatchOpenAICompatRejectsInvalidConcurrencyWithoutMutatingConfig(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	manager := coreauth.NewManager(nil, nil, nil)
	h := NewHandlerWithoutConfigFilePath(&config.Config{
		OpenAICompatibility: []config.OpenAICompatibility{
			{Name: "first", BaseURL: "https://first.example.com", Concurrency: 1},
			{Name: "second", BaseURL: "https://second.example.com", Concurrency: 1},
		},
	}, manager)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPatch, "/v0/management/openai-compatibility", bytes.NewBufferString(`{
		"index": 1,
		"value": {
			"name": "first",
			"base-url": "https://first.example.com/",
			"concurrency": 5
		}
	}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	h.PatchOpenAICompat(ctx)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if got := h.cfg.OpenAICompatibility[1].Name; got != "second" {
		t.Fatalf("config mutated on failed PATCH, name=%q", got)
	}
	if got := h.cfg.OpenAICompatibility[1].BaseURL; got != "https://second.example.com" {
		t.Fatalf("config mutated on failed PATCH, base-url=%q", got)
	}
}

func TestPutOpenAICompatRollsBackOnPersistFailure(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	manager := coreauth.NewManager(nil, nil, nil)
	h := NewHandler(&config.Config{
		OpenAICompatibility: []config.OpenAICompatibility{
			{Name: "keep", BaseURL: "https://keep.example.com", Concurrency: 1},
		},
	}, t.TempDir(), manager)

	body := bytes.NewBufferString(`[
		{"name":"replace","base-url":"https://replace.example.com","concurrency":2}
	]`)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/v0/management/openai-compatibility", body)
	ctx.Request.Header.Set("Content-Type", "application/json")

	h.PutOpenAICompat(ctx)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(h.cfg.OpenAICompatibility) != 1 {
		t.Fatalf("config changed on persist failure: %#v", h.cfg.OpenAICompatibility)
	}
	if got := h.cfg.OpenAICompatibility[0].Name; got != "keep" {
		t.Fatalf("expected rollback to original name, got %q", got)
	}
	if got := h.cfg.OpenAICompatibility[0].BaseURL; got != "https://keep.example.com" {
		t.Fatalf("expected rollback to original base-url, got %q", got)
	}
}

func TestDeleteOpenAICompatRollsBackOnPersistFailure(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	manager := coreauth.NewManager(nil, nil, nil)
	h := NewHandler(&config.Config{
		OpenAICompatibility: []config.OpenAICompatibility{
			{Name: "first", BaseURL: "https://first.example.com"},
			{Name: "second", BaseURL: "https://second.example.com"},
		},
	}, t.TempDir(), manager)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/v0/management/openai-compatibility?index=0", nil)

	h.DeleteOpenAICompat(ctx)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(h.cfg.OpenAICompatibility) != 2 {
		t.Fatalf("config changed on persist failure: %#v", h.cfg.OpenAICompatibility)
	}
	if got := h.cfg.OpenAICompatibility[0].Name; got != "first" {
		t.Fatalf("expected rollback to original first entry, got %q", got)
	}
	if got := h.cfg.OpenAICompatibility[1].Name; got != "second" {
		t.Fatalf("expected rollback to original second entry, got %q", got)
	}
}

func TestPatchGeminiKeyRefreshesUpstreamConcurrencySnapshots(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	manager := coreauth.NewManager(nil, nil, nil)
	cfg := &config.Config{
		GeminiKey: []config.GeminiKey{
			{APIKey: "gem-key", BaseURL: "https://gem.example.com", Concurrency: 1},
		},
	}
	manager.SetConfig(cfg)

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("gemini-api-key:\n  - api-key: gem-key\n    base-url: https://gem.example.com\n    concurrency: 1\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	h := NewHandler(cfg, configPath, manager)

	patchRec := httptest.NewRecorder()
	patchCtx, _ := gin.CreateTestContext(patchRec)
	patchCtx.Request = httptest.NewRequest(http.MethodPatch, "/v0/management/gemini-api-key", bytes.NewBufferString(`{
		"index": 0,
		"value": {
			"concurrency": 7
		}
	}`))
	patchCtx.Request.Header.Set("Content-Type", "application/json")

	h.PatchGeminiKey(patchCtx)

	if patchRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", patchRec.Code, patchRec.Body.String())
	}

	snapshots := manager.LocalConcurrencySnapshots()
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d: %#v", len(snapshots), snapshots)
	}
	if snapshots[0].Cap != 7 {
		t.Fatalf("expected refreshed cap=7, got %#v", snapshots[0])
	}

	type upstreamConcurrencyItem struct {
		Cap int `json:"cap"`
	}
	var payload struct {
		Items []upstreamConcurrencyItem `json:"upstream-concurrency"`
	}
	getRec := httptest.NewRecorder()
	getCtx, _ := gin.CreateTestContext(getRec)
	getCtx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/upstream-concurrency", nil)

	h.GetUpstreamConcurrency(getCtx)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected GET status 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; body=%s", err, getRec.Body.String())
	}
	if len(payload.Items) != 1 || payload.Items[0].Cap != 7 {
		t.Fatalf("expected GET payload to reflect refreshed cap=7, got %#v", payload.Items)
	}
}

func TestGetUpstreamConcurrency(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	manager := coreauth.NewManager(nil, nil, nil)
	manager.SetConfig(&config.Config{
		GeminiKey: []config.GeminiKey{
			{APIKey: "gem-key", BaseURL: "https://gem.example.com", Concurrency: 2},
		},
	})
	if _, err := manager.Register(context.Background(), &coreauth.Auth{
		ID:       "auth-file-1",
		Provider: "codex",
		Label:    "readonly",
		Attributes: map[string]string{
			"concurrency_cap":             "4",
			"concurrency_scope_key":       "auth:auth-file-1",
			"concurrency_mode":            coreauth.LocalConcurrencyModeObserveOnly,
			"concurrency_reason":          coreauth.LocalConcurrencyReasonReadonlySource,
			"concurrency_source_kind":     coreauth.LocalConcurrencySourceAuthFile,
			"concurrency_schema_support":  coreauth.LocalConcurrencySchemaReadonly,
			"concurrency_identity_status": coreauth.LocalConcurrencyIdentityNotApplicable,
		},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	h := NewHandlerWithoutConfigFilePath(&config.Config{}, manager)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/upstream-concurrency", nil)

	h.GetUpstreamConcurrency(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	type upstreamConcurrencyItem struct {
		ScopeKey   string `json:"scope_key"`
		SourceKind string `json:"source_kind"`
		Mode       string `json:"mode"`
		Reason     string `json:"reason"`
		Cap        int    `json:"cap"`
		Generation uint64 `json:"generation"`
	}
	var payload struct {
		Items []upstreamConcurrencyItem `json:"upstream-concurrency"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; body=%s", err, rec.Body.String())
	}
	if len(payload.Items) != 2 {
		t.Fatalf("expected 2 items, got %d: %#v", len(payload.Items), payload.Items)
	}
	for _, item := range payload.Items {
		if item.Generation == 0 {
			t.Fatalf("expected generation > 0: %#v", item)
		}
	}
	if payload.Items[0].ScopeKey == "" || payload.Items[0].SourceKind == "" || payload.Items[0].Mode == "" || payload.Items[0].Reason == "" {
		t.Fatalf("snapshot missing required public fields: %#v", payload.Items[0])
	}
}
