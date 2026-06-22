package cli

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/justinmaks/hedge-local/internal/collect"
	"github.com/justinmaks/hedge-local/internal/config"
	"github.com/justinmaks/hedge-local/internal/normalize"
	"github.com/justinmaks/hedge-local/internal/store"
	"github.com/spf13/cobra"
)

var (
	collectWithLogs bool
	collectPort     int
	collectDaemon   bool
)

var collectCmd = &cobra.Command{
	Use:   "collect",
	Short: "Run the OTLP receiver daemon to collect agent telemetry",
	Long:  "Starts an OTLP/HTTP receiver on localhost that accepts telemetry\nfrom Claude Code and OpenCode, normalizes it, and writes to SQLite.",
	RunE:  runCollect,
}

func init() {
	collectCmd.Flags().BoolVar(&collectWithLogs, "with-logs", false, "also capture log events (full prompts)")
	collectCmd.Flags().IntVar(&collectPort, "port", 0, "OTLP listen port (overrides config, default 4318)")
	collectCmd.Flags().BoolVarP(&collectDaemon, "daemon", "d", false, "run in background (daemon mode)")
	rootCmd.AddCommand(collectCmd)
}

func runCollect(cmd *cobra.Command, args []string) error {
	if collectDaemon {
		return forkDaemon(cmd, args)
	}

	cfgPath := cfgFile
	if cfgPath == "" {
		cfgPath = config.DefaultPath()
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	db := dbPath
	if db == "" {
		if cfg.DBPath != "" {
			db = cfg.DBPath
		} else {
			db = config.DefaultDBPath()
		}
	}

	s, err := store.New(db)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	if err := s.SeedBundledPricing(); err != nil {
		return fmt.Errorf("seed pricing: %w", err)
	}

	port := collectPort
	if port == 0 {
		port = cfg.OTLPPort
	}

	withLogs := collectWithLogs || cfg.WithLogs

	norm := normalize.NewCompositeNormalizer(
		&normalize.ClaudeCodeNormalizer{},
		&normalize.OpenCodeNormalizer{},
	)
	w := collect.NewWriter(s, withLogs)
	r := collect.NewReceiver(norm, w, port)

	if err := r.Start(); err != nil {
		return fmt.Errorf("start receiver: %w", err)
	}
	defer func() { _ = r.Stop() }()

	if os.Getenv("HCLI_DAEMON_CHILD") == "1" {
		pidPath := defaultPIDPath()
		_ = writePIDFile(pidPath, os.Getpid())
		defer func() { _ = removePIDFile(pidPath) }()
	}

	logDir := filepath.Dir(db)
	logFile, err := openSecureAppendLog(filepath.Join(logDir, "daemon.log"))
	if err != nil {
		log.Printf("warning: could not open log file: %v", err)
	} else {
		defer logFile.Close()
		log.SetOutput(logFile)
	}

	log.Printf("hcli collect listening on 127.0.0.1:%d (logs=%v)", r.Port(), withLogs)
	fmt.Fprintf(cmd.OutOrStdout(), "hcli collecting on 127.0.0.1:%d (logs=%v)\n", r.Port(), withLogs)
	fmt.Fprintf(cmd.OutOrStdout(), "db: %s\n", db)
	fmt.Fprintf(cmd.OutOrStdout(), "press Ctrl+C to stop\n")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Printf("hcli collect shutting down")
	fmt.Fprintf(cmd.OutOrStdout(), "\nshutting down...\n")
	return nil
}
