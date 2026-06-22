package views

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/justinmaks/hedge-local/internal/tui"
	"github.com/justinmaks/hedge-local/internal/tui/queries"
)

type CostView struct {
	service   *queries.Service
	dimension int // 0=agent, 1=project, 2=model
	breakdown []queries.CostBreakdown
	trend     []queries.CostPoint
	err       error
}

func NewCostView(service *queries.Service) *CostView {
	return &CostView{service: service}
}

func (v *CostView) Title() string { return "Cost" }

func (v *CostView) Init() tea.Cmd { return nil }

func (v *CostView) Reload(ctx tui.ViewContext) tea.Cmd {
	return func() tea.Msg {
		return costLoadedMsg{loadCost(v.service, v.dimension, ctx.From, ctx.To)}
	}
}

func (v *CostView) Hints() string {
	return "←/→ dimension  r refresh  e date"
}

type costLoadedMsg struct {
	result costResult
}

type costResult struct {
	breakdown []queries.CostBreakdown
	trend     []queries.CostPoint
	err       error
}

var dimNames = []string{"agent", "project", "model"}

func loadCost(service *queries.Service, dim int, from, to time.Time) costResult {
	var r costResult
	dimStr := "agent"
	if dim >= 0 && dim < len(dimNames) {
		dimStr = dimNames[dim]
	}
	r.breakdown, r.err = service.CostByDimension(from, to, dimStr)
	if r.err != nil {
		return r
	}
	r.trend, r.err = service.CostTrend(from, to, "daily")
	return r
}

func (v *CostView) Update(msg tea.Msg, ctx tui.ViewContext) (tui.View, tea.Cmd) {
	switch m := msg.(type) {
	case costLoadedMsg:
		v.breakdown = m.result.breakdown
		v.trend = m.result.trend
		v.err = m.result.err
		return v, nil
	case tea.KeyMsg:
		switch m.String() {
		case "left":
			if v.dimension > 0 {
				v.dimension--
				return v, v.Reload(ctx)
			}
		case "right":
			if v.dimension < len(dimNames)-1 {
				v.dimension++
				return v, v.Reload(ctx)
			}
		}
	}
	return v, nil
}

func (v *CostView) Render(width, height int, theme *tui.Theme) string {
	if v.err != nil {
		return theme.ErrorMsg.Render("Error: " + v.err.Error())
	}

	dimLabel := fmt.Sprintf("Dimension: %s (←/→ to switch)", dimNames[v.dimension])
	header := theme.Header.Render(dimLabel)

	var lines []string
	lines = append(lines, theme.CardTitle.Render("Daily Cost"))
	maxCost := 0.0
	for _, p := range v.trend {
		if p.Cost > maxCost {
			maxCost = p.Cost
		}
	}
	for _, p := range v.trend {
		barWidth := 30
		pct := 0.0
		if maxCost > 0 {
			pct = p.Cost / maxCost * 100
		}
		lines = append(lines, fmt.Sprintf("%s %s $%.2f",
			p.Timestamp.Format("01-02"), tui.Bar(barWidth, pct), p.Cost))
	}

	lines = append(lines, "")
	lines = append(lines, theme.CardTitle.Render(
		fmt.Sprintf("%-15s %10s %6s %8s %10s", "Name", "Cost", "%", "Sessions", "Tokens")))
	for _, b := range v.breakdown {
		lines = append(lines, fmt.Sprintf("%-15s $%9.2f %5.1f%% %8d %10d",
			truncate(b.Name, 15), b.Cost, b.Pct, b.Sessions, b.Tokens))
	}

	return header + "\n\n" + strings.Join(lines, "\n")
}
