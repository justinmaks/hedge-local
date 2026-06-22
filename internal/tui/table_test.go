package tui

import (
	"strings"
	"testing"
)

func TestTableRender(t *testing.T) {
	tbl := NewTable([]Column{
		{Title: "Name", Width: 12},
		{Title: "Cost", Width: 8},
	})
	tbl.SetRows([][]string{
		{"claude_code", "$1.50"},
		{"opencode", "$0.80"},
	})
	output := tbl.Render(30, 10, NewTheme())
	if !strings.Contains(output, "Name") || !strings.Contains(output, "Cost") {
		t.Errorf("render missing headers: %s", output)
	}
	if !strings.Contains(output, "claude_code") || !strings.Contains(output, "opencode") {
		t.Errorf("render missing rows: %s", output)
	}
}

func TestTableScrollDown(t *testing.T) {
	tbl := NewTable([]Column{
		{Title: "Name", Width: 10},
	})
	var rows [][]string
	for i := 0; i < 20; i++ {
		rows = append(rows, []string{string(rune('a' + i))})
	}
	tbl.SetRows(rows)
	tbl.ScrollDown()
	if tbl.cursor != 1 {
		t.Errorf("cursor: got %d, want 1", tbl.cursor)
	}
}

func TestTableSortByColumn(t *testing.T) {
	tbl := NewTable([]Column{
		{Title: "Name", Width: 10},
		{Title: "Cost", Width: 8},
	})
	tbl.SetRows([][]string{
		{"b", "$2.00"},
		{"a", "$1.00"},
	})
	tbl.SortBy(1) // sort by Cost column
	if tbl.Rows[0][0] != "a" {
		t.Errorf("sort: first row should be 'a', got %q", tbl.Rows[0][0])
	}
}

func TestTableSortByNumericValues(t *testing.T) {
	tbl := NewTable([]Column{
		{Title: "Name", Width: 10},
		{Title: "Cost", Width: 8},
	})
	tbl.SetRows([][]string{
		{"ten", "$10.00"},
		{"two", "$2.00"},
		{"seven", "$7.50"},
	})

	tbl.SortBy(1)

	if tbl.Rows[0][0] != "two" || tbl.Rows[1][0] != "seven" || tbl.Rows[2][0] != "ten" {
		t.Fatalf("numeric sort order: got %v", tbl.Rows)
	}
}
