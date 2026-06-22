package tui

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestWriteCSV(t *testing.T) {
	var buf bytes.Buffer
	headers := []string{"name", "cost"}
	rows := [][]string{{"claude_code", "$1.50"}, {"opencode", "$0.80"}}

	if err := WriteCSV(&buf, headers, rows); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "name,cost") {
		t.Errorf("CSV missing header: %s", output)
	}
	if !strings.Contains(output, "claude_code,$1.50") {
		t.Errorf("CSV missing row: %s", output)
	}
}

func TestWriteJSON(t *testing.T) {
	var buf bytes.Buffer
	rows := []map[string]any{{"name": "test", "cost": 1.5}}

	if err := WriteJSON(&buf, rows); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var result []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(result) != 1 || result[0]["name"] != "test" {
		t.Errorf("JSON content unexpected: %s", buf.String())
	}
}

func TestWriteMarkdown(t *testing.T) {
	var buf bytes.Buffer
	headers := []string{"Name", "Cost"}
	rows := [][]string{{"claude_code", "$1.50"}}

	if err := WriteMarkdown(&buf, headers, rows); err != nil {
		t.Fatalf("WriteMarkdown: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "| Name | Cost |") {
		t.Errorf("Markdown missing header: %s", output)
	}
	if !strings.Contains(output, "| claude_code | $1.50 |") {
		t.Errorf("Markdown missing row: %s", output)
	}
}

func TestWriteWeeklyReport(t *testing.T) {
	var buf bytes.Buffer
	stats := WeeklyReportStats{
		FromDate:    "2026-06-14",
		ToDate:      "2026-06-21",
		TotalCost:   2.30,
		TotalTokens: 1234,
		Sessions:    4,
		TopProjects: [][]string{{"hedge-local", "$2.30", "4"}},
		TopTools:    [][]string{{"read", "12", "100.0", "$0.00"}},
		ByAgent:     [][]string{{"claude_code", "$1.50", "800", "2"}},
	}

	if err := WriteWeeklyReport(&buf, stats); err != nil {
		t.Fatalf("WriteWeeklyReport: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "# hcli Weekly Report (2026-06-14 to 2026-06-21)") {
		t.Errorf("weekly report missing title: %s", output)
	}
	if !strings.Contains(output, "- Total cost: $2.30") {
		t.Errorf("weekly report missing summary: %s", output)
	}
	if !strings.Contains(output, "| hedge-local | $2.30 | 4 |") {
		t.Errorf("weekly report missing project row: %s", output)
	}
}
