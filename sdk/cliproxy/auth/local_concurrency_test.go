package auth

import (
	"context"
	"testing"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

type localConcurrencyTestStore struct {
	items []*Auth
}

func (s *localConcurrencyTestStore) List(context.Context) ([]*Auth, error) {
	out := make([]*Auth, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item.Clone())
	}
	return out, nil
}

func (s *localConcurrencyTestStore) Save(context.Context, *Auth) (string, error) { return "", nil }
func (s *localConcurrencyTestStore) Delete(context.Context, string) error        { return nil }

func TestManagerLocalConcurrencySnapshots_ReconcileOnSetConfigAndRegister(t *testing.T) {
	t.Parallel()

	manager := NewManager(nil, nil, nil)
	manager.SetConfig(&internalconfig.Config{
		GeminiKey: []internalconfig.GeminiKey{
			{APIKey: "gem-key", BaseURL: "https://gem.example.com", Concurrency: 2},
		},
		OpenAICompatibility: []internalconfig.OpenAICompatibility{
			{Name: "dup", BaseURL: "https://compat.example.com/v1", Concurrency: 3},
			{Name: " DUP ", BaseURL: "https://compat.example.com:443/v1/", Concurrency: 5},
		},
	})

	snapshots := manager.LocalConcurrencySnapshots()
	if len(snapshots) != 3 {
		t.Fatalf("expected 3 snapshots after SetConfig, got %d: %#v", len(snapshots), snapshots)
	}
	if got := snapshots[0].Generation; got == 0 {
		t.Fatalf("expected generation > 0, got %d", got)
	}

	var foundGemini bool
	var invalidCount int
	for _, snapshot := range snapshots {
		if snapshot.SourceKind == LocalConcurrencySourceGeminiAPIKey {
			foundGemini = true
			if snapshot.Mode != LocalConcurrencyModeObserveOnly || snapshot.Reason != LocalConcurrencyReasonObserveOnlyPhase {
				t.Fatalf("unexpected gemini snapshot: %#v", snapshot)
			}
		}
		if snapshot.SourceKind == LocalConcurrencySourceOpenAICompatibility {
			if snapshot.Reason != LocalConcurrencyReasonInvalidScopeIdentity {
				t.Fatalf("unexpected compat snapshot: %#v", snapshot)
			}
			invalidCount++
		}
	}
	if !foundGemini {
		t.Fatal("expected gemini snapshot")
	}
	if invalidCount != 2 {
		t.Fatalf("expected 2 invalid compat snapshots, got %d", invalidCount)
	}

	beforeGeneration := snapshots[0].Generation
	if _, err := manager.Register(context.Background(), &Auth{
		ID:       "auth-file-1",
		Provider: "codex",
		Label:    "file-user",
		Attributes: map[string]string{
			"concurrency_cap":             "7",
			"concurrency_scope_key":       "auth:auth-file-1",
			"concurrency_mode":            LocalConcurrencyModeObserveOnly,
			"concurrency_reason":          LocalConcurrencyReasonReadonlySource,
			"concurrency_source_kind":     LocalConcurrencySourceAuthFile,
			"concurrency_schema_support":  LocalConcurrencySchemaReadonly,
			"concurrency_identity_status": LocalConcurrencyIdentityNotApplicable,
		},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	snapshots = manager.LocalConcurrencySnapshots()
	if len(snapshots) != 4 {
		t.Fatalf("expected 4 snapshots after Register, got %d", len(snapshots))
	}
	if snapshots[0].Generation <= beforeGeneration {
		t.Fatalf("expected generation to advance, got %d <= %d", snapshots[0].Generation, beforeGeneration)
	}
}

func TestManagerLocalConcurrencySnapshots_ReconcileOnLoad(t *testing.T) {
	t.Parallel()

	store := &localConcurrencyTestStore{
		items: []*Auth{
			{
				ID:       "loaded-auth",
				Provider: "codex",
				Label:    "loaded",
				Attributes: map[string]string{
					"concurrency_cap":             "3",
					"concurrency_scope_key":       "auth:loaded-auth",
					"concurrency_mode":            LocalConcurrencyModeObserveOnly,
					"concurrency_reason":          LocalConcurrencyReasonReadonlySource,
					"concurrency_source_kind":     LocalConcurrencySourceAuthFile,
					"concurrency_schema_support":  LocalConcurrencySchemaReadonly,
					"concurrency_identity_status": LocalConcurrencyIdentityNotApplicable,
				},
			},
		},
	}
	manager := NewManager(store, nil, nil)
	manager.SetConfig(&internalconfig.Config{
		CodexKey: []internalconfig.CodexKey{
			{APIKey: "codex-key", BaseURL: "https://api.openai.com", Concurrency: 1},
		},
	})
	if err := manager.Load(context.Background()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	snapshots := manager.LocalConcurrencySnapshots()
	if len(snapshots) != 2 {
		t.Fatalf("expected 2 snapshots after Load, got %d: %#v", len(snapshots), snapshots)
	}
	for _, snapshot := range snapshots {
		if snapshot.RuntimeAppliedAt.IsZero() {
			t.Fatalf("expected runtime_applied_at to be set: %#v", snapshot)
		}
	}
}

func TestManagerLocalConcurrencySnapshots_ReadonlyAuthFileEdges(t *testing.T) {
	t.Parallel()

	manager := NewManager(nil, nil, nil)
	if _, err := manager.Register(context.Background(), &Auth{
		ID:       "disabled-auth",
		Provider: "codex",
		Status:   StatusDisabled,
		Disabled: true,
		Attributes: map[string]string{
			"concurrency_cap":             "3",
			"concurrency_scope_key":       "auth:disabled-auth",
			"concurrency_mode":            LocalConcurrencyModeObserveOnly,
			"concurrency_reason":          LocalConcurrencyReasonReadonlySource,
			"concurrency_source_kind":     LocalConcurrencySourceAuthFile,
			"concurrency_schema_support":  LocalConcurrencySchemaReadonly,
			"concurrency_identity_status": LocalConcurrencyIdentityNotApplicable,
		},
	}); err != nil {
		t.Fatalf("Register(disabled) error = %v", err)
	}
	if _, err := manager.Register(context.Background(), &Auth{
		ID:       "mismatch-auth",
		Provider: "codex",
		Status:   StatusActive,
		Attributes: map[string]string{
			"concurrency_cap":             "3",
			"concurrency_scope_key":       "auth:mismatch-auth",
			"concurrency_mode":            LocalConcurrencyModeObserveOnly,
			"concurrency_reason":          LocalConcurrencyReasonReadonlySource,
			"concurrency_source_kind":     LocalConcurrencySourceGeminiAPIKey,
			"concurrency_schema_support":  LocalConcurrencySchemaReadonly,
			"concurrency_identity_status": LocalConcurrencyIdentityNotApplicable,
		},
	}); err != nil {
		t.Fatalf("Register(mismatch) error = %v", err)
	}
	if _, err := manager.Register(context.Background(), &Auth{
		ID:       "metadata-fallback-auth",
		Provider: "codex",
		Status:   StatusActive,
		Attributes: map[string]string{
			"concurrency_cap":             "bad",
			"concurrency_scope_key":       "auth:metadata-fallback-auth",
			"concurrency_mode":            LocalConcurrencyModeObserveOnly,
			"concurrency_reason":          LocalConcurrencyReasonReadonlySource,
			"concurrency_source_kind":     LocalConcurrencySourceAuthFile,
			"concurrency_schema_support":  LocalConcurrencySchemaReadonly,
			"concurrency_identity_status": LocalConcurrencyIdentityNotApplicable,
		},
		Metadata: map[string]any{"concurrency": 5},
	}); err != nil {
		t.Fatalf("Register(metadata fallback) error = %v", err)
	}

	snapshots := manager.LocalConcurrencySnapshots()
	if len(snapshots) != 1 {
		t.Fatalf("expected only metadata fallback snapshot, got %d: %#v", len(snapshots), snapshots)
	}
	if snapshots[0].AuthID != "metadata-fallback-auth" || snapshots[0].Cap != 5 {
		t.Fatalf("unexpected fallback snapshot: %#v", snapshots[0])
	}
}
