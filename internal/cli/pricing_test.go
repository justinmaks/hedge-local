package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justinmaks/hedge-local/internal/store"
	"github.com/spf13/cobra"
)

func TestRunPricingImportAndList(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "pricing.db")
	jsonPath := filepath.Join(dir, "pricing.json")
	data := `[{"provider":"openai","model":"gpt-4.1","input_per_1m":2,"output_per_1m":8,"cache_read_per_1m":0,"cache_write_per_1m":0,"effective_from":"2026-01-01T00:00:00Z"}]`
	if err := os.WriteFile(jsonPath, []byte(data), 0644); err != nil {
		t.Fatalf("write pricing json: %v", err)
	}

	oldDB := dbPath
	dbPath = db
	t.Cleanup(func() { dbPath = oldDB })

	if err := runPricingImport(&cobra.Command{}, []string{jsonPath}); err != nil {
		t.Fatalf("runPricingImport: %v", err)
	}

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	if err := runPricingList(cmd, nil); err != nil {
		t.Fatalf("runPricingList: %v", err)
	}
	if !strings.Contains(out.String(), "openai") || !strings.Contains(out.String(), "gpt-4.1") {
		t.Fatalf("list output missing pricing row: %s", out.String())
	}
}

func TestRunPricingFetch(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "pricing.db")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"provider":"openai","model":"gpt-4.1-mini","input_per_1m":0.4,"output_per_1m":1.6,"cache_read_per_1m":0,"cache_write_per_1m":0,"effective_from":"2026-01-01T00:00:00Z"}]`))
	}))
	defer server.Close()

	oldDB := dbPath
	oldURL := pricingFetchURL
	dbPath = db
	pricingFetchURL = server.URL
	t.Cleanup(func() { dbPath = oldDB; pricingFetchURL = oldURL })

	if err := runPricingFetch(&cobra.Command{}, nil); err != nil {
		t.Fatalf("runPricingFetch: %v", err)
	}
	s, err := store.New(db)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()
	rows, err := s.ListPricing()
	if err != nil {
		t.Fatalf("ListPricing: %v", err)
	}
	if len(rows) != 1 || rows[0].Source != "fetched" {
		t.Fatalf("rows: %#v", rows)
	}
}

func TestRunPricingListRespectsConfigDBPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	customDB := filepath.Join(dir, "custom.db")
	cfgPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("db_path = \""+customDB+"\"\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	jsonPath := filepath.Join(dir, "pricing.json")
	data := `[{"provider":"anthropic","model":"claude-sonnet-4","input_per_1m":3,"output_per_1m":15,"cache_read_per_1m":0.3,"cache_write_per_1m":3.75,"effective_from":"2026-01-01T00:00:00Z"}]`
	if err := os.WriteFile(jsonPath, []byte(data), 0644); err != nil {
		t.Fatalf("write pricing json: %v", err)
	}

	oldDB := dbPath
	oldCfg := cfgFile
	dbPath = ""
	cfgFile = cfgPath
	t.Cleanup(func() { dbPath = oldDB; cfgFile = oldCfg })

	if err := runPricingImport(&cobra.Command{}, []string{jsonPath}); err != nil {
		t.Fatalf("runPricingImport: %v", err)
	}

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	if err := runPricingList(cmd, nil); err != nil {
		t.Fatalf("runPricingList: %v", err)
	}
	if !strings.Contains(out.String(), "anthropic") || !strings.Contains(out.String(), "claude-sonnet-4") {
		t.Fatalf("list output missing pricing row from custom db: %s", out.String())
	}
	if _, err := os.Stat(customDB); err != nil {
		t.Fatalf("custom db not created (config db_path ignored): %v", err)
	}
}

func TestRunPricingListDoesNotSeedBundled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	db := filepath.Join(dir, "pricing.db")

	oldDB := dbPath
	dbPath = db
	t.Cleanup(func() { dbPath = oldDB })

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	if err := runPricingList(cmd, nil); err != nil {
		t.Fatalf("runPricingList: %v", err)
	}
	output := out.String()
	if strings.Contains(output, "anthropic") || strings.Contains(output, "openai") {
		t.Fatalf("list should not seed bundled pricing, but found rows: %s", output)
	}
}

func TestRunPricingFetchNon2xx(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	db := filepath.Join(dir, "pricing.db")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	oldDB := dbPath
	oldURL := pricingFetchURL
	dbPath = db
	pricingFetchURL = server.URL
	t.Cleanup(func() { dbPath = oldDB; pricingFetchURL = oldURL })

	err := runPricingFetch(&cobra.Command{}, nil)
	if err == nil {
		t.Fatalf("expected error for non-2xx response, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected error to mention status 404, got: %v", err)
	}
}

func TestRunPricingFetchRejectsBadScheme(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	db := filepath.Join(dir, "pricing.db")

	oldDB := dbPath
	oldURL := pricingFetchURL
	dbPath = db
	pricingFetchURL = "ftp://example.com/pricing.json"
	t.Cleanup(func() { dbPath = oldDB; pricingFetchURL = oldURL })

	err := runPricingFetch(&cobra.Command{}, nil)
	if err == nil {
		t.Fatalf("expected error for bad URL scheme, got nil")
	}
	if !strings.Contains(err.Error(), "http://") && !strings.Contains(err.Error(), "https://") {
		t.Fatalf("expected error to mention http/https, got: %v", err)
	}
}
