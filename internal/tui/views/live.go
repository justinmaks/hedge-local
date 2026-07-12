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

var liveAgentFilters = []string{"all", "claude_code", "opencode"}
var liveTypeFilters = []string{"all", "llm", "tool", "event"}

const (
	facePulse = iota
	faceTable
)

// pulseBucketDur is one waveform cell; pulseBuckets is the full window kept
// so wide terminals still fill their width (300s at 2s per cell).
const pulseBucketDur = 2 * time.Second
const pulseBuckets = 150

type LiveView struct {
	service     *queries.Service
	spans       []queries.SpanRow
	table       *tui.Table
	face        int
	buckets     []queries.LiveBucket
	stats       queries.LiveStats
	statsAt     time.Time
	paused      bool
	follow      bool
	from        time.Time
	to          time.Time
	agentFilter int // 0=all, 1=claude_code, 2=opencode
	typeFilter  int // 0=all, 1=llm, 2=tool, 3=event
	err         error
}

func NewLiveView(service *queries.Service) *LiveView {
	tbl := tui.NewTable([]tui.Column{
		{Title: "Time", Width: 18},
		{Title: "Agent", Width: 12},
		{Title: "Type", Width: 6},
		{Title: "Detail", Width: 20},
		{Title: "Tokens", Width: 8},
		{Title: "Cost", Width: 8},
	})
	return &LiveView{service: service, table: tbl, follow: true, face: facePulse}
}

func (v *LiveView) Title() string { return "Live" }

func (v *LiveView) Init() tea.Cmd {
	return pollLive()
}

func (v *LiveView) Reload(ctx tui.ViewContext) tea.Cmd {
	v.from = ctx.From
	v.to = ctx.To
	if v.face == faceTable {
		return v.loadSpansCmd(ctx.From, ctx.To)
	}
	return v.loadPulseCmd()
}

func (v *LiveView) Hints() string {
	status := ""
	if v.paused {
		status += "[PAUSED] "
	}
	if v.face == facePulse {
		return status + "t table  p pause"
	}
	if !v.follow {
		status += "[MANUAL] "
	}
	return status + "t pulse  p pause  c clear  f follow  ←/→ agent  [/ ] type  ↑↓ scroll"
}

func (v *LiveView) Update(msg tea.Msg, ctx tui.ViewContext) (tui.View, tea.Cmd) {
	switch m := msg.(type) {
	case liveLoadedMsg:
		v.err = m.err
		if m.err == nil {
			v.spans = m.spans
			v.refreshTable()
		}
		return v, nil
	case pulseLoadedMsg:
		v.err = m.err
		if m.err == nil {
			v.buckets = m.buckets
			v.stats = m.stats
			v.statsAt = m.at
		}
		return v, nil
	case tui.LiveTickMsg:
		return v.UpdateLiveTick(m, ctx)

	case tea.KeyMsg:
		switch m.String() {
		case "t":
			if v.face == facePulse {
				v.face = faceTable
				return v, v.loadSpansCmd(v.from, v.to)
			}
			v.face = facePulse
			return v, v.loadPulseCmd()
		case "p":
			v.paused = !v.paused
		case "c":
			if v.face == faceTable {
				v.spans = nil
				v.err = nil
				v.setRows(nil)
			}
		case "f":
			v.follow = !v.follow
		case "left":
			if v.face == faceTable && v.agentFilter > 0 {
				v.agentFilter--
				return v, v.Reload(ctx)
			}
		case "right":
			if v.face == faceTable && v.agentFilter < len(liveAgentFilters)-1 {
				v.agentFilter++
				return v, v.Reload(ctx)
			}
		case "[":
			if v.face == faceTable && v.typeFilter > 0 {
				v.typeFilter--
				return v, v.Reload(ctx)
			}
		case "]":
			if v.face == faceTable && v.typeFilter < len(liveTypeFilters)-1 {
				v.typeFilter++
				return v, v.Reload(ctx)
			}
		case "up":
			v.table.ScrollUp()
		case "down":
			v.table.ScrollDown()
		}
	}

	return v, nil
}

