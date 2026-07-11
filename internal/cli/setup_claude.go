package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

func backupIfExists(path string, newContent []byte) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if bytes.Equal(data, newContent) {
		// File already matches what we are about to write; keep any
		// earlier backup instead of clobbering it with our own output.
		return nil
	}
	backupPath := path + ".backup"
	return os.WriteFile(backupPath, data, 0644)
}

// installSessionStartHook merges a SessionStart hook running
// "<hcli> session-start" into the Claude Code settings file, preserving all
// other settings. Telemetry carries no working directory, so this hook is
// the only wrapper-free source of per-project attribution. Returns true if
// the file was changed.
func installSessionStartHook(settingsPath, hcliPath string) (bool, error) {
	settings := map[string]any{}
	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return false, fmt.Errorf("parse %s (not modifying it): %w", settingsPath, err)
		}
	} else if !os.IsNotExist(err) {
		return false, err
	}

	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		if _, exists := settings["hooks"]; exists {
			return false, fmt.Errorf("%s has an unexpected hooks shape; not modifying it", settingsPath)
		}
		hooks = map[string]any{}
	}
	entries, ok := hooks["SessionStart"].([]any)
	if !ok {
		if _, exists := hooks["SessionStart"]; exists {
			return false, fmt.Errorf("%s has an unexpected SessionStart shape; not modifying it", settingsPath)
		}
		entries = []any{}
	}

	// Already installed (any command mentioning session-start counts, so a
	// user-edited path is left alone).
	raw, _ := json.Marshal(entries)
	if strings.Contains(string(raw), "session-start") {
		return false, nil
	}

	entries = append(entries, map[string]any{
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": hcliPath + " session-start",
			},
		},
	})
	hooks["SessionStart"] = entries
	settings["hooks"] = hooks

	updated, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return false, err
	}
	updated = append(updated, '\n')

	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		return false, err
	}
	if err := backupIfExists(settingsPath, updated); err != nil {
		return false, fmt.Errorf("backup settings: %w", err)
	}
	if err := os.WriteFile(settingsPath, updated, 0644); err != nil {
		return false, err
	}
	return true, nil
}

func runSetupClaude(cmd *cobra.Command, args []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	hedgeDir := filepath.Join(home, ".hedge")
	if err := mkdirSecure(hedgeDir); err != nil {
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
`

	if err := backupIfExists(envPath, []byte(content)); err != nil {
		return fmt.Errorf("backup env.sh: %w", err)
	}
	if err := os.WriteFile(envPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write env.sh: %w", err)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Wrote %s\n\n", envPath)

	hcliPath, err := os.Executable()
	if err != nil || hcliPath == "" {
		hcliPath = "hcli"
	}
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	changed, err := installSessionStartHook(settingsPath, hcliPath)
	if err != nil {
		fmt.Fprintf(out, "Warning: could not install the per-project hook: %v\n", err)
		fmt.Fprintf(out, "Sessions will appear under (ungrouped); see docs/advanced.md.\n\n")
	} else if changed {
		fmt.Fprintf(out, "Installed SessionStart hook in %s\n", settingsPath)
		fmt.Fprintf(out, "(attributes each session to the directory you start claude in)\n\n")
	} else {
		fmt.Fprintf(out, "SessionStart hook already installed in %s\n\n", settingsPath)
	}

	fmt.Fprintf(out, "Add this line to your shell rc file (~/.bashrc or ~/.zshrc):\n")
	fmt.Fprintf(out, "  source ~/.hedge/env.sh\n\n")
	fmt.Fprintf(out, "Then restart your shell or run: source ~/.hedge/env.sh\n\n")
	fmt.Fprintf(out, "Next: run 'hcli collect' in one terminal, then use Claude Code in another.\n")
	return nil
}
