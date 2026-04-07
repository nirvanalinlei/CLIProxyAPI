package synthesizer

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

func TestConfigSynthesizer_ConcurrencyAttrs(t *testing.T) {
	t.Parallel()

	synth := NewConfigSynthesizer()
	ctx := &SynthesisContext{
		Config: &config.Config{
			GeminiKey: []config.GeminiKey{
				{
					APIKey:      "gem-key",
					BaseURL:     "https://gem.example.com",
					Concurrency: 2,
				},
			},
			OpenAICompatibility: []config.OpenAICompatibility{
				{
					Name:        "",
					BaseURL:     "https://compat.example.com",
					Concurrency: 4,
					APIKeyEntries: []config.OpenAICompatibilityAPIKey{
						{APIKey: "compat-key"},
					},
				},
			},
		},
		Now:         time.Date(2026, 4, 7, 0, 0, 0, 0, time.UTC),
		IDGenerator: NewStableIDGenerator(),
	}

	auths, err := synth.Synthesize(ctx)
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if len(auths) != 2 {
		t.Fatalf("expected 2 auths, got %d", len(auths))
	}

	gemini := auths[0]
	if gemini.Attributes["concurrency_cap"] != "2" {
		t.Fatalf("gemini concurrency_cap = %q, want 2", gemini.Attributes["concurrency_cap"])
	}
	if gemini.Attributes["concurrency_mode"] != coreauth.LocalConcurrencyModeObserveOnly {
		t.Fatalf("gemini concurrency_mode = %q", gemini.Attributes["concurrency_mode"])
	}
	if gemini.Attributes["concurrency_reason"] != coreauth.LocalConcurrencyReasonObserveOnlyPhase {
		t.Fatalf("gemini concurrency_reason = %q", gemini.Attributes["concurrency_reason"])
	}
	if gemini.Attributes["concurrency_scope_key"] != "auth:"+gemini.ID {
		t.Fatalf("gemini concurrency_scope_key = %q, want auth:%s", gemini.Attributes["concurrency_scope_key"], gemini.ID)
	}

	compat := auths[1]
	if compat.Attributes["concurrency_mode"] != coreauth.LocalConcurrencyModeDisabled {
		t.Fatalf("compat concurrency_mode = %q", compat.Attributes["concurrency_mode"])
	}
	if compat.Attributes["concurrency_reason"] != coreauth.LocalConcurrencyReasonInvalidScopeIdentity {
		t.Fatalf("compat concurrency_reason = %q", compat.Attributes["concurrency_reason"])
	}
	if compat.Attributes["concurrency_identity_status"] != coreauth.LocalConcurrencyIdentityInvalid {
		t.Fatalf("compat concurrency_identity_status = %q", compat.Attributes["concurrency_identity_status"])
	}
}

func TestSynthesizeAuthFile_ConcurrencyAttrs(t *testing.T) {
	t.Parallel()

	authDir := t.TempDir()
	fullPath := filepath.Join(authDir, "user.json")
	payload, err := json.Marshal(map[string]any{
		"type":        "codex",
		"email":       "user@example.com",
		"concurrency": 4,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	auths := SynthesizeAuthFile(&SynthesisContext{
		Config:      &config.Config{AuthDir: authDir},
		AuthDir:     authDir,
		Now:         time.Date(2026, 4, 7, 0, 0, 0, 0, time.UTC),
		IDGenerator: NewStableIDGenerator(),
	}, fullPath, payload)
	if len(auths) != 1 {
		t.Fatalf("expected 1 auth, got %d", len(auths))
	}

	auth := auths[0]
	if auth.Attributes["concurrency_cap"] != "4" {
		t.Fatalf("concurrency_cap = %q, want 4", auth.Attributes["concurrency_cap"])
	}
	if auth.Attributes["concurrency_source_kind"] != coreauth.LocalConcurrencySourceAuthFile {
		t.Fatalf("concurrency_source_kind = %q", auth.Attributes["concurrency_source_kind"])
	}
	if auth.Attributes["concurrency_reason"] != coreauth.LocalConcurrencyReasonReadonlySource {
		t.Fatalf("concurrency_reason = %q", auth.Attributes["concurrency_reason"])
	}
	if auth.Attributes["concurrency_scope_key"] != "auth:"+auth.ID {
		t.Fatalf("concurrency_scope_key = %q, want auth:%s", auth.Attributes["concurrency_scope_key"], auth.ID)
	}
}

func TestSynthesizeAuthFile_GeminiVirtualAuthsPropagateConcurrencyAttrs(t *testing.T) {
	t.Parallel()

	authDir := t.TempDir()
	fullPath := filepath.Join(authDir, "gemini.json")
	payload, err := json.Marshal(map[string]any{
		"type":        "gemini",
		"email":       "gemini@example.com",
		"project_id":  "project-a,project-b",
		"concurrency": 2,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	auths := SynthesizeAuthFile(&SynthesisContext{
		Config:      &config.Config{AuthDir: authDir},
		AuthDir:     authDir,
		Now:         time.Date(2026, 4, 7, 0, 0, 0, 0, time.UTC),
		IDGenerator: NewStableIDGenerator(),
	}, fullPath, payload)
	if len(auths) != 3 {
		t.Fatalf("expected primary + 2 virtual auths, got %d", len(auths))
	}

	for _, auth := range auths[1:] {
		if auth.Attributes["concurrency_cap"] != "2" {
			t.Fatalf("%s concurrency_cap = %q, want 2", auth.ID, auth.Attributes["concurrency_cap"])
		}
		if auth.Attributes["concurrency_scope_key"] != "auth:"+auth.ID {
			t.Fatalf("%s concurrency_scope_key = %q, want auth:%s", auth.ID, auth.Attributes["concurrency_scope_key"], auth.ID)
		}
		if auth.Attributes["concurrency_reason"] != coreauth.LocalConcurrencyReasonReadonlySource {
			t.Fatalf("%s concurrency_reason = %q", auth.ID, auth.Attributes["concurrency_reason"])
		}
	}
}
