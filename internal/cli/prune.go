package cli

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/justinmaks/hedge-local/internal/store"
	"github.com/spf13/cobra"
)

var (
	pruneOlderThan string
	pruneDryRun    bool
	pruneYes       bool
	pruneVacuum    bool
)

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Delete telemetry older than a retention window",
	Long: `Deletes LLM calls, tool calls, and events older than the given age,
plus sessions that started before it and have no remaining spans.

The window comes from --older-than (e.g. 90d, 12w) or the retention_days
config key. Use --dry-run to preview and --vacuum to reclaim disk space.`,
	RunE: runPrune,
}

func init() {
	pruneCmd.Flags().StringVar(&pruneOlderThan, "older-than", "", "age cutoff, e.g. 90d or 12w (default: retention_days from config)")
	pruneCmd.Flags().BoolVar(&pruneDryRun, "dry-run", false, "show what would be deleted without doing it")
	pruneCmd.Flags().BoolVar(&pruneYes, "yes", false, "skip confirmation prompt")
	pruneCmd.Flags().BoolVar(&pruneVacuum, "vacuum", false, "run VACUUM afterwards to reclaim disk space")
	rootCmd.AddCommand(pruneCmd)
}

// parseRetention accepts Nd (days) and Nw (weeks).
func parseRetention(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if len(s) < 2 {
		return 0, fmt.Errorf("invalid retention %q (use e.g. 90d or 12w)", s)
	}
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid retention %q (use e.g. 90d or 12w)", s)
	}
	switch s[len(s)-1] {
	case 'd':
		return time.Duration(n) * 24 * time.Hour, nil
	case 'w':
		return time.Duration(n) * 7 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("invalid retention unit in %q (use d or w)", s)
	}
}

func runPrune(cmd *cobra.Command, args []string) error {
	cfg, db, err := loadCLIConfigAndDB()
	if err != nil {
		return err
	}

	spec := pruneOlderThan
	if spec == "" && cfg.RetentionDays > 0 {
		spec = fmt.Sprintf("%dd", cfg.RetentionDays)
	}
	if spec == "" {
		return fmt.Errorf("no retention window: pass --older-than (e.g. 90d) or set retention_days in config")
	}
	age, err := parseRetention(spec)
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-age)

	s, err := store.New(db)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	out := cmd.OutOrStdout()

	counts, err := s.PruneCounts(cutoff)
	if err != nil {
		return err
	}
	total := counts.LLMCalls + counts.ToolCalls + counts.Events + counts.Sessions
	fmt.Fprintf(out, "Older than %s (before %s):\n", spec, cutoff.Local().Format("2006-01-02 15:04"))
	fmt.Fprintf(out, "  llm calls:  %d\n", counts.LLMCalls)
	fmt.Fprintf(out, "  tool calls: %d\n", counts.ToolCalls)
	fmt.Fprintf(out, "  events:     %d\n", counts.Events)
	fmt.Fprintf(out, "  sessions:   %d\n", counts.Sessions)

	if total == 0 {
		fmt.Fprintln(out, "Nothing to prune.")
		return nil
	}
	if pruneDryRun {
		fmt.Fprintln(out, "Dry run — nothing deleted.")
		return nil
	}

	if !pruneYes {
		fmt.Fprintf(out, "Delete these %d rows permanently? [y/N]: ", total)
		reader := bufio.NewReader(cmd.InOrStdin())
		line, _ := reader.ReadString('\n')
		answer := strings.ToLower(strings.TrimSpace(line))
		if answer != "y" && answer != "yes" {
			fmt.Fprintln(out, "Aborted.")
			return nil
		}
	}

	result, err := s.PruneBefore(cutoff)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Pruned %d llm calls, %d tool calls, %d events, %d sessions.\n",
		result.LLMCalls, result.ToolCalls, result.Events, result.Sessions)

	if pruneVacuum {
		fmt.Fprintln(out, "Running VACUUM...")
		if err := s.Vacuum(); err != nil {
			return fmt.Errorf("vacuum: %w", err)
		}
		fmt.Fprintln(out, "Done.")
	}
	return nil
}
