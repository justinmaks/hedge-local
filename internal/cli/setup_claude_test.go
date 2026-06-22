package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

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
