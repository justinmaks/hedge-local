package tui

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func WriteCSV(w io.Writer, headers []string, rows [][]string) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(headers); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}
	for _, row := range rows {
		if err := cw.Write(row); err != nil {
			return fmt.Errorf("write csv row: %w", err)
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return fmt.Errorf("flush csv: %w", err)
	}
	return nil
}

func WriteJSON(w io.Writer, rows any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rows); err != nil {
		return fmt.Errorf("write json: %w", err)
	}
	return nil
}

func WriteMarkdown(w io.Writer, headers []string, rows [][]string) error {
	if _, err := fmt.Fprintf(w, "| %s |\n", strings.Join(headers, " | ")); err != nil {
		return fmt.Errorf("write markdown header: %w", err)
	}
	if _, err := fmt.Fprintf(w, "| %s |\n", strings.Join(repeatString("---", len(headers)), " | ")); err != nil {
		return fmt.Errorf("write markdown divider: %w", err)
	}
	for _, row := range rows {
		if _, err := fmt.Fprintf(w, "| %s |\n", strings.Join(row, " | ")); err != nil {
			return fmt.Errorf("write markdown row: %w", err)
		}
	}
	return nil
}

func repeatString(s string, n int) []string {
	result := make([]string, n)
	for i := range result {
		result[i] = s
	}
	return result
}

type WeeklyReportStats struct {
	FromDate    string
	ToDate      string
	TotalCost   float64
	TotalTokens int
	Sessions    int
	TopProjects [][]string
	TopTools    [][]string
	ByAgent     [][]string
}

func WriteWeeklyReport(w io.Writer, stats WeeklyReportStats) error {
	if _, err := fmt.Fprintf(w, "# hcli Weekly Report (%s to %s)\n\n", stats.FromDate, stats.ToDate); err != nil {
		return fmt.Errorf("write weekly title: %w", err)
	}
	if _, err := fmt.Fprintf(w, "## Summary\n- Total cost: $%.2f\n- Total tokens: %d\n- Sessions: %d\n\n", stats.TotalCost, stats.TotalTokens, stats.Sessions); err != nil {
		return fmt.Errorf("write weekly summary: %w", err)
	}
	if err := writeWeeklyTable(w, "Top Projects", []string{"Project", "Cost", "Sessions"}, stats.TopProjects); err != nil {
		return err
	}
	if err := writeWeeklyTable(w, "Top Tools", []string{"Tool", "Calls", "Success %", "Cost"}, stats.TopTools); err != nil {
		return err
	}
	if err := writeWeeklyTable(w, "By Agent", []string{"Agent", "Cost", "Tokens", "Sessions"}, stats.ByAgent); err != nil {
		return err
	}
	return nil
}

func writeWeeklyTable(w io.Writer, title string, headers []string, rows [][]string) error {
	if _, err := fmt.Fprintf(w, "## %s\n", title); err != nil {
		return fmt.Errorf("write weekly section %q: %w", title, err)
	}
	if err := WriteMarkdown(w, headers, rows); err != nil {
		return fmt.Errorf("write weekly table %q: %w", title, err)
	}
	if _, err := fmt.Fprint(w, "\n"); err != nil {
		return fmt.Errorf("write weekly spacing %q: %w", title, err)
	}
	return nil
}
