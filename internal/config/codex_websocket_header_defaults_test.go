package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigOptional_CodexHeaderDefaults(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configYAML := []byte(`
codex-header-defaults:
  user-agent: "  my-codex-client/1.0  "
  beta-features: "  feature-a,feature-b  "
  desktop-cloak: true
  version: "  0.118.0-alpha.2  "
`)
	if err := os.WriteFile(configPath, configYAML, 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := LoadConfigOptional(configPath, false)
	if err != nil {
		t.Fatalf("LoadConfigOptional() error = %v", err)
	}

	if got := cfg.CodexHeaderDefaults.UserAgent; got != "my-codex-client/1.0" {
		t.Fatalf("UserAgent = %q, want %q", got, "my-codex-client/1.0")
	}
	if got := cfg.CodexHeaderDefaults.BetaFeatures; got != "feature-a,feature-b" {
		t.Fatalf("BetaFeatures = %q, want %q", got, "feature-a,feature-b")
	}
	if cfg.CodexHeaderDefaults.DesktopCloak == nil || !*cfg.CodexHeaderDefaults.DesktopCloak {
		t.Fatalf("DesktopCloak = %v, want true", cfg.CodexHeaderDefaults.DesktopCloak)
	}
	if got := cfg.CodexHeaderDefaults.Version; got != "0.118.0-alpha.2" {
		t.Fatalf("Version = %q, want %q", got, "0.118.0-alpha.2")
	}
}
