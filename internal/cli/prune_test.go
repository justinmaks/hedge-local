package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/justinmaks/hedge-local/internal/store"
	"github.com/spf13/cobra"
)

func TestParseRetention(t *testing.T) {
	cases := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"90d", 90 * 24 * time.Hour, false},
		{"12w", 12 * 7 * 24 * time.Hour, false},
		{"1D", 24 * time.Hour, false},
		{"", 0, true},
		{"d", 0, true},
		{"90", 0, true},
		{"-5d", 0, true},
		{"90h", 0, true},
	}
	for _, c := range cases {
		got, err := parseRetention(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseRetention(%q): expected error", c.in)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("parseRetention(%q): got %v err=%v, want %v", c.in, got, err, c.want)
		}
	}
}

func setupPruneEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	oldOlder, oldDry, oldYes, oldVac := pruneOlderThan, pruneDryRun, pruneYes, pruneVacuum
	oldCfg, oldDB := cfgFile, dbPath
	t.Cleanup(func() {
		pruneOlderThan, pruneDryRun, pruneYes, pruneVacuum = oldOlder, oldDry, oldYes, oldVac
		cfgFile, dbPath = oldCfg, oldDB
	})
	cfgFile, dbPath = "", ""

	db := filepath.Join(home, ".hedge", "hedge.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	pid, _ := s.ProjectUpsert("/repo", "repo")
	now := time.Now()
	old := now.Add(-100 * 24 * time.Hour)
	oldSess, _ := s.SessionUpsert("ext-old", "claude_code", pid, old, "1.0")
	newSess, _ := s.SessionUpsert("ext-new", "claude_code", pid, now, "1.0")
	if _, err := s.LLMCallInsert(store.LLMCallParams{
		SessionID: oldSess, SpanID: "sp-old", StartedAt: old, Agent: "claude_code", Model: "m",
	}); err != nil {
		t.Fatalf("old llm: %v", err)
	}
	if _, err := s.LLMCallInsert(store.LLMCallParams{
		SessionID: newSess, SpanID: "sp-new", StartedAt: now, Agent: "claude_code", Model: "m",
	}); err != nil {
		t.Fatalf("new llm: %v", err)
	}
	return db
}

func pruneRowCount(t *testing.T, dbPath, table string) int {
	t.Helper()
	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer s.Close()
	var n int
	s.DB().QueryRow("SELECT count(*) FROM " + table).Scan(&n)
	return n
}

func TestPrune_deletesWithYes(t *testing.T) {
	db := setupPruneEnv(t)
	pruneOlderThan, pruneYes, pruneDryRun, pruneVacuum = "90d", true, false, false

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	if err := runPrune(cmd, nil); err != nil {
		t.Fatalf("runPrune: %v", err)
	}
	if got := pruneRowCount(t, db, "llm_calls"); got != 1 {
		t.Errorf("llm_calls after prune: got %d, want 1", got)
	}
	if got := pruneRowCount(t, db, "sessions"); got != 1 {
		t.Errorf("sessions after prune: got %d, want 1", got)
	}
	if !strings.Contains(out.String(), "Pruned 1 llm calls") {
		t.Errorf("output should report pruned rows:\n%s", out.String())
	}
}

func TestPrune_dryRunDeletesNothing(t *testing.T) {
	db := setupPruneEnv(t)
	pruneOlderThan, pruneYes, pruneDryRun, pruneVacuum = "90d", false, true, false

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	if err := runPrune(cmd, nil); err != nil {
		t.Fatalf("runPrune dry-run: %v", err)
	}
	if got := pruneRowCount(t, db, "llm_calls"); got != 2 {
		t.Errorf("dry-run should keep all llm_calls, got %d", got)
	}
	if !strings.Contains(out.String(), "Dry run") {
		t.Errorf("output should say dry run:\n%s", out.String())
	}
}

func TestPrune_promptDeclinedKeepsRows(t *testing.T) {
	db := setupPruneEnv(t)
	pruneOlderThan, pruneYes, pruneDryRun, pruneVacuum = "90d", false, false, false

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	cmd.SetIn(strings.NewReader("n\n"))
	if err := runPrune(cmd, nil); err != nil {
		t.Fatalf("runPrune: %v", err)
	}
	if got := pruneRowCount(t, db, "llm_calls"); got != 2 {
		t.Errorf("declined prompt should keep rows, got %d", got)
	}
	if !strings.Contains(out.String(), "Aborted") {
		t.Errorf("output should say aborted:\n%s", out.String())
	}
}

func TestPrune_noWindowErrors(t *testing.T) {
	setupPruneEnv(t)
	pruneOlderThan, pruneYes, pruneDryRun, pruneVacuum = "", true, false, false

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	if err := runPrune(cmd, nil); err == nil {
		t.Fatal("expected error when no --older-than and no retention_days")
	}
}

func TestPrune_usesRetentionDaysFromConfig(t *testing.T) {
	db := setupPruneEnv(t)
	home := os.Getenv("HOME")
	cfgDir := filepath.Join(home, ".hedge")
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte("retention_days = 90\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	pruneOlderThan, pruneYes, pruneDryRun, pruneVacuum = "", true, false, false

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	if err := runPrune(cmd, nil); err != nil {
		t.Fatalf("runPrune with config retention: %v", err)
	}
	if got := pruneRowCount(t, db, "llm_calls"); got != 1 {
		t.Errorf("config-driven prune should delete old rows, got %d", got)
	}
}
