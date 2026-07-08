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
