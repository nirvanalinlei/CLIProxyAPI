package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	proxyconfig "github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

func newUpstreamConcurrencyTestServer(t *testing.T) *Server {
	t.Helper()
	t.Setenv("MANAGEMENT_PASSWORD", "secret-123")
	return newTestServer(t)
}

func TestManagementUpstreamConcurrencyRoute(t *testing.T) {
	server := newUpstreamConcurrencyTestServer(t)
	manager := coreauth.NewManager(nil, nil, nil)
	manager.SetConfig(&proxyconfig.Config{
		GeminiKey: []proxyconfig.GeminiKey{
			{APIKey: "gem-key", BaseURL: "https://gem.example.com", Concurrency: 2},
		},
	})
	server.mgmt.SetAuthManager(manager)

	req := httptest.NewRequest(http.MethodGet, "/v0/management/upstream-concurrency", nil)
	req.Header.Set("Authorization", "Bearer secret-123")
	rr := httptest.NewRecorder()

	server.engine.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: got %d body=%s", rr.Code, rr.Body.String())
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
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v; body=%s", err, rr.Body.String())
	}
	if len(payload.Items) != 1 {
		t.Fatalf("expected 1 item, got %d: %#v", len(payload.Items), payload.Items)
	}
	if payload.Items[0].SourceKind != coreauth.LocalConcurrencySourceGeminiAPIKey {
		t.Fatalf("source_kind = %q", payload.Items[0].SourceKind)
	}
	if payload.Items[0].ScopeKey == "" || payload.Items[0].Cap != 2 || payload.Items[0].Mode != coreauth.LocalConcurrencyModeObserveOnly {
		t.Fatalf("unexpected snapshot public contract: %#v", payload.Items[0])
	}
	if payload.Items[0].Reason != coreauth.LocalConcurrencyReasonObserveOnlyPhase {
		t.Fatalf("reason = %q", payload.Items[0].Reason)
	}
}
