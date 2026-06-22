package cli

import (
	"fmt"

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
	_, db, err := loadCLIConfigAndDB()
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
	return runTUIApp(svc, daemonRunning())
}

func loadCLIConfigAndDB() (*config.Config, string, error) {
	cfgPath := cfgFile
	if cfgPath == "" {
		cfgPath = config.DefaultPath()
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, "", fmt.Errorf("load config: %w", err)
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

func daemonRunning() bool {
	pid, err := readPIDFile(defaultPIDPath())
	return err == nil && processAlive(pid)
}
