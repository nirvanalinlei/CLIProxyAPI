package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

const (
	LocalConcurrencyModeObserveOnly = "observe_only"
	LocalConcurrencyModeDisabled    = "disabled"

	LocalConcurrencyReasonObserveOnlyPhase     = "observe_only_phase"
	LocalConcurrencyReasonNoCap                = "no_cap"
	LocalConcurrencyReasonInvalidScopeIdentity = "invalid_scope_identity"
	LocalConcurrencyReasonReadonlySource       = "readonly_source"

	LocalConcurrencySourceGeminiAPIKey        = "gemini_api_key"
	LocalConcurrencySourceClaudeAPIKey        = "claude_api_key"
	LocalConcurrencySourceCodexAPIKey         = "codex_api_key"
	LocalConcurrencySourceOpenAICompatibility = "openai_compatibility"
	LocalConcurrencySourceVertexAPIKey        = "vertex_api_key"
	LocalConcurrencySourceAuthFile            = "auth_file"

	LocalConcurrencySchemaManaged  = "managed"
	LocalConcurrencySchemaReadonly = "readonly"

	LocalConcurrencyIdentityValid         = "valid"
	LocalConcurrencyIdentityInvalid       = "invalid"
	LocalConcurrencyIdentityNotApplicable = "not_applicable"
	LocalConcurrencyIdentityNotRequired   = "not_required"
)

type LocalConcurrencySnapshot struct {
	ScopeKey         string     `json:"scope_key"`
	SourceKind       string     `json:"source_kind"`
	SchemaSupport    string     `json:"schema_support"`
	Mode             string     `json:"mode"`
	Reason           string     `json:"reason"`
	Generation       uint64     `json:"generation"`
	RuntimeAppliedAt time.Time  `json:"runtime_applied_at"`
	IdentityStatus   string     `json:"identity_status"`
	IdentityDetail   string     `json:"identity_detail,omitempty"`
	Cap              int        `json:"cap"`
	Inflight         int        `json:"inflight"`
	RejectCount      uint64     `json:"reject_count"`
	LastRejectAt     *time.Time `json:"last_reject_at,omitempty"`
	AuthID           string     `json:"auth_id,omitempty"`
	Provider         string     `json:"provider,omitempty"`
	Label            string     `json:"label,omitempty"`
}

type localConcurrencyScope struct {
	snapshot LocalConcurrencySnapshot
}

type localConcurrencyRegistry struct {
	mu               sync.RWMutex
	scopes           map[string]*localConcurrencyScope
	extras           []LocalConcurrencySnapshot
	generation       uint64
	runtimeAppliedAt time.Time
}

func newLocalConcurrencyRegistry() *localConcurrencyRegistry {
	return &localConcurrencyRegistry{
		scopes: make(map[string]*localConcurrencyScope),
	}
}

func (r *localConcurrencyRegistry) Replace(entries []LocalConcurrencySnapshot) {
	if r == nil {
		return
	}
	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.generation++
	r.runtimeAppliedAt = now
	nextScopes := make(map[string]*localConcurrencyScope, len(entries))
	nextExtras := make([]LocalConcurrencySnapshot, 0)
	for i := range entries {
		entry := entries[i]
		entry.Cap = internalconfig.NormalizeConcurrencyValue(entry.Cap)
		entry.Generation = r.generation
		entry.RuntimeAppliedAt = now
		scopeKey := strings.TrimSpace(entry.ScopeKey)
		if scopeKey == "" {
			nextExtras = append(nextExtras, entry)
			continue
		}
		if _, exists := nextScopes[scopeKey]; exists {
			nextExtras = append(nextExtras, entry)
			continue
		}
		nextScopes[scopeKey] = &localConcurrencyScope{snapshot: entry}
	}
	r.scopes = nextScopes
	r.extras = nextExtras
}

func (r *localConcurrencyRegistry) Snapshots() []LocalConcurrencySnapshot {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]LocalConcurrencySnapshot, 0, len(r.scopes)+len(r.extras))
	for _, scope := range r.scopes {
		if scope == nil {
			continue
		}
		out = append(out, scope.snapshot)
	}
	out = append(out, r.extras...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].SourceKind != out[j].SourceKind {
			return out[i].SourceKind < out[j].SourceKind
		}
		if out[i].ScopeKey != out[j].ScopeKey {
			return out[i].ScopeKey < out[j].ScopeKey
		}
		if out[i].AuthID != out[j].AuthID {
			return out[i].AuthID < out[j].AuthID
		}
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		if out[i].Label != out[j].Label {
			return out[i].Label < out[j].Label
		}
		return out[i].IdentityDetail < out[j].IdentityDetail
	})
	return out
}

