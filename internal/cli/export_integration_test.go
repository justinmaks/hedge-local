package cli

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/justinmaks/hedge-local/internal/collect"
	"github.com/justinmaks/hedge-local/internal/normalize"
	"github.com/justinmaks/hedge-local/internal/store"
	"github.com/spf13/cobra"
)

func TestIntegration_exportJSON(t *testing.T) {
	databasePath := seedExportIntegrationStore(t)

	oldDB := dbPath
	oldCfg := cfgFile
	oldRange := exportRange
	oldFormat := exportFormat
	oldData := exportData
	oldOut := exportOut
	dbPath = databasePath
	cfgFile = ""
	exportRange = "7d"
	exportFormat = "json"
	exportData = "sessions"
	exportOut = "-"
	t.Cleanup(func() {
		dbPath = oldDB
		cfgFile = oldCfg
		exportRange = oldRange
		exportFormat = oldFormat
		exportData = oldData
		exportOut = oldOut
	})

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)
	if err := runExport(cmd, nil); err != nil {
		t.Fatalf("runExport: %v", err)
	}

	var rows []map[string]string
	if err := json.Unmarshal(buf.Bytes(), &rows); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, buf.String())
	}
	if len(rows) == 0 {
		t.Fatalf("expected at least 1 row, got 0")
	}
	if got := rows[0]["agent"]; got != "claude_code" {
		t.Fatalf("row agent: got %q, want %q", got, "claude_code")
	}
}

func seedExportIntegrationStore(t *testing.T) string {
	t.Helper()

	databasePath := t.TempDir() + "/test.db"
	s, err := store.New(databasePath)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()
	if err := s.SeedBundledPricing(); err != nil {
		t.Fatalf("SeedBundledPricing: %v", err)
	}

	w := collect.NewWriter(s, false)
	startedAt := time.Now().Add(-1 * time.Hour)
	if err := w.Write([]normalize.Event{{
		Type:        normalize.EventLLMCall,
		Timestamp:   startedAt,
		Agent:       "claude_code",
		SessionID:   "export-sess",
		ProjectPath: "/tmp/test",
		LLMCall: &normalize.LLMCallData{
			StartedAt:    startedAt,
			DurationMs:   1000,
			Model:        "claude-sonnet-4",
			Provider:     "anthropic",
			InputTokens:  500,
			OutputTokens: 200,
			StopReason:   "end_turn",
		},
	}}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	return databasePath
}
