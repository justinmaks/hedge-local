package views

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/justinmaks/hedge-local/internal/tui"
	"github.com/justinmaks/hedge-local/internal/tui/queries"
)

type ModelsView struct {
	service     *queries.Service
	stats       []queries.ModelStats
	table       *tui.Table
	agentFilter int // 0=all, 1=claude_code, 2=opencode
	err         error
}

func NewModelsView(service *queries.Service) *ModelsView {
	tbl := tui.NewTable([]tui.Column{
		{Title: "Model", Width: 20},
		{Title: "Calls", Width: 7},
		{Title: "Input", Width: 10},
		{Title: "Output", Width: 10},
		{Title: "Cache%", Width: 7},
		{Title: "Cost", Width: 10},
		{Title: "TTFT", Width: 8},
	})
	return &ModelsView{service: service, table: tbl}
}

func (v *ModelsView) Title() string { return "Models" }

func (v *ModelsView) Init() tea.Cmd { return nil }

func (v *ModelsView) Reload(ctx tui.ViewContext) tea.Cmd {
	return func() tea.Msg {
		stats, err := v.service.ModelSummary(ctx.From, ctx.To)
		return modelsLoadedMsg{stats, err}
	}
}

func (v *ModelsView) Hints() string {
	return "↑↓ scroll  ←/→ filter  1-7 sort"
}

type modelsLoadedMsg struct {
	stats []queries.ModelStats
	err   error
}

var modelFilters = []string{"all", "claude_code", "opencode"}

func (v *ModelsView) Update(msg tea.Msg, ctx tui.ViewContext) (tui.View, tea.Cmd) {
	switch m := msg.(type) {
	case modelsLoadedMsg:
		v.stats = m.stats
		v.err = m.err
		v.refreshTable()
		return v, nil
	case tea.KeyMsg:
		switch m.String() {
		case "up":
			v.table.ScrollUp()
		case "down":
			v.table.ScrollDown()
		case "left":
			if v.agentFilter > 0 {
				v.agentFilter--
				v.refreshTable()
			}
		case "right":
			if v.agentFilter < len(modelFilters)-1 {
				v.agentFilter++
				v.refreshTable()
			}
		case "1", "2", "3", "4", "5", "6", "7":
			idx := int(m.String()[0] - '1')
			v.table.SortBy(idx)
		}
	}
	return v, nil
}

func (v *ModelsView) refreshTable() {
	stats := v.filteredStats()
	var rows [][]string
	for _, ms := range stats {
		rows = append(rows, []string{
			ms.Model,
			fmt.Sprintf("%d", ms.Calls),
			fmt.Sprintf("%d", ms.InputTokens),
			fmt.Sprintf("%d", ms.OutputTokens),
			fmt.Sprintf("%.1f%%", ms.CachePct),
			fmt.Sprintf("$%.2f", ms.Cost),
			fmt.Sprintf("%.0fms", ms.AvgTTFTMs),
		})
	}
	v.table.SetRows(rows)
}

func (v *ModelsView) Render(width, height int, theme *tui.Theme) string {
	if v.err != nil {
		return theme.ErrorMsg.Render("Error: " + v.err.Error())
	}
	filterLabel := fmt.Sprintf("Filter: %s (←/→)", modelFilters[v.agentFilter])
	var bars []string
	bars = append(bars, theme.CardTitle.Render("Cost by Model"))
	stats := v.filteredStats()
	maxCost := 0.0
	for _, ms := range stats {
		if ms.Cost > maxCost {
			maxCost = ms.Cost
		}
	}
	modelBarWidth := width - 30
	if modelBarWidth < 10 {
		modelBarWidth = 10
	}
	if modelBarWidth > 60 {
		modelBarWidth = 60
	}
	for _, ms := range stats {
		pct := 0.0
		if maxCost > 0 {
			pct = ms.Cost / maxCost * 100
		}
		bars = append(bars, fmt.Sprintf("%-20s %s $%.2f", truncate(ms.Model, 20), tui.Bar(modelBarWidth, pct), ms.Cost))
	}
	barsSection := strings.Join(bars, "\n")

	return lipgloss.JoinVertical(lipgloss.Top,
		theme.Header.Render(filterLabel),
		"",
		v.table.Render(width, height/2, theme),
		"",
		barsSection,
	)
}

func (v *ModelsView) filteredStats() []queries.ModelStats {
	filter := modelFilters[v.agentFilter]
	if filter == "all" {
		return v.stats
	}
	filtered := make([]queries.ModelStats, 0, len(v.stats))
	for _, stat := range v.stats {
		if stat.Agent == filter {
			filtered = append(filtered, stat)
		}
	}
	return filtered
}