func buildLocalConcurrencySnapshots(cfg *internalconfig.Config, auths []*Auth) []LocalConcurrencySnapshot {
	out := buildManagedLocalConcurrencySnapshots(cfg)
	out = append(out, buildReadonlyLocalConcurrencySnapshots(auths)...)
	return out
}

func buildManagedLocalConcurrencySnapshots(cfg *internalconfig.Config) []LocalConcurrencySnapshot {
	if cfg == nil {
		return nil
	}
	idGen := newLocalStableIDGenerator()
	out := make([]LocalConcurrencySnapshot, 0, len(cfg.GeminiKey)+len(cfg.ClaudeKey)+len(cfg.CodexKey)+len(cfg.OpenAICompatibility)+len(cfg.VertexCompatAPIKey))
	for i := range cfg.GeminiKey {
		entry := cfg.GeminiKey[i]
		key := strings.TrimSpace(entry.APIKey)
		if key == "" {
			continue
		}
		authID := idGen.Next("gemini:apikey", key, strings.TrimSpace(entry.BaseURL))
		out = append(out, newManagedSingleAuthConcurrencySnapshot(
			authID,
			"gemini",
			"gemini-apikey",
			LocalConcurrencySourceGeminiAPIKey,
			entry.Concurrency,
		))
	}
	for i := range cfg.ClaudeKey {
		entry := cfg.ClaudeKey[i]
		key := strings.TrimSpace(entry.APIKey)
		if key == "" {
			continue
		}
		authID := idGen.Next("claude:apikey", key, strings.TrimSpace(entry.BaseURL))
		out = append(out, newManagedSingleAuthConcurrencySnapshot(
			authID,
			"claude",
			"claude-apikey",
			LocalConcurrencySourceClaudeAPIKey,
			entry.Concurrency,
		))
	}
	for i := range cfg.CodexKey {
		entry := cfg.CodexKey[i]
		key := strings.TrimSpace(entry.APIKey)
		if key == "" {
			continue
		}
		authID := idGen.Next("codex:apikey", key, strings.TrimSpace(entry.BaseURL))
		out = append(out, newManagedSingleAuthConcurrencySnapshot(
			authID,
			"codex",
			"codex-apikey",
			LocalConcurrencySourceCodexAPIKey,
			entry.Concurrency,
		))
	}
	compatIssues := make(map[int]internalconfig.OpenAICompatibilityConcurrencyIssue)
	for _, issue := range internalconfig.OpenAICompatibilityConcurrencyIssues(cfg.OpenAICompatibility) {
		compatIssues[issue.Index] = issue
	}
	for i := range cfg.OpenAICompatibility {
		entry := cfg.OpenAICompatibility[i]
		if !entry.IsEnabled() {
			continue
		}
		provider := strings.ToLower(strings.TrimSpace(entry.Name))
		if provider == "" {
			provider = "openai-compatibility"
		}
		out = append(out, newManagedOpenAICompatibilitySnapshot(entry, compatIssues[i], provider))
	}
	for i := range cfg.VertexCompatAPIKey {
		entry := cfg.VertexCompatAPIKey[i]
		key := strings.TrimSpace(entry.APIKey)
		if key == "" {
			continue
		}
		authID := idGen.Next("vertex:apikey", key, strings.TrimSpace(entry.BaseURL), strings.TrimSpace(entry.ProxyURL))
		out = append(out, newManagedSingleAuthConcurrencySnapshot(
			authID,
			"vertex",
			"vertex-apikey",
			LocalConcurrencySourceVertexAPIKey,
			entry.Concurrency,
		))
	}
	return out
}

func buildReadonlyLocalConcurrencySnapshots(auths []*Auth) []LocalConcurrencySnapshot {
	if len(auths) == 0 {
		return nil
	}
	out := make([]LocalConcurrencySnapshot, 0, len(auths))
	for _, auth := range auths {
		if auth == nil || auth.Disabled || auth.Status == StatusDisabled {
			continue
		}
		snapshot, ok := newReadonlyAuthFileConcurrencySnapshot(auth)
		if !ok {
			continue
		}
		out = append(out, snapshot)
	}
	return out
}

