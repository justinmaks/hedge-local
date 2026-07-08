package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var uninstallYes bool
var uninstallDryRun bool

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove hcli data and configuration from this machine",
	Long: `Removes the ~/.hedge/ directory (database, config, env files, logs).

The binary itself is NOT removed — uninstall it via your package manager
(go install, brew, apt, dnf) or delete it manually.

Manual cleanup still needed after running this command:
  - Remove 'source ~/.hedge/env.sh' from your shell rc file
  - Remove 'source ~/.hedge/opencode-env.sh' from your shell rc file
  - Remove the @devtheops/opencode-plugin-otel entry from opencode.json
    (only if you no longer need OTEL for other tools)`,
	RunE: runUninstall,
}

func init() {
	uninstallCmd.Flags().BoolVar(&uninstallYes, "yes", false, "skip confirmation prompt")
	uninstallCmd.Flags().BoolVar(&uninstallDryRun, "dry-run", false, "show what would be removed without doing it")
	rootCmd.AddCommand(uninstallCmd)
}

func runUninstall(cmd *cobra.Command, args []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	hedgeDir := filepath.Join(home, ".hedge")

	if _, err := os.Stat(hedgeDir); os.IsNotExist(err) {
		fmt.Fprintln(cmd.OutOrStdout(), "Nothing to remove — ~/.hedge/ does not exist.")
		return nil
	}

	entries, err := os.ReadDir(hedgeDir)
	if err != nil {
		return fmt.Errorf("read ~/.hedge: %w", err)
	}

	out := cmd.OutOrStdout()

	if uninstallDryRun {
		fmt.Fprintf(out, "Dry run — would remove %s/ containing:\n", hedgeDir)
		for _, e := range entries {
			fmt.Fprintf(out, "  %s\n", e.Name())
		}
		fmt.Fprintln(out)
		printManualCleanup(out)
		return nil
	}

	if err := os.RemoveAll(hedgeDir); err != nil {
		return fmt.Errorf("remove %s: %w", hedgeDir, err)
	}

	fmt.Fprintf(out, "Removed %s/\n\n", hedgeDir)
	printManualCleanup(out)
	return nil
}

func printManualCleanup(out interface{ Write([]byte) (int, error) }) {
	fmt.Fprintln(out, "Manual cleanup still needed:")
	fmt.Fprintln(out, "  1. Remove 'source ~/.hedge/env.sh' from your shell rc (~/.bashrc, ~/.zshrc)")
	fmt.Fprintln(out, "  2. Remove 'source ~/.hedge/opencode-env.sh' from your shell rc (if used)")
	fmt.Fprintln(out, "  3. Remove @devtheops/opencode-plugin-otel from opencode.json (if no longer needed)")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "To remove the binary:")
	fmt.Fprintln(out, "  go install:    rm $(which hcli)")
	fmt.Fprintln(out, "  brew:          brew uninstall hcli")
	fmt.Fprintln(out, "  .deb/.rpm:     dpkg -r hcli / rpm -e hcli")
	fmt.Fprintln(out, "  manual:        rm /usr/local/bin/hcli")
}
