package executor

import (
	"testing"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

func TestParseOpenAIUsageChatCompletions(t *testing.T) {
	data := []byte(`{"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3,"prompt_tokens_details":{"cached_tokens":4},"completion_tokens_details":{"reasoning_tokens":5}}}`)
	detail := parseOpenAIUsage(data)
	if detail.InputTokens != 1 {
		t.Fatalf("input tokens = %d, want %d", detail.InputTokens, 1)
	}
	if detail.OutputTokens != 2 {
		t.Fatalf("output tokens = %d, want %d", detail.OutputTokens, 2)
	}
	if detail.TotalTokens != 3 {
		t.Fatalf("total tokens = %d, want %d", detail.TotalTokens, 3)
	}
	if detail.CachedTokens != 4 {
		t.Fatalf("cached tokens = %d, want %d", detail.CachedTokens, 4)
	}
	if detail.ReasoningTokens != 5 {
		t.Fatalf("reasoning tokens = %d, want %d", detail.ReasoningTokens, 5)
	}
}

func TestParseOpenAIUsageResponses(t *testing.T) {
	data := []byte(`{"usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30,"input_tokens_details":{"cached_tokens":7},"output_tokens_details":{"reasoning_tokens":9}}}`)
	detail := parseOpenAIUsage(data)
	if detail.InputTokens != 10 {
		t.Fatalf("input tokens = %d, want %d", detail.InputTokens, 10)
	}
	if detail.OutputTokens != 20 {
		t.Fatalf("output tokens = %d, want %d", detail.OutputTokens, 20)
	}
	if detail.TotalTokens != 30 {
		t.Fatalf("total tokens = %d, want %d", detail.TotalTokens, 30)
	}
	if detail.CachedTokens != 7 {
		t.Fatalf("cached tokens = %d, want %d", detail.CachedTokens, 7)
	}
	if detail.ReasoningTokens != 9 {
		t.Fatalf("reasoning tokens = %d, want %d", detail.ReasoningTokens, 9)
	}
}

func TestParseOpenAIStreamUsageResponses(t *testing.T) {
	line := []byte(`data: {"type":"response.completed","response":{"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5,"input_tokens_details":{"cached_tokens":1},"output_tokens_details":{"reasoning_tokens":4}}}}`)
	detail, ok := parseOpenAIStreamUsage(line)
	if !ok {
		t.Fatalf("expected usage parse success")
	}
	if detail.InputTokens != 2 {
		t.Fatalf("input tokens = %d, want %d", detail.InputTokens, 2)
	}
	if detail.OutputTokens != 3 {
		t.Fatalf("output tokens = %d, want %d", detail.OutputTokens, 3)
	}
	if detail.TotalTokens != 5 {
		t.Fatalf("total tokens = %d, want %d", detail.TotalTokens, 5)
	}
	if detail.CachedTokens != 1 {
		t.Fatalf("cached tokens = %d, want %d", detail.CachedTokens, 1)
	}
	if detail.ReasoningTokens != 4 {
		t.Fatalf("reasoning tokens = %d, want %d", detail.ReasoningTokens, 4)
	}
}

func TestMaskUsageSourceAPIKey(t *testing.T) {
	cases := []struct {
		name string
		key  string
		want string
	}{
		{name: "short placeholder", key: "any", want: "a********y"},
		{name: "single char", key: "a", want: "a********a"},
		{name: "regular key", key: "sk-test-123456", want: "sk******56"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := maskUsageSourceAPIKey(tc.key)
			if got != tc.want {
				t.Fatalf("maskUsageSourceAPIKey(%q) = %q, want %q", tc.key, got, tc.want)
			}
		})
	}
}

func TestResolveUsageSourceMasksAPIKeyAccountInfo(t *testing.T) {
	auth := &cliproxyauth.Auth{
		Provider: "kiroaccountmanager",
		Attributes: map[string]string{
			"api_key": "any",
		},
	}

	got := resolveUsageSource(auth, "")
	if got != "a********y" {
		t.Fatalf("resolveUsageSource() = %q, want %q", got, "a********y")
	}
}

func TestResolveUsageSourceKeepsOAuthIdentity(t *testing.T) {
	auth := &cliproxyauth.Auth{
		Provider: "gemini-cli",
		Metadata: map[string]any{
			"email": "user@example.com",
		},
	}

	got := resolveUsageSource(auth, "")
	if got != "user@example.com" {
		t.Fatalf("resolveUsageSource() = %q, want %q", got, "user@example.com")
	}
}