func newManagedSingleAuthConcurrencySnapshot(authID, provider, label, sourceKind string, cap int) LocalConcurrencySnapshot {
	cap = internalconfig.NormalizeConcurrencyValue(cap)
	mode, reason := localConcurrencyModeAndReason(cap, false)
	return LocalConcurrencySnapshot{
		ScopeKey:       "auth:" + strings.TrimSpace(authID),
		SourceKind:     sourceKind,
		SchemaSupport:  LocalConcurrencySchemaManaged,
		Mode:           mode,
		Reason:         reason,
		IdentityStatus: LocalConcurrencyIdentityNotApplicable,
		Cap:            cap,
		AuthID:         strings.TrimSpace(authID),
		Provider:       provider,
		Label:          label,
	}
}

func newManagedOpenAICompatibilitySnapshot(entry internalconfig.OpenAICompatibility, issue internalconfig.OpenAICompatibilityConcurrencyIssue, provider string) LocalConcurrencySnapshot {
	cap := internalconfig.NormalizeConcurrencyValue(entry.Concurrency)
	scopeKey, normalizedName, normalizedBaseURL, ok := internalconfig.OpenAICompatibilityConcurrencyScopeKey(entry.Name, entry.BaseURL)
	snapshot := LocalConcurrencySnapshot{
		ScopeKey:      scopeKey,
		SourceKind:    LocalConcurrencySourceOpenAICompatibility,
		SchemaSupport: LocalConcurrencySchemaManaged,
		Cap:           cap,
		Provider:      provider,
		Label:         strings.TrimSpace(entry.Name),
	}
	if issue.Reason != "" {
		snapshot.ScopeKey = strings.TrimSpace(issue.ScopeKey)
		snapshot.Mode = LocalConcurrencyModeDisabled
		snapshot.Reason = LocalConcurrencyReasonInvalidScopeIdentity
		snapshot.IdentityStatus = LocalConcurrencyIdentityInvalid
		snapshot.IdentityDetail = formatOpenAICompatibilityConcurrencyIssue(issue)
		return snapshot
	}
	if cap <= 0 {
		snapshot.Mode = LocalConcurrencyModeDisabled
		snapshot.Reason = LocalConcurrencyReasonNoCap
		snapshot.IdentityStatus = LocalConcurrencyIdentityNotRequired
		if ok {
			snapshot.IdentityDetail = normalizedName + " | " + normalizedBaseURL
		}
		return snapshot
	}
	snapshot.Mode = LocalConcurrencyModeObserveOnly
	snapshot.Reason = LocalConcurrencyReasonObserveOnlyPhase
	snapshot.IdentityStatus = LocalConcurrencyIdentityValid
	if ok {
		snapshot.IdentityDetail = normalizedName + " | " + normalizedBaseURL
	}
	return snapshot
}

func newReadonlyAuthFileConcurrencySnapshot(auth *Auth) (LocalConcurrencySnapshot, bool) {
	if auth == nil {
		return LocalConcurrencySnapshot{}, false
	}
	if snapshot, ok := newReadonlyAuthFileConcurrencySnapshotFromAttrs(auth); ok {
		return snapshot, true
	}
	cap, ok := parseMetadataConcurrencyValue(auth.Metadata)
	if !ok {
		return LocalConcurrencySnapshot{}, false
	}
	mode, reason := localConcurrencyModeAndReason(cap, true)
	return LocalConcurrencySnapshot{
		ScopeKey:       "auth:" + strings.TrimSpace(auth.ID),
		SourceKind:     LocalConcurrencySourceAuthFile,
		SchemaSupport:  LocalConcurrencySchemaReadonly,
		Mode:           mode,
		Reason:         reason,
		IdentityStatus: LocalConcurrencyIdentityNotApplicable,
		Cap:            cap,
		AuthID:         strings.TrimSpace(auth.ID),
		Provider:       strings.TrimSpace(auth.Provider),
		Label:          strings.TrimSpace(auth.Label),
	}, true
}

