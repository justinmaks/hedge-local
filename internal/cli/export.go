package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/justinmaks/hedge-local/internal/store"
	"github.com/justinmaks/hedge-local/internal/tui"
	"github.com/justinmaks/hedge-local/internal/tui/queries"
	"github.com/spf13/cobra"
)

var (
	exportRange  string
	exportFormat string
	exportData   string
	exportOut    string
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export telemetry data headless (no TUI)",
	RunE:  runExport,
}

func init() {
	exportCmd.Flags().StringVar(&exportRange, "range", "7d", "Date range: today, 7d, 30d, custom:YYYY-MM-DD:YYYY-MM-DD")
	exportCmd.Flags().StringVar(&exportFormat, "format", "csv", "Format: csv, json, markdown")
	exportCmd.Flags().StringVar(&exportData, "data", "sessions", "Data: sessions, llm_calls, tool_calls, events, weekly_report")
	exportCmd.Flags().StringVar(&exportOut, "out", "", "Output file path or - for stdout")
	rootCmd.AddCommand(exportCmd)
}

func runExport(cmd *cobra.Command, args []string) error {
	from, to, err := parseRange(exportRange)
	if err != nil {
		return err
	}

	_, db, err := loadCLIConfigAndDB()
	if err != nil {
		return err
	}

	s, err := store.New(db)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	svc := queries.NewService(s)

	var buf bytes.Buffer
	if exportData == "weekly_report" {
		err = exportWeeklyReport(&buf, svc, from, to)
	} else {
		err = exportDataRows(&buf, svc, from, to)
	}
	if err != nil {
		return err
	}

	if exportOut == "" || exportOut == "-" {
		_, err := io.Copy(cmd.OutOrStdout(), &buf)
		return err
	}

	if err := os.WriteFile(exportOut, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write export file: %w", err)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Exported to %s\n", exportOut)
	return nil
}

func parseRange(r string) (time.Time, time.Time, error) {
	now := time.Now()
	to := now.Add(time.Hour)
	switch r {
	case "today":
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()), to, nil
	case "7d":
		return now.Add(-7 * 24 * time.Hour), to, nil
	case "30d":
		return now.Add(-30 * 24 * time.Hour), to, nil
	default:
		if !strings.HasPrefix(r, "custom:") {
			return time.Time{}, time.Time{}, fmt.Errorf("unknown range: %s", r)
		}
		parts := strings.Split(strings.TrimPrefix(r, "custom:"), ":")
		if len(parts) != 2 {
			return time.Time{}, time.Time{}, fmt.Errorf("custom range format: custom:YYYY-MM-DD:YYYY-MM-DD")
		}
		from, err := time.ParseInLocation("2006-01-02", parts[0], time.Local)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid from date: %w", err)
		}
		to, err := time.ParseInLocation("2006-01-02", parts[1], time.Local)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid to date: %w", err)
		}
		return from, to.Add(24 * time.Hour), nil
	}
}

