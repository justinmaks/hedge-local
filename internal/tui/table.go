package tui

import (
	"sort"
	"strconv"
	"strings"
)

type Column struct {
	Title string
	Width int
}

type Table struct {
	Columns  []Column
	Rows     [][]string
	cursor   int
	scroll   int
	sortCol  int
	sortDesc bool
	selected int
}

func NewTable(cols []Column) *Table {
	return &Table{Columns: cols, sortCol: -1, selected: -1}
}

func (t *Table) SetRows(rows [][]string) {
	t.Rows = rows
	t.cursor = 0
	t.scroll = 0
}

func (t *Table) ScrollDown() {
	if t.cursor < len(t.Rows)-1 {
		t.cursor++
	}
}

func (t *Table) Cursor() int {
	return t.cursor
}

func (t *Table) SetCursor(cursor int) {
	if cursor < 0 {
		cursor = 0
	}
	if len(t.Rows) == 0 {
		t.cursor = 0
		t.scroll = 0
		return
	}
	if cursor >= len(t.Rows) {
		cursor = len(t.Rows) - 1
	}
	t.cursor = cursor
	if t.scroll > t.cursor {
		t.scroll = t.cursor
	}
}

func (t *Table) ScrollUp() {
	if t.cursor > 0 {
		t.cursor--
	}
}

func (t *Table) SortBy(col int) {
	if col < 0 || col >= len(t.Columns) {
		return
	}
	if t.sortCol == col {
		t.sortDesc = !t.sortDesc
	} else {
		t.sortCol = col
		t.sortDesc = false
	}
	sort.SliceStable(t.Rows, func(i, j int) bool {
		left, right := t.Rows[i][col], t.Rows[j][col]
		if leftNum, rightNum, ok := numericCellOrder(left, right); ok {
			if t.sortDesc {
				return leftNum > rightNum
			}
			return leftNum < rightNum
		}
		if t.sortDesc {
			return left > right
		}
		return left < right
	})
}

func numericCellOrder(left, right string) (float64, float64, bool) {
	leftNum, leftOK := parseNumericCell(left)
	rightNum, rightOK := parseNumericCell(right)
	if !leftOK || !rightOK {
		return 0, 0, false
	}
	return leftNum, rightNum, true
}

func parseNumericCell(value string) (float64, bool) {
	cleaned := strings.TrimSpace(value)
	replacements := []string{"$", "%", ",", "ms"}
	for _, replacement := range replacements {
		cleaned = strings.ReplaceAll(cleaned, replacement, "")
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(cleaned), 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func (t *Table) Render(width, height int, theme *Theme) string {
	_ = width
	if len(t.Rows) == 0 {
		return theme.Dim.Render("No data")
	}

	availHeight := height - 2 // header + border
	if availHeight < 1 {
		availHeight = 1
	}

	if t.cursor < t.scroll {
		t.scroll = t.cursor
	}
	if t.cursor >= t.scroll+availHeight {
		t.scroll = t.cursor - availHeight + 1
	}

	end := t.scroll + availHeight
	if end > len(t.Rows) {
		end = len(t.Rows)
	}

	var headers []string
	for i, col := range t.Columns {
		title := col.Title
		if i == t.sortCol {
			if t.sortDesc {
				title += " ↓"
			} else {
				title += " ↑"
			}
		}
		headers = append(headers, theme.TableHeader.Render(padOrTruncate(title, col.Width)))
	}
	headerLine := strings.Join(headers, " ")

	var lines []string
	lines = append(lines, headerLine)
	for idx := t.scroll; idx < end; idx++ {
		var cells []string
		for i, col := range t.Columns {
			val := ""
			if i < len(t.Rows[idx]) {
				val = t.Rows[idx][i]
			}
			style := theme.TableRow
			if idx%2 == 1 {
				style = theme.TableRowAlt
			}
			if idx == t.cursor {
				style = theme.Selected
			}
			cells = append(cells, style.Render(padOrTruncate(val, col.Width)))
		}
		lines = append(lines, strings.Join(cells, " "))
	}

	return strings.Join(lines, "\n")
}

func padOrTruncate(s string, width int) string {
	if len(s) > width {
		return s[:width-1] + "…"
	}
	return s + strings.Repeat(" ", width-len(s))
}
