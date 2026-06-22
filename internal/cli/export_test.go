package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/justinmaks/hedge-local/internal/collect"
	"github.com/justinmaks/hedge-local/internal/normalize"
	"github.com/justinmaks/hedge-local/internal/store"
	"github.com/spf13/cobra"
)

func TestRunExportWritesSessionsCSVToStdout(t *testing.T) {
	db := seedExportTestStore(t)

	oldDB := dbPath
	oldCfg := cfgFile
	oldRange := exportRange
	oldFormat := exportFormat
	oldData := exportData
	oldOut := exportOut
	dbPath = db
	cfgFile = ""
	exportRange = "7d"
	exportFormat = "csv"
	exportData = "sessions"
	exportOut = ""
	t.Cleanup(func() {
		dbPath = oldDB
		cfgFile = oldCfg
		exportRange = oldRange
		exportFormat = oldFormat
		exportData = oldData
		exportOut = oldOut
	})

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	if err := runExport(cmd, nil); err != nil {
		t.Fatalf("runExport: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "session_id,agent,project,started_at,ended_at,cost_usd,input_tokens,output_tokens,tool_calls,message_count") {
		t.Fatalf("stdout missing csv header: %s", got)
	}
	if !strings.Contains(got, "sess-export,claude_code,/tmp/export-project") {
		t.Fatalf("stdout missing exported session row: %s", got)
	}
}

func TestRunExportWritesWeeklyReportToFile(t *testing.T) {
	db := seedExportTestStore(t)
	outPath := filepath.Join(t.TempDir(), "weekly.md")

	oldDB := dbPath
	oldCfg := cfgFile
	oldRange := exportRange
	oldFormat := exportFormat
	oldData := exportData
	oldOut := exportOut
	dbPath = db
	cfgFile = ""
	exportRange = "7d"
	exportFormat = "json"
	exportData = "weekly_report"
	exportOut = outPath
	t.Cleanup(func() {
		dbPath = oldDB
		cfgFile = oldCfg
		exportRange = oldRange
		exportFormat = oldFormat
		exportData = oldData
		exportOut = oldOut
	})

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	if err := runExport(cmd, nil); err != nil {
		t.Fatalf("runExport: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "# hcli Weekly Report") {
		t.Fatalf("weekly report missing heading: %s", text)
	}
	if !strings.Contains(text, "## By Agent") {
		t.Fatalf("weekly report missing by-agent section: %s", text)
	}
	if !strings.Contains(out.String(), "Exported to "+outPath) {
		t.Fatalf("command output missing file path: %s", out.String())
	}
}

func seedExportTestStore(t *testing.T) string {
	t.Helper()
	db := filepath.Join(t.TempDir(), "export.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()
	if err := s.SeedBundledPricing(); err != nil {
		t.Fatalf("SeedBundledPricing: %v", err)
	}

	w := collect.NewWriter(s, true)
	startedAt := time.Now().Add(-2 * time.Hour)
	events := []normalize.Event{
		{
			Type:        normalize.EventLLMCall,
			Timestamp:   startedAt,
			Agent:       "claude_code",
			SessionID:   "sess-export",
			ProjectPath: "/tmp/export-project",
			LLMCall: &normalize.LLMCallData{
				StartedAt:    startedAt,
				DurationMs:   1500,
				Model:        "claude-sonnet-4",
				Provider:     "anthropic",
				InputTokens:  1000,
				OutputTokens: 500,
				StopReason:   "end_turn",
			},
		},
		{
			Type:        normalize.EventToolCall,
			Timestamp:   startedAt.Add(2 * time.Minute),
			Agent:       "claude_code",
			SessionID:   "sess-export",
			ProjectPath: "/tmp/export-project",
			ToolCall: &normalize.ToolCallData{
				StartedAt:  startedAt.Add(2 * time.Minute),
				DurationMs: 250,
				ToolName:   "bash",
				Success:    true,
			},
		},
		{
			Type:        normalize.EventLog,
			Timestamp:   startedAt.Add(3 * time.Minute),
			Agent:       "claude_code",
			SessionID:   "sess-export",
			ProjectPath: "/tmp/export-project",
			Log: &normalize.LogData{
				Name:    "message",
				Payload: []byte(`{"text":"hello"}`),
			},
		},
	}
	if err := w.Write(events); err != nil {
		t.Fatalf("Write: %v", err)
	}

	return db
}
