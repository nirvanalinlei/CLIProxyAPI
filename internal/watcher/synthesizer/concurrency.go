package synthesizer

import (
	"strconv"
	"strings"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

func applyManagedSingleAuthConcurrencyAttrs(attrs map[string]string, authID, sourceKind string, cap int) {
	if len(attrs) == 0 {
		return
	}
	cap = internalconfig.NormalizeConcurrencyValue(cap)
	mode := coreauth.LocalConcurrencyModeDisabled
	reason := coreauth.LocalConcurrencyReasonNoCap
	if cap > 0 {
		mode = coreauth.LocalConcurrencyModeObserveOnly
		reason = coreauth.LocalConcurrencyReasonObserveOnlyPhase
	}
	applyConcurrencyAttrs(
		attrs,
		cap,
		"auth:"+strings.TrimSpace(authID),
		mode,
		reason,
		sourceKind,
		coreauth.LocalConcurrencySchemaManaged,
		coreauth.LocalConcurrencyIdentityNotApplicable,
		"",
	)
}

func applyManagedOpenAICompatibilityConcurrencyAttrs(
	attrs map[string]string,
	entry internalconfig.OpenAICompatibility,
	issue *internalconfig.OpenAICompatibilityConcurrencyIssue,
) {
	if len(attrs) == 0 {
		return
	}
	cap := internalconfig.NormalizeConcurrencyValue(entry.Concurrency)
	scopeKey, normalizedName, normalizedBaseURL, ok := internalconfig.OpenAICompatibilityConcurrencyScopeKey(entry.Name, entry.BaseURL)
	mode := coreauth.LocalConcurrencyModeDisabled
	reason := coreauth.LocalConcurrencyReasonNoCap
	identityStatus := coreauth.LocalConcurrencyIdentityNotRequired
	identityDetail := ""
	if ok {
		identityDetail = normalizedName + " | " + normalizedBaseURL
	}
	if issue != nil {
		scopeKey = strings.TrimSpace(issue.ScopeKey)
		mode = coreauth.LocalConcurrencyModeDisabled
		reason = coreauth.LocalConcurrencyReasonInvalidScopeIdentity
		identityStatus = coreauth.LocalConcurrencyIdentityInvalid
		identityDetail = formatOpenAICompatibilityConcurrencyIssue(*issue)
	} else if cap > 0 {
		mode = coreauth.LocalConcurrencyModeObserveOnly
		reason = coreauth.LocalConcurrencyReasonObserveOnlyPhase
		identityStatus = coreauth.LocalConcurrencyIdentityValid
	}
	applyConcurrencyAttrs(
		attrs,
		cap,
		scopeKey,
		mode,
		reason,
		coreauth.LocalConcurrencySourceOpenAICompatibility,
		coreauth.LocalConcurrencySchemaManaged,
		identityStatus,
		identityDetail,
	)
}

func applyReadonlyAuthFileConcurrencyAttrs(auth *coreauth.Auth, metadata map[string]any) {
	if auth == nil {
		return
	}
	cap, ok := parseMetadataConcurrencyValue(metadata)
	if !ok {
		return
	}
	if auth.Attributes == nil {
		auth.Attributes = make(map[string]string)
	}
	mode, reason := coreauth.LocalConcurrencyModeDisabled, coreauth.LocalConcurrencyReasonNoCap
	if cap > 0 {
		mode, reason = coreauth.LocalConcurrencyModeObserveOnly, coreauth.LocalConcurrencyReasonReadonlySource
	}
	applyConcurrencyAttrs(
		auth.Attributes,
		cap,
		"auth:"+strings.TrimSpace(auth.ID),
		mode,
		reason,
		coreauth.LocalConcurrencySourceAuthFile,
		coreauth.LocalConcurrencySchemaReadonly,
		coreauth.LocalConcurrencyIdentityNotApplicable,
		"",
	)
}

func applyConcurrencyAttrs(
	attrs map[string]string,
	cap int,
	scopeKey, mode, reason, sourceKind, schemaSupport, identityStatus, identityDetail string,
) {
	if len(attrs) == 0 {
		return
	}
	attrs["concurrency_cap"] = strconv.Itoa(internalconfig.NormalizeConcurrencyValue(cap))
	attrs["concurrency_scope_key"] = strings.TrimSpace(scopeKey)
	attrs["concurrency_mode"] = strings.TrimSpace(mode)
	attrs["concurrency_reason"] = strings.TrimSpace(reason)
	attrs["concurrency_source_kind"] = strings.TrimSpace(sourceKind)
	attrs["concurrency_schema_support"] = strings.TrimSpace(schemaSupport)
	attrs["concurrency_identity_status"] = strings.TrimSpace(identityStatus)
	if trimmed := strings.TrimSpace(identityDetail); trimmed != "" {
		attrs["concurrency_identity_detail"] = trimmed
	} else {
		delete(attrs, "concurrency_identity_detail")
	}
}

func propagateConcurrencyAttrsForAuthID(attrs, source map[string]string, authID string) {
	if len(attrs) == 0 || len(source) == 0 {
		return
	}
	keys := []string{
		"concurrency_cap",
		"concurrency_mode",
		"concurrency_reason",
		"concurrency_source_kind",
		"concurrency_schema_support",
		"concurrency_identity_status",
		"concurrency_identity_detail",
	}
	propagated := false
	for _, key := range keys {
		value := strings.TrimSpace(source[key])
		if value == "" {
			continue
		}
		attrs[key] = value
		propagated = true
	}
	if propagated {
		attrs["concurrency_scope_key"] = "auth:" + strings.TrimSpace(authID)
	}
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
	return strings.TrimSpace(issue.Reason)
}
