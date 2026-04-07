package config

import "testing"

func TestNormalizeConcurrencyBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "normalizes parsed url",
			input: " HTTPS://API.EXAMPLE.COM:443/v1/?q=1#frag ",
			want:  "https://api.example.com/v1",
		},
		{
			name:  "rejects non absolute url",
			input: " API.EXAMPLE.COM/Path/ ",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := NormalizeConcurrencyBaseURL(tt.input); got != tt.want {
				t.Fatalf("NormalizeConcurrencyBaseURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestOpenAICompatibilityConcurrencyIssues(t *testing.T) {
	t.Parallel()

	entries := []OpenAICompatibility{
		{
			Name:        " Foo ",
			BaseURL:     "HTTPS://api.example.com:443/v1/",
			Concurrency: 2,
		},
		{
			Name:        "foo",
			BaseURL:     "https://api.example.com/v1",
			Concurrency: 1,
		},
		{
			Name:        "",
			BaseURL:     "https://other.example.com",
			Concurrency: 3,
		},
		{
			Name:        "disabled",
			BaseURL:     "",
			Concurrency: 0,
		},
	}

	issues := OpenAICompatibilityConcurrencyIssues(entries)
	if len(issues) != 3 {
		t.Fatalf("expected 3 issues, got %d: %#v", len(issues), issues)
	}

	if issues[0].Index != 0 || issues[0].Reason != OpenAICompatibilityConcurrencyIssueDuplicateScope {
		t.Fatalf("unexpected first issue: %#v", issues[0])
	}
	if issues[0].ScopeKey != "compat:foo|https://api.example.com/v1" {
		t.Fatalf("unexpected duplicate scope key: %q", issues[0].ScopeKey)
	}
	if issues[1].Index != 1 || issues[1].Reason != OpenAICompatibilityConcurrencyIssueDuplicateScope {
		t.Fatalf("unexpected second issue: %#v", issues[1])
	}
	if issues[2].Index != 2 || issues[2].Reason != OpenAICompatibilityConcurrencyIssueMissingName {
		t.Fatalf("unexpected third issue: %#v", issues[2])
	}

	err := ValidateOpenAICompatibilityConcurrencyEntries(entries)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	validationErr, ok := err.(*OpenAICompatibilityConcurrencyValidationError)
	if !ok {
		t.Fatalf("expected OpenAICompatibilityConcurrencyValidationError, got %T", err)
	}
	if len(validationErr.Issues) != 3 {
		t.Fatalf("validation error issues = %d, want 3", len(validationErr.Issues))
	}
}

func TestOpenAICompatibilityConcurrencyIssuesTreatsInvalidBaseURLAsMissing(t *testing.T) {
	t.Parallel()

	entries := []OpenAICompatibility{
		{
			Name:        "foo",
			BaseURL:     "api.example.com/path",
			Concurrency: 2,
		},
	}

	issues := OpenAICompatibilityConcurrencyIssues(entries)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d: %#v", len(issues), issues)
	}
	if issues[0].Reason != OpenAICompatibilityConcurrencyIssueMissingBaseURL {
		t.Fatalf("unexpected issue: %#v", issues[0])
	}
}

func TestOpenAICompatibilityConcurrencyIssuesIgnoresDisabledProviders(t *testing.T) {
	t.Parallel()

	disabled := false
	entries := []OpenAICompatibility{
		{
			Name:        "dup",
			BaseURL:     "https://compat.example.com/v1",
			Concurrency: 2,
		},
		{
			Name:        " DUP ",
			BaseURL:     "https://compat.example.com:443/v1/",
			Enabled:     &disabled,
			Concurrency: 3,
		},
	}

	if issues := OpenAICompatibilityConcurrencyIssues(entries); len(issues) != 0 {
		t.Fatalf("expected disabled provider to be ignored, got %#v", issues)
	}
	if err := ValidateOpenAICompatibilityConcurrencyEntries(entries); err != nil {
		t.Fatalf("expected validation to pass, got %v", err)
	}
}
