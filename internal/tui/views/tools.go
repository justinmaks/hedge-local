package views

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/justinmaks/hedge-local/internal/store"
	"github.com/justinmaks/hedge-local/internal/tui"
	"github.com/justinmaks/hedge-local/internal/tui/queries"
)

type ToolsView struct {
	service     *queries.Service
	stats       []queries.ToolStats
	table       *tui.Table
	detailTable *tui.Table
	focused     bool
	selected    string
	detailRows  [][]string
	from        time.Time
	to          time.Time
	err         error
}

func NewToolsView(service *queries.Service) *ToolsView {
	tbl := tui.NewTable([]tui.Column{
		{Title: "Tool", Width: 15},
		{Title: "Calls", Width: 7},
		{Title: "Success%", Width: 9},
		{Title: "Avg ms", Width: 8},
		{Title: "Cost", Width: 10},
		{Title: "Top Error", Width: 20},
	})
	detailTbl := tui.NewTable([]tui.Column{
		{Title: "Time", Width: 18},
		{Title: "Agent", Width: 12},
		{Title: "Duration", Width: 10},
		{Title: "Success", Width: 8},
		{Title: "Error", Width: 25},
	})
	return &ToolsView{service: service, table: tbl, detailTable: detailTbl}
}

func (v *ToolsView) Title() string { return "Tools" }

func (v *ToolsView) Init() tea.Cmd { return nil }

func (v *ToolsView) Reload(ctx tui.ViewContext) tea.Cmd {
	v.from = ctx.From
	v.to = ctx.To
	return func() tea.Msg {
		stats, err := v.service.ToolSummary(ctx.From, ctx.To)
		return toolsLoadedMsg{stats, err}
	}
}

func (v *ToolsView) Hints() string {
	if v.focused {
		return "esc back  ↑↓ scroll"
	}
	return "↑↓ scroll  enter detail  1-6 sort"
}

type toolsLoadedMsg struct {
	stats []queries.ToolStats
	err   error
}

func (v *ToolsView) Update(msg tea.Msg, ctx tui.ViewContext) (tui.View, tea.Cmd) {
	switch m := msg.(type) {
	case toolsLoadedMsg:
		v.stats = m.stats
		v.err = m.err
		v.refreshTable()
		if v.focused && v.selected != "" {
			v.loadDetail(v.selected)
		}
		return v, nil
	case tea.KeyMsg:
		if v.focused {
			switch m.String() {
			case "esc":
				v.focused = false
			case "up":
				v.detailTable.ScrollUp()
			case "down":
				v.detailTable.ScrollDown()
			}
			return v, nil
		}
		switch m.String() {
		case "up":
			v.table.ScrollUp()
		case "down":
			v.table.ScrollDown()
		case "enter":
			if v.table.Cursor() < len(v.table.Rows) {
				v.focused = true
				v.selected = v.table.Rows[v.table.Cursor()][0]
				v.loadDetail(v.selected)
			}
		case "1", "2", "3", "4", "5", "6":
			idx := int(m.String()[0] - '1')
			v.table.SortBy(idx)
		}
	}
	return v, nil
}

func (v *ToolsView) refreshTable() {
	var rows [][]string
	for _, ts := range v.stats {
		rows = append(rows, []string{
			ts.ToolName,
			fmt.Sprintf("%d", ts.Calls),
			fmt.Sprintf("%.1f%%", ts.SuccessRate),
			fmt.Sprintf("%.0f", ts.AvgLatencyMs),
			fmt.Sprintf("$%.2f", ts.TotalCost),
			ts.TopError,
		})
	}
	v.table.SetRows(rows)
}

func (v *ToolsView) loadDetail(toolName string) {
	oldCursor := 0
	if v.detailTable != nil {
		oldCursor = v.detailTable.Cursor()
	}
	db := v.service.Store().DB()
	rows, err := db.Query(
		`SELECT started_at, agent, duration_ms, success, COALESCE(error_message, '')
		 FROM tool_calls WHERE tool_name = ? AND started_at BETWEEN ? AND ?
		 ORDER BY started_at DESC LIMIT 20`,
		toolName, store.FormatTime(v.from), store.FormatTime(v.to),
	)
	if err != nil {
		v.detailRows = [][]string{{"Error: " + err.Error()}}
		v.detailTable.SetRows(v.detailRows)
		return
	}
	defer rows.Close()
	v.detailRows = nil
	for rows.Next() {
		var startedAt time.Time
		var agent string
		var dur int
		var success bool
		var errMsg string
		if err := rows.Scan(&startedAt, &agent, &dur, &success, &errMsg); err != nil {
			continue
		}
		v.detailRows = append(v.detailRows, []string{
			startedAt.Local().Format("01-02 15:04:05"),
			agent,
			fmt.Sprintf("%dms", dur),
			fmt.Sprintf("%v", success),
			errMsg,
		})
	}
	v.detailTable.SetRows(v.detailRows)
	v.detailTable.SetCursor(oldCursor)
}

func (v *ToolsView) Render(width, height int, theme *tui.Theme) string {
	if v.err != nil {
		return theme.ErrorMsg.Render("Error: " + v.err.Error())
	}
	if v.focused {
		header := theme.Header.Render("Tool Detail (esc to return)")
		return header + "\n" + v.detailTable.Render(width, height-2, theme)
	}
	return v.table.Render(width, height, theme)
}
