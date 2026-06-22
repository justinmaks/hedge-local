package cli

import (
	"fmt"

	"github.com/justinmaks/hedge-local/internal/collect"
	"github.com/justinmaks/hedge-local/internal/normalize"
	"github.com/justinmaks/hedge-local/internal/store"
	"github.com/justinmaks/hedge-local/internal/tui/queries"
	"github.com/spf13/cobra"
)

var (
	cfgFile string
	dbPath  string
	verbose bool
	quiet   bool
)

var rootCmd = &cobra.Command{
	Use:   "hcli",
	Short: "Local-only telemetry TUI for coding agents",
	Long:  "hcli collects OpenTelemetry metrics from coding agents (Claude Code,\nOpenCode) into a local SQLite database and visualizes them in a TUI.\nRun hcli with no subcommand to start the TUI with an embedded receiver when available.",
	RunE:  runRoot,
}

var Execute = rootCmd.Execute

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default ~/.hedge/config.toml)")
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "", "database path (default ~/.hedge/hedge.db)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose logging")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "only show errors")
}

func runRoot(cmd *cobra.Command, args []string) error {
	cfg, db, err := loadCLIConfigAndDB()
	if err != nil {
		return err
	}

	s, err := store.New(db)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	if err := s.SeedBundledPricing(); err != nil {
		return fmt.Errorf("seed pricing: %w", err)
	}

	norm := normalize.NewCompositeNormalizer(
		&normalize.ClaudeCodeNormalizer{},
		&normalize.OpenCodeNormalizer{},
	)
	w := collect.NewWriter(s, cfg.WithLogs)
	r := collect.NewReceiver(norm, w, cfg.OTLPPort)

	if err := r.Start(); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not start receiver (%v). Running TUI without collection.\n", err)
		svc := queries.NewService(s)
		return runTUIApp(svc, false)
	}
	defer r.Stop()

	svc := queries.NewService(s)
	return runTUIApp(svc, true)
}
