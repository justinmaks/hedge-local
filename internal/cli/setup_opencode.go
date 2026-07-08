package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

const opencodePluginName = "@devtheops/opencode-plugin-otel"

var opencodeConfigPath string

var setupOpenCodeCmd = &cobra.Command{
	Use:   "opencode",
	Short: "Configure OpenCode to send OTEL telemetry to hcli",
	RunE:  runSetupOpenCode,
}

func init() {
	setupOpenCodeCmd.Flags().StringVar(&opencodeConfigPath, "config", "", "OpenCode config path (default ~/.config/opencode/opencode.json)")
	setupCmd.AddCommand(setupOpenCodeCmd)
}

func runSetupOpenCode(cmd *cobra.Command, args []string) error {
	configPath := opencodeConfigPath
	if configPath == "" {
		configPath = defaultOpenCodeConfigPath()
	}
	envPath := filepath.Join(defaultHedgeDir(), "opencode-env.sh")
	if err := configureOpenCode(configPath, envPath); err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Updated %s\n", configPath)
	fmt.Fprintf(out, "Wrote %s\n\n", envPath)
	fmt.Fprintf(out, "Next steps:\n")
	fmt.Fprintf(out, "  source ~/.hedge/opencode-env.sh\n")
	fmt.Fprintf(out, "  hcli collect\n")
	fmt.Fprintf(out, "  opencode\n")
	return nil
}

func configureOpenCode(configPath, envPath string) error {
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("create opencode config dir: %w", err)
	}
	if err := mkdirSecure(filepath.Dir(envPath)); err != nil {
		return fmt.Errorf("create env dir: %w", err)
	}

	cfg := map[string]any{"$schema": "https://opencode.ai/config.json"}
	if data, err := os.ReadFile(configPath); err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("parse opencode config: %w", err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read opencode config: %w", err)
	}

	plugins := stringSlice(cfg["plugin"])
	if !containsString(plugins, opencodePluginName) {
		plugins = append(plugins, opencodePluginName)
	}
	cfg["plugin"] = plugins

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal opencode config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("write opencode config: %w", err)
	}

	if err := backupIfExists(envPath); err != nil {
		return fmt.Errorf("backup opencode env: %w", err)
	}
	env := `# hcli telemetry environment for OpenCode
# Source this from your shell rc (~/.bashrc, ~/.zshrc):
#   source ~/.hedge/opencode-env.sh

export OPENCODE_ENABLE_TELEMETRY=1
export OPENCODE_OTLP_ENDPOINT=http://localhost:4318
export OPENCODE_OTLP_PROTOCOL=http/protobuf

# For per-project attribution, uncomment and use as a shell function:
# opencode() { OPENCODE_RESOURCE_ATTRIBUTES="hcli.project_path=$PWD" command opencode "$@"; }
`
	if err := os.WriteFile(envPath, []byte(env), 0644); err != nil {
		return fmt.Errorf("write opencode env: %w", err)
	}
	return nil
}

func stringSlice(v any) []string {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	var result []string
	for _, item := range items {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func defaultOpenCodeConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".config", "opencode", "opencode.json")
}

func defaultHedgeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".hedge")
}
