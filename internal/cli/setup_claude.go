package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure agents to send telemetry to hcli",
}

var setupClaudeCmd = &cobra.Command{
	Use:   "claude",
	Short: "Configure Claude Code to send OTEL telemetry to hcli",
	Long:  "Writes OTEL environment variables to ~/.hedge/env.sh and prints\nshell-rc instructions to activate them.",
	RunE:  runSetupClaude,
}

func init() {
	setupCmd.AddCommand(setupClaudeCmd)
	rootCmd.AddCommand(setupCmd)
}

func runSetupClaude(cmd *cobra.Command, args []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	hedgeDir := filepath.Join(home, ".hedge")
	if err := os.MkdirAll(hedgeDir, 0755); err != nil {
		return fmt.Errorf("create ~/.hedge: %w", err)
	}

	envPath := filepath.Join(hedgeDir, "env.sh")
	content := `# hcli telemetry environment for Claude Code
# Source this from your shell rc (~/.bashrc, ~/.zshrc):
#   source ~/.hedge/env.sh

export CLAUDE_CODE_ENABLE_TELEMETRY=1
export CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1
export OTEL_METRICS_EXPORTER=otlp
export OTEL_TRACES_EXPORTER=otlp
export OTEL_LOGS_EXPORTER=none
export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
export OTEL_METRIC_EXPORT_INTERVAL=5000
export OTEL_LOG_TOOL_DETAILS=1

# For per-project attribution, uncomment and use as a shell function:
# claude() { OTEL_RESOURCE_ATTRIBUTES="hcli.project_path=$PWD" command claude "$@"; }
`

	if err := os.WriteFile(envPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write env.sh: %w", err)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Wrote %s\n\n", envPath)
	fmt.Fprintf(out, "Add this line to your shell rc file (~/.bashrc or ~/.zshrc):\n")
	fmt.Fprintf(out, "  source ~/.hedge/env.sh\n\n")
	fmt.Fprintf(out, "Then restart your shell or run: source ~/.hedge/env.sh\n\n")
	fmt.Fprintf(out, "For per-project attribution, uncomment the claude() wrapper function\n")
	fmt.Fprintf(out, "in %s (recommended).\n\n", envPath)
	fmt.Fprintf(out, "Next: run 'hcli collect' in one terminal, then use Claude Code in another.\n")
	return nil
}