func exportDataRows(w io.Writer, svc *queries.Service, from, to time.Time) error {
	var headers []string
	var rows [][]string

	switch exportData {
	case "sessions":
		headers = []string{"session_id", "agent", "project", "started_at", "ended_at", "cost_usd", "input_tokens", "output_tokens", "tool_calls", "message_count"}
		exportRows, err := svc.ExportSessions(from, to)
		if err != nil {
			return err
		}
		rows = make([][]string, 0, len(exportRows))
		for _, r := range exportRows {
			endedAt := ""
			if r.EndedAt != nil {
				endedAt = r.EndedAt.Format(time.RFC3339)
			}
			rows = append(rows, []string{
				r.ExternalID,
				r.Agent,
				r.ProjectPath,
				r.StartedAt.Format(time.RFC3339),
				endedAt,
				fmt.Sprintf("%.4f", r.TotalCostUSD),
				fmt.Sprintf("%d", r.InputTokens),
				fmt.Sprintf("%d", r.OutputTokens),
				fmt.Sprintf("%d", r.ToolCallCount),
				fmt.Sprintf("%d", r.MessageCount),
			})
		}
	case "llm_calls":
		headers = []string{"started_at", "agent", "model", "provider", "input_tokens", "output_tokens", "cache_read", "cache_write", "cost_usd", "duration_ms", "stop_reason"}
		exportRows, err := svc.ExportLLMCalls(from, to)
		if err != nil {
			return err
		}
		rows = make([][]string, 0, len(exportRows))
		for _, r := range exportRows {
			rows = append(rows, []string{
				r.StartedAt.Format(time.RFC3339),
				r.Agent,
				r.Model,
				r.Provider,
				fmt.Sprintf("%d", r.InputTokens),
				fmt.Sprintf("%d", r.OutputTokens),
				fmt.Sprintf("%d", r.CacheReadTokens),
				fmt.Sprintf("%d", r.CacheWriteTokens),
				fmt.Sprintf("%.4f", r.CostUSD),
				fmt.Sprintf("%d", r.DurationMs),
				r.StopReason,
			})
		}
	case "tool_calls":
		headers = []string{"started_at", "agent", "tool_name", "success", "duration_ms", "error_message"}
		exportRows, err := svc.ExportToolCalls(from, to)
		if err != nil {
			return err
		}
		rows = make([][]string, 0, len(exportRows))
		for _, r := range exportRows {
			rows = append(rows, []string{
				r.StartedAt.Format(time.RFC3339),
				r.Agent,
				r.ToolName,
				fmt.Sprintf("%t", r.Success),
				fmt.Sprintf("%d", r.DurationMs),
				r.ErrorMessage,
			})
		}
	case "events":
		headers = []string{"timestamp", "agent", "event_name", "payload"}
		exportRows, err := svc.ExportEvents(from, to)
		if err != nil {
			return err
		}
		rows = make([][]string, 0, len(exportRows))
		for _, r := range exportRows {
			rows = append(rows, []string{
				r.Timestamp.Format(time.RFC3339),
				r.Agent,
				r.EventName,
				r.Payload,
			})
		}
	default:
		return fmt.Errorf("unknown data type: %s", exportData)
	}

	switch exportFormat {
	case "csv":
		return tui.WriteCSV(w, headers, rows)
	case "json":
		return tui.WriteJSON(w, rowsAsObjects(headers, rows))
	case "markdown":
		return tui.WriteMarkdown(w, headers, rows)
	default:
		return fmt.Errorf("unknown format: %s", exportFormat)
	}
}

func exportWeeklyReport(w io.Writer, svc *queries.Service, from, to time.Time) error {
	overview, err := svc.OverviewSummary(from, to)
	if err != nil {
		return err
	}
	projects, err := svc.ProjectSummary(from, to)
	if err != nil {
		return err
	}
	tools, err := svc.ToolSummary(from, to)
	if err != nil {
		return err
	}
	agents, err := svc.CostByDimension(from, to, "agent")
	if err != nil {
		return err
	}

	stats := tui.WeeklyReportStats{
		FromDate:    from.Format("2006-01-02"),
		ToDate:      to.Format("2006-01-02"),
		TotalCost:   overview.TodayCost,
		TotalTokens: overview.TodayTokens,
		Sessions:    overview.TodaySessions,
	}

	for _, project := range projects[:minInt(len(projects), 5)] {
		name := project.Name
		if name == "" {
			name = project.Path
		}
		if name == "" {
			name = "(unknown)"
		}
		stats.TopProjects = append(stats.TopProjects, []string{
			name,
			fmt.Sprintf("$%.2f", project.Cost),
			fmt.Sprintf("%d", project.Sessions),
		})
	}

	for _, tool := range tools[:minInt(len(tools), 5)] {
		stats.TopTools = append(stats.TopTools, []string{
			tool.ToolName,
			fmt.Sprintf("%d", tool.Calls),
			fmt.Sprintf("%.1f", tool.SuccessRate),
			fmt.Sprintf("$%.2f", tool.TotalCost),
		})
	}

	for _, agent := range agents[:minInt(len(agents), 5)] {
		stats.ByAgent = append(stats.ByAgent, []string{
			agent.Name,
			fmt.Sprintf("$%.2f", agent.Cost),
			fmt.Sprintf("%d", agent.Tokens),
			fmt.Sprintf("%d", agent.Sessions),
		})
	}

	return tui.WriteWeeklyReport(w, stats)
}

func rowsAsObjects(headers []string, rows [][]string) []map[string]string {
	result := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		item := make(map[string]string, len(headers))
		for i, header := range headers {
			if i < len(row) {
				item[header] = row[i]
			}
		}
		result = append(result, item)
	}
	return result
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
