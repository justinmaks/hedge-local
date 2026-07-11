package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/justinmaks/hedge-local/internal/store"
	"github.com/spf13/cobra"
)

func sessionStartEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	oldCfg, oldDB := cfgFile, dbPath
	t.Cleanup(func() { cfgFile, dbPath = oldCfg, oldDB })
	cfgFile, dbPath = "", ""
	return filepath.Join(home, ".hedge", "hedge.db")
}

func runSessionStartWith(t *testing.T, payload string) {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetIn(strings.NewReader(payload))
	if err := runSessionStart(cmd, nil); err != nil {
		t.Fatalf("runSessionStart: %v", err)
	}
}

func TestSessionStart_hookBeforeTelemetry(t *testing.T) {
	db := sessionStartEnv(t)

	runSessionStartWith(t, `{"session_id":"sess-hook-1","cwd":"/home/u/repos/myproject","hook_event_name":"SessionStart"}`)

	// Telemetry arrives later and must keep the attribution.
	s, err := store.New(db)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()
	if _, err := s.SessionUpsert("sess-hook-1", "claude_code", 0, time.Now(), ""); err != nil {
		t.Fatalf("telemetry upsert: %v", err)
	}

	var name string
	err = s.DB().QueryRow(
		`SELECT p.name FROM sessions s JOIN projects p ON s.project_id = p.id WHERE s.external_id = 'sess-hook-1'`,
	).Scan(&name)
	if err != nil {
		t.Fatalf("query attribution: %v", err)
	}
	if name != "myproject" {
		t.Errorf("project: got %q, want myproject", name)
	}
}

func TestSessionStart_hookAfterTelemetry(t *testing.T) {
	db := sessionStartEnv(t)

	// Telemetry creates the session first, unattributed.
	s, err := store.New(db)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	pid, _ := s.ProjectUpsert("(ungrouped)", "(ungrouped)")
	if _, err := s.SessionUpsert("sess-hook-2", "claude_code", pid, time.Now(), ""); err != nil {
		t.Fatalf("telemetry upsert: %v", err)
	}
	s.Close()

	runSessionStartWith(t, `{"session_id":"sess-hook-2","cwd":"/home/u/repos/otherproject"}`)

	s2, err := store.New(db)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer s2.Close()
	var name string
	err = s2.DB().QueryRow(
		`SELECT p.name FROM sessions s JOIN projects p ON s.project_id = p.id WHERE s.external_id = 'sess-hook-2'`,
	).Scan(&name)
	if err != nil {
		t.Fatalf("query attribution: %v", err)
	}
	if name != "otherproject" {
		t.Errorf("project: got %q, want otherproject", name)
	}
}

func TestSessionStart_emptyPayloadIsQuietNoop(t *testing.T) {
	sessionStartEnv(t)
	runSessionStartWith(t, `{}`)
	runSessionStartWith(t, `{"session_id":"x"}`)
}

func TestInstallSessionStartHook_freshFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	changed, err := installSessionStartHook(path, "/usr/local/bin/hcli")
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if !changed {
		t.Fatal("fresh install should report changed")
	}

	data, _ := os.ReadFile(path)
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if !strings.Contains(string(data), "/usr/local/bin/hcli session-start") {
		t.Errorf("hook command missing:\n%s", data)
	}

	// Idempotent.
	changed, err = installSessionStartHook(path, "/usr/local/bin/hcli")
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	if changed {
		t.Error("second install should be a no-op")
	}
}

func TestInstallSessionStartHook_preservesExistingSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	existing := `{
  "model": "claude-fable-5",
  "enabledPlugins": {"some@plugin": true},
  "hooks": {
    "SessionStart": [
      {"hooks": [{"type": "command", "command": "echo existing"}]}
    ],
    "PreToolUse": [
      {"matcher": "Bash", "hooks": [{"type": "command", "command": "echo pre"}]}
    ]
  }
}`
	if err := os.WriteFile(path, []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	changed, err := installSessionStartHook(path, "hcli")
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if !changed {
		t.Fatal("should have appended the hook")
	}

	data, _ := os.ReadFile(path)
	out := string(data)
	for _, want := range []string{"claude-fable-5", "some@plugin", "echo existing", "PreToolUse", "echo pre", "hcli session-start"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q after merge:\n%s", want, out)
		}
	}
}

func TestInstallSessionStartHook_invalidJSONUntouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := installSessionStartHook(path, "hcli"); err == nil {
		t.Fatal("invalid settings should error, not be overwritten")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "{not json" {
		t.Error("invalid settings file was modified")
	}
}