func (v *LiveView) UpdateLiveTick(msg tui.LiveTickMsg, ctx tui.ViewContext) (tui.View, tea.Cmd) {
	_ = msg
	if v.paused {
		return v, pollLive()
	}
	if v.face == facePulse {
		return v, tea.Batch(v.loadPulseCmd(), pollLive())
	}
	return v, tea.Batch(v.Reload(ctx), pollLive())
}

type liveLoadedMsg struct {
	spans []queries.SpanRow
	err   error
}

type pulseLoadedMsg struct {
	buckets []queries.LiveBucket
	stats   queries.LiveStats
	at      time.Time
	err     error
}

func (v *LiveView) loadPulseCmd() tea.Cmd {
	service := v.service
	return func() tea.Msg {
		now := time.Now()
		buckets, err := service.LiveWindow(now, pulseBuckets, pulseBucketDur)
		if err != nil {
			return pulseLoadedMsg{err: err}
		}
		stats, err := service.LiveStats(now)
		return pulseLoadedMsg{buckets: buckets, stats: stats, at: now, err: err}
	}
}

func (v *LiveView) loadSpansCmd(from, to time.Time) tea.Cmd {
	filter := liveTypeFilters[v.typeFilter]
	agent := liveAgentFilters[v.agentFilter]
	return func() tea.Msg {
		spans, err := v.service.RecentSpansInRange(from, to, 50, filter, agent)
		return liveLoadedMsg{spans: spans, err: err}
	}
}

func (v *LiveView) refreshTable() {
	rows := make([][]string, 0, len(v.spans))
	for _, s := range v.spans {
		rows = append(rows, []string{
			s.Timestamp.Format("01-02 15:04:05"),
			s.Agent,
			s.SpanType,
			s.Detail,
			fmt.Sprintf("%d", s.Tokens),
			fmt.Sprintf("$%.4f", s.Cost),
		})
	}
	v.setRows(rows)
}

func (v *LiveView) setRows(rows [][]string) {
	cursor := 0
	if !v.follow {
		cursor = v.table.Cursor()
	}

	v.table.SetRows(rows)

	if !v.follow {
		maxCursor := len(rows) - 1
		if cursor > maxCursor {
			cursor = maxCursor
		}
		for i := 0; i < cursor; i++ {
			v.table.ScrollDown()
		}
	}
}

func (v *LiveView) Render(width, height int, theme *tui.Theme) string {
	if v.err != nil {
		return theme.ErrorMsg.Render("Error: " + v.err.Error())
	}
	if v.face == facePulse {
		return v.renderPulse(width, theme)
	}
	label := fmt.Sprintf("Agent: %s | Type: %s",
		liveAgentFilters[v.agentFilter], liveTypeFilters[v.typeFilter])
	return theme.Header.Render(label) + "\n" + v.table.Render(width, height-2, theme)
}

