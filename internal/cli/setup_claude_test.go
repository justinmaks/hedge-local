package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestSetupClaude_backsUpExistingEnvSh(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	envDir := filepath.Join(home, ".hedge")
	if err := os.MkdirAll(envDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	original := "# my custom env\nexport FOO=bar\n"
	if err := os.WriteFile(filepath.Join(envDir, "env.sh"), []byte(original), 0644); err != nil {
		t.Fatalf("write original: %v", err)
	}

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	if err := runSetupClaude(cmd, nil); err != nil {
		t.Fatalf("runSetupClaude: %v", err)
	}

	backupData, err := os.ReadFile(filepath.Join(envDir, "env.sh.backup"))
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(backupData) != original {
		t.Fatalf("backup content mismatch: got %q", string(backupData))
	}

	mainData, err := os.ReadFile(filepath.Join(envDir, "env.sh"))
	if err != nil {
		t.Fatalf("read env.sh: %v", err)
	}
	if !strings.Contains(string(mainData), "CLAUDE_CODE_ENABLE_TELEMETRY") {
		t.Fatalf("env.sh should contain telemetry vars: %s", string(mainData))
	}
}

func TestSetupClaude_noBackupWhenFresh(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	if err := runSetupClaude(cmd, nil); err != nil {
		t.Fatalf("runSetupClaude: %v", err)
	}

	backupPath := filepath.Join(home, ".hedge", "env.sh.backup")
	if _, err := os.Stat(backupPath); err == nil {
		t.Fatal("backup should not exist when no prior file")
	}
}

func TestSetupClaudeWritesTelemetryEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	if err := runSetupClaude(cmd, nil); err != nil {
		t.Fatalf("runSetupClaude: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, ".hedge", "env.sh"))
	if err != nil {
		t.Fatalf("read env.sh: %v", err)
	}
	content := string(data)
	for _, want := range []string{
		"export CLAUDE_CODE_ENABLE_TELEMETRY=1",
		"export CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1",
		"export OTEL_METRICS_EXPORTER=otlp",
		"export OTEL_TRACES_EXPORTER=otlp",
		"export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf",
		"export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318",
		"export OTEL_METRIC_EXPORT_INTERVAL=5000",
		"export OTEL_LOG_TOOL_DETAILS=1",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("env.sh missing %q:\n%s", want, content)
		}
	}
}
