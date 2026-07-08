package views

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/justinmaks/hedge-local/internal/tui"
	"github.com/justinmaks/hedge-local/internal/tui/queries"
)

type OverviewView struct {
	service *queries.Service
	stats   queries.OverviewStats
	trend   []queries.CostPoint
	err     error
}

func NewOverviewView(service *queries.Service) *OverviewView {
	return &OverviewView{service: service}
}

func (v *OverviewView) Title() string { return "Today" }

func (v *OverviewView) Init() tea.Cmd { return nil }

func (v *OverviewView) Reload(ctx tui.ViewContext) tea.Cmd {
	return func() tea.Msg {
		return overviewLoadedMsg{loadOverview(v.service, ctx.From, ctx.To)}
	}
}

func (v *OverviewView) Hints() string {
	return "r refresh  e date  1-7 tabs"
}

type overviewLoadedMsg struct {
	result overviewResult
}

type overviewResult struct {
	stats queries.OverviewStats
	trend []queries.CostPoint
	err   error
}

func loadOverview(service *queries.Service, from, to time.Time) overviewResult {
	var result overviewResult

	result.stats, result.err = service.OverviewSummary(from, to)
	if result.err != nil {
		return result
	}
	result.trend, result.err = service.CostTrend(from, to, "daily")
	return result
}

func (v *OverviewView) Update(msg tea.Msg, ctx tui.ViewContext) (tui.View, tea.Cmd) {
	switch m := msg.(type) {
	case overviewLoadedMsg:
		v.stats = m.result.stats
		v.trend = m.result.trend
		v.err = m.result.err
		return v, nil
	}
	return v, nil
}

func (v *OverviewView) Render(width, height int, theme *tui.Theme) string {
	if v.err != nil {
		return theme.ErrorMsg.Render("Error: " + v.err.Error())
	}

	cardWidth := (width - 6) / 4
	deltaStr := fmt.Sprintf("(%.1f%% vs prev)", v.stats.CostDeltaPct)
	if v.stats.CostDeltaPct > 0 {
		deltaStr = "↑ " + deltaStr
	} else {
		deltaStr = "↓ " + deltaStr
	}

	cards := []string{
		renderCard("Cost", fmt.Sprintf("$%.2f", v.stats.TodayCost), cardWidth, theme),
		renderCard("Tokens", fmt.Sprintf("%d", v.stats.TodayTokens), cardWidth, theme),
		renderCard("Sessions", fmt.Sprintf("%d", v.stats.TodaySessions), cardWidth, theme),
		renderCard("7-day Cost", fmt.Sprintf("$%.2f %s", v.stats.WeekCost, deltaStr), cardWidth, theme),
	}
	cardRow := lipgloss.JoinHorizontal(lipgloss.Top, cards...)

	var sparkValues []float64
	for _, p := range v.trend {
		sparkValues = append(sparkValues, p.Cost)
	}
	sparkWidth := width - 4
	if sparkWidth < 20 {
		sparkWidth = 20
	}
	sparkline := tui.Sparkline(sparkValues, maxFloat(sparkValues), sparkWidth)
	sparkSection := theme.CardTitle.Render("7-day Cost Trend") + "\n" + sparkline

	var agentLines []string
	agentLines = append(agentLines, theme.CardTitle.Render("By Agent"))
	maxAgentCost := 0.0
	for _, ab := range v.stats.ByAgent {
		if ab.Cost > maxAgentCost {
			maxAgentCost = ab.Cost
		}
	}
	for _, ab := range v.stats.ByAgent {
		barWidth := 20
		pct := 0.0
		if maxAgentCost > 0 {
			pct = ab.Cost / maxAgentCost * 100
		}
		agentLines = append(agentLines, fmt.Sprintf("%-15s %s $%.2f", ab.Agent, tui.Bar(barWidth, pct), ab.Cost))
	}
	agentSection := strings.Join(agentLines, "\n")

	var sessionLines []string
	sessionLines = append(sessionLines, theme.CardTitle.Render("Recent Sessions"))
	for _, s := range v.stats.RecentSessions {
		sessionLines = append(sessionLines, fmt.Sprintf("%-20s %-12s $%-8.2f %s",
			truncate(s.ExternalID, 20), s.Agent, s.Cost, s.StartedAt.Format("01-02 15:04")))
	}
	sessionSection := strings.Join(sessionLines, "\n")

	return lipgloss.JoinVertical(lipgloss.Top,
		cardRow,
		"",
		sparkSection,
		"",
		agentSection,
		"",
		sessionSection,
	)
}

func renderCard(title, value string, width int, theme *tui.Theme) string {
	content := theme.CardTitle.Render(title) + "\n" + theme.CardValue.Render(value)
	return theme.Card.Width(width).Render(content)
}

func maxFloat(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	m := vals[0]
	for _, v := range vals[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
