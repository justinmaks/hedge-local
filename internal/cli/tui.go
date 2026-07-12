package cli

import (
	"fmt"
	"os"

	"github.com/justinmaks/hedge-local/internal/config"
	"github.com/justinmaks/hedge-local/internal/store"
	"github.com/justinmaks/hedge-local/internal/tui"
	"github.com/justinmaks/hedge-local/internal/tui/queries"
	"github.com/justinmaks/hedge-local/internal/tui/views"
	"github.com/spf13/cobra"
)

var runTUIApp = tui.RunApp

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Run the hcli terminal UI (reads existing DB, no receiver)",
	RunE:  runTUI,
}

func init() {
	registerDefaultViews()
	rootCmd.AddCommand(tuiCmd)
}

func registerDefaultViews() {
	// CLI owns view registration so package tui does not import package views.
	tui.RegisterViewFactory(0, func(service *queries.Service) tui.View { return views.NewOverviewView(service) })
	tui.RegisterViewFactory(1, func(service *queries.Service) tui.View { return views.NewCostView(service) })
	tui.RegisterViewFactory(2, func(service *queries.Service) tui.View { return views.NewToolsView(service) })
	tui.RegisterViewFactory(3, func(service *queries.Service) tui.View { return views.NewModelsView(service) })
	tui.RegisterViewFactory(4, func(service *queries.Service) tui.View { return views.NewProjectsView(service) })
	tui.RegisterViewFactory(5, func(service *queries.Service) tui.View { return views.NewLiveView(service) })
	tui.RegisterViewFactory(6, func(service *queries.Service) tui.View { return views.NewExportView(service) })
}

func runTUI(cmd *cobra.Command, args []string) error {
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

	svc := queries.NewService(s)
	probe := collectorProbe(cfg.OTLPPort)
	return runTUIApp(svc, probe(), probe)
}

func loadCLIConfigAndDB() (*config.Config, string, error) {
	var cfg *config.Config
	var err error
	if cfgFile != "" {
		// An explicitly named config file must exist; a typo should not
		// silently fall back to defaults.
		cfg, err = config.LoadExplicit(cfgFile)
	} else {
		cfg, err = config.Load(config.DefaultPath())
	}
	if err != nil {
		return nil, "", fmt.Errorf("load config: %w", err)
	}
	for _, key := range cfg.UnknownKeys {
		fmt.Fprintf(os.Stderr, "warning: unknown config key %q\n", key)
	}

	db := dbPath
	if db == "" {
		if cfg.DBPath != "" {
			db = cfg.DBPath
		} else {
			db = config.DefaultDBPath()
		}
	}

	return cfg, db, nil
}
