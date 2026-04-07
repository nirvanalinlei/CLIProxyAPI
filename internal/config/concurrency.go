package config

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

const (
	OpenAICompatibilityConcurrencyIssueMissingName    = "missing_name"
	OpenAICompatibilityConcurrencyIssueMissingBaseURL = "missing_base_url"
	OpenAICompatibilityConcurrencyIssueDuplicateScope = "duplicate_scope"
)

type OpenAICompatibilityConcurrencyIssue struct {
	Index    int    `json:"index"`
	ScopeKey string `json:"scope_key,omitempty"`
	Name     string `json:"name,omitempty"`
	BaseURL  string `json:"base_url,omitempty"`
	Reason   string `json:"reason"`
}

type OpenAICompatibilityConcurrencyValidationError struct {
	Issues []OpenAICompatibilityConcurrencyIssue `json:"issues"`
}

func (e *OpenAICompatibilityConcurrencyValidationError) Error() string {
	if e == nil || len(e.Issues) == 0 {
		return "invalid openai-compatibility concurrency configuration"
	}
	parts := make([]string, 0, len(e.Issues))
	for _, issue := range e.Issues {
		part := fmt.Sprintf("index %d: %s", issue.Index, issue.Reason)
		if issue.ScopeKey != "" {
			part += " (" + issue.ScopeKey + ")"
		}
		parts = append(parts, part)
	}
	return "invalid openai-compatibility concurrency configuration: " + strings.Join(parts, "; ")
}

func NormalizeConcurrencyValue(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func NormalizeConcurrencyBaseURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || !parsed.IsAbs() || strings.TrimSpace(parsed.Scheme) == "" || strings.TrimSpace(parsed.Host) == "" {
		return ""
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	port := strings.TrimSpace(parsed.Port())
	switch {
	case port == "":
	case scheme == "http" && port == "80":
	case scheme == "https" && port == "443":
	default:
		host = host + ":" + port
	}
	path := strings.TrimRight(parsed.EscapedPath(), "/")
	if path == "/" {
		path = ""
	}
	return scheme + "://" + host + path
}

func OpenAICompatibilityConcurrencyScopeKey(name, baseURL string) (scopeKey, normalizedName, normalizedBaseURL string, ok bool) {
	normalizedName = strings.ToLower(strings.TrimSpace(name))
	normalizedBaseURL = NormalizeConcurrencyBaseURL(baseURL)
	if normalizedName == "" || normalizedBaseURL == "" {
		return "", normalizedName, normalizedBaseURL, false
	}
	return "compat:" + normalizedName + "|" + normalizedBaseURL, normalizedName, normalizedBaseURL, true
}

func OpenAICompatibilityConcurrencyIssues(entries []OpenAICompatibility) []OpenAICompatibilityConcurrencyIssue {
	if len(entries) == 0 {
		return nil
	}
	type scopeInfo struct {
		index   int
		name    string
		baseURL string
	}
	seen := make(map[string]scopeInfo, len(entries))
	duplicated := make(map[string]bool, len(entries))
	out := make([]OpenAICompatibilityConcurrencyIssue, 0)
	for i := range entries {
		entry := entries[i]
		if !entry.IsEnabled() {
			continue
		}
		if NormalizeConcurrencyValue(entry.Concurrency) <= 0 {
			continue
		}
		scopeKey, normalizedName, normalizedBaseURL, ok := OpenAICompatibilityConcurrencyScopeKey(entry.Name, entry.BaseURL)
		switch {
		case normalizedName == "":
			out = append(out, OpenAICompatibilityConcurrencyIssue{
				Index:   i,
				Name:    strings.TrimSpace(entry.Name),
				BaseURL: strings.TrimSpace(entry.BaseURL),
				Reason:  OpenAICompatibilityConcurrencyIssueMissingName,
			})
			continue
		case normalizedBaseURL == "":
			out = append(out, OpenAICompatibilityConcurrencyIssue{
				Index:   i,
				Name:    strings.TrimSpace(entry.Name),
				BaseURL: strings.TrimSpace(entry.BaseURL),
				Reason:  OpenAICompatibilityConcurrencyIssueMissingBaseURL,
			})
			continue
		case !ok:
			continue
		}
		if prev, exists := seen[scopeKey]; exists {
			if !duplicated[scopeKey] {
				out = append(out, OpenAICompatibilityConcurrencyIssue{
					Index:    prev.index,
					ScopeKey: scopeKey,
					Name:     prev.name,
					BaseURL:  prev.baseURL,
					Reason:   OpenAICompatibilityConcurrencyIssueDuplicateScope,
				})
				duplicated[scopeKey] = true
			}
			out = append(out, OpenAICompatibilityConcurrencyIssue{
				Index:    i,
				ScopeKey: scopeKey,
				Name:     normalizedName,
				BaseURL:  normalizedBaseURL,
				Reason:   OpenAICompatibilityConcurrencyIssueDuplicateScope,
			})
			continue
		}
		seen[scopeKey] = scopeInfo{
			index:   i,
			name:    normalizedName,
			baseURL: normalizedBaseURL,
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Index != out[j].Index {
			return out[i].Index < out[j].Index
		}
		return out[i].Reason < out[j].Reason
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

func ValidateOpenAICompatibilityConcurrencyEntries(entries []OpenAICompatibility) error {
	issues := OpenAICompatibilityConcurrencyIssues(entries)
	if len(issues) == 0 {
		return nil
	}
	return &OpenAICompatibilityConcurrencyValidationError{Issues: issues}
}
