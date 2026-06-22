package views

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/justinmaks/hedge-local/internal/tui"
	"github.com/justinmaks/hedge-local/internal/tui/queries"
)

var liveAgentFilters = []string{"all", "claude_code", "opencode"}
var liveTypeFilters = []string{"all", "llm", "tool", "event"}

type LiveView struct {
	service     *queries.Service
	spans       []queries.SpanRow
	table       *tui.Table
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
	return &LiveView{service: service, table: tbl, follow: true}
}

func (v *LiveView) Title() string { return "Live" }

func (v *LiveView) Init() tea.Cmd {
	return pollLive()
}

func (v *LiveView) Reload(ctx tui.ViewContext) tea.Cmd {
	v.from = ctx.From
	v.to = ctx.To
	return v.loadSpansCmd(ctx.From, ctx.To)
}

func (v *LiveView) Hints() string {
	status := ""
	if v.paused {
		status += "[PAUSED] "
	}
	if !v.follow {
		status += "[MANUAL] "
	}
	return status + "p pause  c clear  f follow  ←/→ agent  [/ ] type  ↑↓ scroll"
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
	case tui.LiveTickMsg:
		return v.UpdateLiveTick(m, ctx)

	case tea.KeyMsg:
		switch m.String() {
		case "p":
			v.paused = !v.paused
		case "c":
			v.spans = nil
			v.err = nil
			v.setRows(nil)
		case "f":
			v.follow = !v.follow
		case "left":
			if v.agentFilter > 0 {
				v.agentFilter--
				return v, v.Reload(ctx)
			}
		case "right":
			if v.agentFilter < len(liveAgentFilters)-1 {
				v.agentFilter++
				return v, v.Reload(ctx)
			}
		case "[":
			if v.typeFilter > 0 {
				v.typeFilter--
				return v, v.Reload(ctx)
			}
		case "]":
			if v.typeFilter < len(liveTypeFilters)-1 {
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
	if !v.paused {
		return v, tea.Batch(v.Reload(ctx), pollLive())
	}
	return v, pollLive()
}

type liveLoadedMsg struct {
	spans []queries.SpanRow
	err   error
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

	label := fmt.Sprintf("Agent: %s | Type: %s",
		liveAgentFilters[v.agentFilter], liveTypeFilters[v.typeFilter])
	return theme.Header.Render(label) + "\n" + v.table.Render(width, height-2, theme)
}

func pollLive() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return tui.LiveTickMsg(t)
	})
}
