package cli

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/justinmaks/hedge-local/internal/collect"
	"github.com/justinmaks/hedge-local/internal/store"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon status, DB size, and span count",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	cfg, db, err := loadCLIConfigAndDB()
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	// The health probe is the truth about collection: it sees the daemon,
	// a service-managed collector, and embedded receivers alike.
	if collect.HealthCheck(cfg.OTLPPort, 300*time.Millisecond) {
		fmt.Fprintf(out, "collector: listening on 127.0.0.1:%d\n", cfg.OTLPPort)
	} else {
		fmt.Fprintf(out, "collector: not running (telemetry is being dropped)\n")
		fmt.Fprintf(out, "  start one: 'hcli service install' (always-on) or 'hcli collect -d'\n")
	}
	printStatus(out, defaultPIDPath(), db)
	return nil
}

func printStatus(w io.Writer, pidPath, dbPath string) {
	if pid, err := readPIDFile(pidPath); err != nil {
		fmt.Fprintf(w, "hcli daemon: not running\n")
		fmt.Fprintf(w, "  run 'hcli collect -d' to start\n")
	} else if !processAlive(pid) {
		fmt.Fprintf(w, "hcli daemon: stopped (stale PID file at %s)\n", pidPath)
		fmt.Fprintf(w, "  run 'hcli collect -d' to start\n")
		_ = removePIDFile(pidPath)
	} else {
		fmt.Fprintf(w, "hcli daemon: running (PID %d)\n", pid)
		fmt.Fprintf(w, "  log:      %s\n", defaultLogPath())
	}

	if dbStat, err := os.Stat(dbPath); err == nil {
		fmt.Fprintf(w, "  db:       %s (%s)\n", dbPath, formatBytes(dbStat.Size()))
	} else {
		fmt.Fprintf(w, "  db:       %s (not found)\n", dbPath)
		return
	}

	s, err := store.New(dbPath)
	if err != nil {
		fmt.Fprintf(w, "  spans:    (could not open db)\n")
		return
	}
	defer s.Close()

	now := time.Now()
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	var sessionCount int
	_ = s.DB().QueryRow(`SELECT COUNT(*) FROM sessions WHERE started_at >= ?`, store.FormatTime(from)).Scan(&sessionCount)
	fmt.Fprintf(w, "  sessions: %d today\n", sessionCount)

	var llmCount int
	_ = s.DB().QueryRow(`SELECT COUNT(*) FROM llm_calls WHERE started_at >= ?`, store.FormatTime(from)).Scan(&llmCount)
	fmt.Fprintf(w, "  llm calls: %d today\n", llmCount)
}

func formatBytes(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	case n < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	default:
		return fmt.Sprintf("%.1f GB", float64(n)/(1024*1024*1024))
	}
}
