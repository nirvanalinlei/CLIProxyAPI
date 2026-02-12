package config

import "testing"

func TestSanitizeOpenAICompatibility_WireAPI(t *testing.T) {
	cfg := &Config{OpenAICompatibility: []OpenAICompatibility{{
		Name:    "p1",
		BaseURL: "https://example.com",
		WireAPI: " ReSpoNses ",
	}, {
		Name:    "p2",
		BaseURL: "https://example.com",
		WireAPI: "unknown",
	}, {
		Name:    "p3",
		BaseURL: "https://example.com",
		WireAPI: "  ",
	}}}

	cfg.SanitizeOpenAICompatibility()

	if got := cfg.OpenAICompatibility[0].WireAPI; got != "responses" {
		t.Fatalf("wire-api normalize = %q", got)
	}
	if got := cfg.OpenAICompatibility[1].WireAPI; got != "chat" {
		t.Fatalf("wire-api default = %q", got)
	}
	if got := cfg.OpenAICompatibility[2].WireAPI; got != "" {
		t.Fatalf("wire-api empty = %q", got)
	}
}
