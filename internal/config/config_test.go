package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_defaultsWhenNoFile(t *testing.T) {
	cfg, err := Load("/nonexistent/config.toml")
	if err != nil {
		t.Fatalf("Load should succeed with defaults when file missing: %v", err)
	}
	if cfg.DBPath != "" {
		t.Errorf("expected empty DBPath when no config, got %q", cfg.DBPath)
	}
	if cfg.OTLPPort != 4318 {
		t.Errorf("expected default OTLPPort 4318, got %d", cfg.OTLPPort)
	}
	if cfg.WithLogs {
		t.Error("expected WithLogs=false by default")
	}
}

func TestLoad_readsTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
db_path = "/custom/hedge.db"
otlp_port = 5555
with_logs = true
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.DBPath != "/custom/hedge.db" {
		t.Errorf("expected DBPath /custom/hedge.db, got %q", cfg.DBPath)
	}
	if cfg.OTLPPort != 5555 {
		t.Errorf("expected OTLPPort 5555, got %d", cfg.OTLPPort)
	}
	if !cfg.WithLogs {
		t.Error("expected WithLogs=true")
	}
}

func TestDefaultPath(t *testing.T) {
	t.Setenv("HOME", "/tmp/fakehome")
	got := DefaultPath()
	want := "/tmp/fakehome/.hedge/config.toml"
	if got != want {
		t.Errorf("DefaultPath() = %q, want %q", got, want)
	}
}
