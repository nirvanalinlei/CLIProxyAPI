package management

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func TestPatchOpenAICompat_BaseURLEmptyRemovesEntry(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		OpenAICompatibility: []config.OpenAICompatibility{
			{Name: "p1", BaseURL: "http://base"},
		},
	}
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("openai-compatibility: []\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	h := NewHandler(cfg, configPath, nil)

	body := `{"index":0,"value":{"base-url":"   "}}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPatch, "/api/management/openai-compatibility", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.PatchOpenAICompat(c)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", w.Code)
	}
	if len(cfg.OpenAICompatibility) != 0 {
		t.Fatalf("expected entry removed: %+v", cfg.OpenAICompatibility)
	}
}
