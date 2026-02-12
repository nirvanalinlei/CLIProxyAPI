package management

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func TestNormalizeOpenAICompatibilityEntry_WireAPI(t *testing.T) {
	entry := config.OpenAICompatibility{WireAPI: " ReSpoNses "}
	normalizeOpenAICompatibilityEntry(&entry)
	if entry.WireAPI != "responses" {
		t.Fatalf("wire-api = %q", entry.WireAPI)
	}
}
