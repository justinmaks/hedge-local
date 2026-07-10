package tui

import (
	"strings"
	"testing"
)

func TestTable_renderUsesFullWidth(t *testing.T) {
	cols := []Column{
		{Title: "Name", Width: 10},
		{Title: "Value", Width: 8},
	}
	tbl := NewTable(cols)
	tbl.SetRows([][]string{{"hello", "42"}})

	out := tbl.Render(120, 10, NewTheme())
	firstLine := strings.Split(out, "\n")[0]
	// The rendered line should be wider than 10+1+8=19
	if len(firstLine) < 40 {
		t.Errorf("table not expanding to width, got line length %d:\n%s", len(firstLine), out)
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

func TestDistributeWidths_evenDistribution(t *testing.T) {
	tbl := &Table{
		Columns: []Column{
			{Title: "A", Width: 10},
			{Title: "B", Width: 10},
			{Title: "C", Width: 10},
		},
	}
	widths := tbl.distributeWidths(100)
	// 100 total - 2 spaces spacing = 98 for columns
	// 98 / 3 = 32 remainder 2 → [33, 33, 32]
	if widths[0]+widths[1]+widths[2]+2 != 100 {
		t.Errorf("widths should sum to 100 (with spacing): %v", widths)
	}
}
