package views

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/justinmaks/hedge-local/internal/tui"
	"github.com/justinmaks/hedge-local/internal/tui/queries"
)

type costMode int

const (
	costModeDaily costMode = iota
	costModeHourly
)

type CostView struct {
	service   *queries.Service
	dimension int // 0=agent, 1=project, 2=model
	breakdown []queries.CostBreakdown
	trend     []queries.CostPoint
	hourly    []queries.CostPoint
	err       error
	mode      costMode
	cursor    int
	drillDay  time.Time
}

func NewCostView(service *queries.Service) *CostView {
	return &CostView{service: service, mode: costModeDaily}
}

func (v *CostView) Title() string { return "Cost" }

func (v *CostView) Init() tea.Cmd { return nil }

func (v *CostView) Reload(ctx tui.ViewContext) tea.Cmd {
	return func() tea.Msg {
		return costLoadedMsg{loadCost(v.service, v.dimension, ctx.From, ctx.To)}
	}
}

func (v *CostView) Hints() string {
	switch v.mode {
	case costModeHourly:
		return "esc back"
	default:
		return "←/→ dimension  ↑↓ select  Enter drill-down  r refresh  e date"
	}
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

type costHourlyLoadedMsg struct {
	result costHourlyResult
}

type costHourlyResult struct {
	points []queries.CostPoint
	err    error
}

func (v *CostView) loadHourly(ctx tui.ViewContext) tea.Cmd {
	if v.service == nil {
		return nil
	}
	day := v.drillDay
	from := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	to := from.Add(24 * time.Hour)
	return func() tea.Msg {
		points, err := v.service.CostTrend(from, to, "hourly")
		return costHourlyLoadedMsg{result: costHourlyResult{points: points, err: err}}
	}
}

func (v *CostView) Update(msg tea.Msg, ctx tui.ViewContext) (tui.View, tea.Cmd) {
	switch m := msg.(type) {
	case costLoadedMsg:
		v.breakdown = m.result.breakdown
		v.trend = m.result.trend
		v.err = m.result.err
		if v.cursor >= len(v.trend) {
			v.cursor = 0
		}
		return v, nil
	case costHourlyLoadedMsg:
		v.hourly = m.result.points
		v.err = m.result.err
		return v, nil
	case tea.KeyMsg:
		switch v.mode {
		case costModeDaily:
			return v.updateDaily(m, ctx)
		case costModeHourly:
			return v.updateHourly(m, ctx)
		}
	}
	return v, nil
}

func (v *CostView) updateDaily(m tea.KeyMsg, ctx tui.ViewContext) (tui.View, tea.Cmd) {
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
	case "up", "k":
		if v.cursor > 0 {
			v.cursor--
		}
	case "down", "j":
		if v.cursor < len(v.trend)-1 {
			v.cursor++
		}
	case "enter", "\r":
		if len(v.trend) > 0 && v.cursor < len(v.trend) {
			v.drillDay = v.trend[v.cursor].Timestamp
			v.mode = costModeHourly
			v.cursor = 0
			return v, v.loadHourly(ctx)
		}
	}
	return v, nil
}

func (v *CostView) updateHourly(m tea.KeyMsg, ctx tui.ViewContext) (tui.View, tea.Cmd) {
	switch m.String() {
	case "esc", "\x1b":
		v.mode = costModeDaily
		v.hourly = nil
		v.cursor = 0
		return v, nil
	}
	return v, nil
}

func (v *CostView) Render(width, height int, theme *tui.Theme) string {
	if v.err != nil {
		return theme.ErrorMsg.Render("Error: " + v.err.Error())
	}

	dimLabel := fmt.Sprintf("Dimension: %s (←/→ to switch)", dimNames[v.dimension])
	header := theme.Header.Render(dimLabel)

	switch v.mode {
	case costModeDaily:
		return header + "\n\n" + v.renderDailyBars(width, theme)
	case costModeHourly:
		return header + "\n\n" + v.renderHourlyBars(width, theme)
	}
	return header
}

func (v *CostView) renderDailyBars(width int, theme *tui.Theme) string {
	var lines []string
	lines = append(lines, theme.CardTitle.Render("Daily Cost (Enter to drill down)"))

	if len(v.trend) == 0 {
		lines = append(lines, theme.Dim.Render("  No data for this period"))
		return strings.Join(lines, "\n")
	}

	maxCost := 0.0
	for _, p := range v.trend {
		if p.Cost > maxCost {
			maxCost = p.Cost
		}
	}

	barWidth := width - 25
	if barWidth < 10 {
		barWidth = 10
	}
	if barWidth > 60 {
		barWidth = 60
	}
	for i, p := range v.trend {
		pct := 0.0
		if maxCost > 0 {
			pct = p.Cost / maxCost * 100
		}
		dateStr := p.Timestamp.Format("01-02")
		costStr := fmt.Sprintf("$%.2f", p.Cost)
		bar := tui.Bar(barWidth, pct)

		if i == v.cursor {
			lines = append(lines, theme.Selected.Render("▸ "+dateStr+"  "+bar+"  "+costStr))
		} else {
			lines = append(lines, "  "+dateStr+"  "+bar+"  "+costStr)
		}
	}

	lines = append(lines, "")
	lines = append(lines, theme.CardTitle.Render(
		fmt.Sprintf("%-15s %10s %6s %8s %10s", "Name", "Cost", "%", "Sessions", "Tokens")))
	for _, b := range v.breakdown {
		lines = append(lines, fmt.Sprintf("%-15s $%9.2f %5.1f%% %8d %10d",
			truncate(b.Name, 15), b.Cost, b.Pct, b.Sessions, b.Tokens))
	}

	return strings.Join(lines, "\n")
}

func (v *CostView) renderHourlyBars(width int, theme *tui.Theme) string {
	var lines []string
	title := fmt.Sprintf("Hourly Cost for %s (Esc to go back)", v.drillDay.Format("2006-01-02"))
	lines = append(lines, theme.CardTitle.Render(title))

	if len(v.hourly) == 0 {
		lines = append(lines, theme.Dim.Render("  No hourly data for this day"))
		return strings.Join(lines, "\n")
	}

	maxCost := 0.0
	for _, p := range v.hourly {
		if p.Cost > maxCost {
			maxCost = p.Cost
		}
	}

	barWidth := width - 25
	if barWidth < 10 {
		barWidth = 10
	}
	if barWidth > 60 {
		barWidth = 60
	}
	for _, p := range v.hourly {
		pct := 0.0
		if maxCost > 0 {
			pct = p.Cost / maxCost * 100
		}
		hourStr := p.Timestamp.Format("15:04")
		costStr := fmt.Sprintf("$%.2f", p.Cost)
		bar := tui.Bar(barWidth, pct)
		lines = append(lines, fmt.Sprintf("  %s  %s  %7s", hourStr, bar, costStr))
	}

	total := 0.0
	for _, p := range v.hourly {
		total += p.Cost
	}
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("  Day total: $%.2f (%d hours)", total, len(v.hourly)))

	return strings.Join(lines, "\n")
}