func newReadonlyAuthFileConcurrencySnapshotFromAttrs(auth *Auth) (LocalConcurrencySnapshot, bool) {
	if auth == nil || len(auth.Attributes) == 0 {
		return LocalConcurrencySnapshot{}, false
	}
	schemaSupport := strings.TrimSpace(auth.Attributes["concurrency_schema_support"])
	sourceKind := strings.TrimSpace(auth.Attributes["concurrency_source_kind"])
	if schemaSupport != LocalConcurrencySchemaReadonly || sourceKind != LocalConcurrencySourceAuthFile {
		return LocalConcurrencySnapshot{}, false
	}
	cap, err := strconv.Atoi(strings.TrimSpace(auth.Attributes["concurrency_cap"]))
	if err != nil {
		return LocalConcurrencySnapshot{}, false
	}
	cap = internalconfig.NormalizeConcurrencyValue(cap)
	scopeKey := strings.TrimSpace(auth.Attributes["concurrency_scope_key"])
	if scopeKey == "" {
		scopeKey = "auth:" + strings.TrimSpace(auth.ID)
	}
	mode := strings.TrimSpace(auth.Attributes["concurrency_mode"])
	reason := strings.TrimSpace(auth.Attributes["concurrency_reason"])
	if mode == "" || reason == "" {
		mode, reason = localConcurrencyModeAndReason(cap, true)
	}
	identityStatus := strings.TrimSpace(auth.Attributes["concurrency_identity_status"])
	if identityStatus == "" {
		identityStatus = LocalConcurrencyIdentityNotApplicable
	}
	return LocalConcurrencySnapshot{
		ScopeKey:       scopeKey,
		SourceKind:     LocalConcurrencySourceAuthFile,
		SchemaSupport:  LocalConcurrencySchemaReadonly,
		Mode:           mode,
		Reason:         reason,
		IdentityStatus: identityStatus,
		IdentityDetail: strings.TrimSpace(auth.Attributes["concurrency_identity_detail"]),
		Cap:            cap,
		AuthID:         strings.TrimSpace(auth.ID),
		Provider:       strings.TrimSpace(auth.Provider),
		Label:          strings.TrimSpace(auth.Label),
	}, true
}

func localConcurrencyModeAndReason(cap int, readonly bool) (mode, reason string) {
	cap = internalconfig.NormalizeConcurrencyValue(cap)
	if cap <= 0 {
		return LocalConcurrencyModeDisabled, LocalConcurrencyReasonNoCap
	}
	if readonly {
		return LocalConcurrencyModeObserveOnly, LocalConcurrencyReasonReadonlySource
	}
	return LocalConcurrencyModeObserveOnly, LocalConcurrencyReasonObserveOnlyPhase
}

func formatOpenAICompatibilityConcurrencyIssue(issue internalconfig.OpenAICompatibilityConcurrencyIssue) string {
	switch issue.Reason {
	case internalconfig.OpenAICompatibilityConcurrencyIssueMissingName:
		return "name is required when concurrency > 0"
	case internalconfig.OpenAICompatibilityConcurrencyIssueMissingBaseURL:
		return "base-url is required when concurrency > 0"
	case internalconfig.OpenAICompatibilityConcurrencyIssueDuplicateScope:
		if issue.ScopeKey != "" {
			return "duplicate concurrency scope: " + issue.ScopeKey
		}
	}
	return issue.Reason
}

func parseMetadataConcurrencyValue(metadata map[string]any) (int, bool) {
	if len(metadata) == 0 {
		return 0, false
	}
	raw, ok := metadata["concurrency"]
	if !ok {
		return 0, false
	}
	switch value := raw.(type) {
	case int:
		return internalconfig.NormalizeConcurrencyValue(value), true
	case int64:
		return internalconfig.NormalizeConcurrencyValue(int(value)), true
	case float64:
		return internalconfig.NormalizeConcurrencyValue(int(value)), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return 0, false
		}
		return internalconfig.NormalizeConcurrencyValue(parsed), true
	default:
		return 0, false
	}
}

type localStableIDGenerator struct {
	counters map[string]int
}

func newLocalStableIDGenerator() *localStableIDGenerator {
	return &localStableIDGenerator{counters: make(map[string]int)}
}

func (g *localStableIDGenerator) Next(kind string, parts ...string) string {
	if g == nil {
		return kind + ":000000000000"
	}
	hasher := sha256.New()
	hasher.Write([]byte(kind))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		hasher.Write([]byte{0})
		hasher.Write([]byte(trimmed))
	}
	digest := hex.EncodeToString(hasher.Sum(nil))
	if len(digest) < 12 {
		digest = fmt.Sprintf("%012s", digest)
	}
	short := digest[:12]
	key := kind + ":" + short
	index := g.counters[key]
	g.counters[key] = index + 1
	if index > 0 {
		short = fmt.Sprintf("%s-%d", short, index)
	}
	return fmt.Sprintf("%s:%s", kind, short)
}