var (
	pulseWaveStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	pulseLaneStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	pulseErrStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	pulseLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

func (v *LiveView) renderPulse(width int, theme *tui.Theme) string {
	title := "PULSE"
	if v.paused {
		title += "  [PAUSED]"
	}
	header := theme.Header.Render(title) + pulseLabelStyle.Render(
		fmt.Sprintf("  output tokens · %ds per cell", int(pulseBucketDur.Seconds())))

	waveWidth := width - 4
	if waveWidth < 20 {
		waveWidth = 20
	}
	if waveWidth > pulseBuckets {
		waveWidth = pulseBuckets
	}
	visible := v.buckets
	if len(visible) > waveWidth {
		visible = visible[len(visible)-waveWidth:]
	}

	// Decay trail: each spike falls off over the following cells, so
	// bursty single-user traffic reads as an EKG trace instead of
	// isolated blips in empty space.
	values := make([]float64, len(visible))
	maxTokens := 0.0
	for i, b := range visible {
		values[i] = float64(b.OutputTokens)
		if i > 0 && values[i-1]*0.55 > values[i] {
			values[i] = values[i-1] * 0.55
		}
		if values[i] > maxTokens {
			maxTokens = values[i]
		}
	}
	top, bottom := tui.Waveform(values, maxTokens, waveWidth)

	// Idle cells get a dim baseline (a heartbeat at rest, not a gap);
	// live cells keep the bright accent so the signal owns the color.
	var bottomStyled strings.Builder
	for _, r := range bottom {
		if r == ' ' {
			bottomStyled.WriteString(pulseLaneStyle.Render("▁"))
		} else {
			bottomStyled.WriteString(pulseWaveStyle.Render(string(r)))
		}
	}

	var lane strings.Builder
	for i := 0; i < waveWidth; i++ {
		if i >= len(visible) {
			lane.WriteString(" ")
			continue
		}
		switch {
		case visible[i].ToolErrors > 0:
			lane.WriteString(pulseErrStyle.Render("✗"))
		case visible[i].ToolCalls > 0:
			lane.WriteString(pulseLaneStyle.Render("·"))
		default:
			lane.WriteString(" ")
		}
	}

	st := v.stats
	burnSpark := tui.Sparkline(st.BurnHistory, maxFloat(st.BurnHistory), 5)
	vsAvg := ""
	if st.Avg7dCost > 0 {
		pct := st.TodayCost / st.Avg7dCost * 100
		if pct > 100 {
			pct = 100
		}
		vsAvg = fmt.Sprintf("   today $%.2f %s vs 7d avg ($%.2f)",
			st.TodayCost, tui.Bar(14, pct), st.Avg7dCost)
	} else {
		vsAvg = fmt.Sprintf("   today $%.2f", st.TodayCost)
	}
	burnLine := fmt.Sprintf("burn %s/hr %s%s",
		theme.CardValue.Render(fmt.Sprintf("$%.2f", st.BurnPerHour)), burnSpark, vsAvg)

	now := v.statsAt
	if now.IsZero() {
		now = time.Now()
	}
	session := "session --:--"
	if !st.SessionStart.IsZero() {
		session = "session " + formatElapsed(now.Sub(st.SessionStart))
	}
	idle := "idle --"
	if !st.LastSpanAt.IsZero() {
		idle = "idle " + formatIdle(now.Sub(st.LastSpanAt))
	}
	rhythmLine := fmt.Sprintf("%s   %s   cache hit %.0f%%", session, idle, st.CachePct)

	nowLine := ""
	if st.LastModel != "" {
		nowLine = "now: " + theme.CardValue.Render(st.LastModel)
	}
	if st.LastTool != "" {
		status := "ok"
		if !st.LastToolOK {
			status = "failed"
		}
		if nowLine != "" {
			nowLine += "   "
		}
		nowLine += fmt.Sprintf("last tool: %s (%.1fs %s)",
			st.LastTool, float64(st.LastToolMs)/1000, status)
	}
	if nowLine == "" {
		nowLine = theme.Dim.Render("waiting for activity…")
	}

	lines := []string{
		header,
		"",
		"  " + pulseWaveStyle.Render(top),
		"  " + bottomStyled.String(),
		"  " + lane.String(),
		"",
		"  " + burnLine,
		"  " + rhythmLine,
		"  " + nowLine,
	}
	// Pad every line to the full width so stale cells never bleed
	// through in terminals with imperfect clearing.
	for i, l := range lines {
		if pad := width - lipgloss.Width(l); pad > 0 {
			lines[i] = l + strings.Repeat(" ", pad)
		}
	}
	return strings.Join(lines, "\n")
}

func formatElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

func formatIdle(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
}

func pollLive() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return tui.LiveTickMsg(t)
	})
}
