package tui

import (
	"github.com/charmbracelet/x/ansi"
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
	// extra = 100 - fixed(30) - spacing(2) = 68; 68/3 = 22 remainder 2,
	// remainder goes to the first two columns: 10+23, 10+23, 10+22.
	want := []int{33, 33, 32}
	for i := range want {
		if widths[i] != want[i] {
			t.Fatalf("widths: got %v, want %v", widths, want)
		}
	}
}

func TestPadOrTruncate_unicode(t *testing.T) {
	// Multibyte runes must not be split and output must occupy exactly
	// the requested display width.
	got := padOrTruncate("héllo wörld", 8)
	if w := displayWidth(got); w != 8 {
		t.Errorf("truncated width: got %d, want 8 (%q)", w, got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected ellipsis suffix, got %q", got)
	}

	// Wide (CJK) runes count as two cells.
	got = padOrTruncate("日本語テスト", 7)
	if w := displayWidth(got); w > 7 {
		t.Errorf("CJK truncation overflows: width %d > 7 (%q)", w, got)
	}

	// Padding is width-based too.
	got = padOrTruncate("héllo", 10)
	if w := displayWidth(got); w != 10 {
		t.Errorf("padded width: got %d, want 10 (%q)", w, got)
	}

	if got := padOrTruncate("anything", 0); got != "" {
		t.Errorf("zero width should return empty, got %q", got)
	}
}

func displayWidth(s string) int {
	return ansi.StringWidth(s)
}
