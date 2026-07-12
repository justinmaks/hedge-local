package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/justinmaks/hedge-local/internal/tui/queries"
)

const defaultViewCount = 7

type ViewFactory func(*queries.Service) View

var defaultViewFactories = make([]ViewFactory, defaultViewCount)

type View interface {
	Title() string
	Init() tea.Cmd
	Update(msg tea.Msg, ctx ViewContext) (View, tea.Cmd)
	Render(width, height int, theme *Theme) string
	Hints() string
}

type ReloadableView interface {
	Reload(ctx ViewContext) tea.Cmd
}

type LiveTickMsg time.Time

type LiveTickView interface {
	UpdateLiveTick(msg LiveTickMsg, ctx ViewContext) (View, tea.Cmd)
}

type ViewContext struct {
	From   time.Time
	To     time.Time
	Width  int
	Height int
}

type App struct {
	theme      *Theme
	keymap     KeyMap
	service    *queries.Service
	views      []View
	activeView int
	dateFilter DateFilter
	width      int
	height     int
	showHelp   bool
	showDate   bool
	dateSaved  DateFilter
	collecting bool
	spanCount  int

	// collectorProbe rechecks whether a collector is answering; refreshed
	// on the live tick so the badge follows daemon/service starts and
	// stops while the TUI is open.
	collectorProbe func() bool
	lastProbe      time.Time
}

func NewApp(service *queries.Service, collecting bool) *App {
	theme := NewTheme()
	app := &App{
		theme:      theme,
		keymap:     DefaultKeyMap(),
		service:    service,
		dateFilter: NewDateFilter(),
		collecting: collecting,
	}
	app.initViews()
	return app
}

func RegisterViewFactory(idx int, factory ViewFactory) {
	if idx >= 0 && idx < len(defaultViewFactories) {
		defaultViewFactories[idx] = factory
	}
}

func (a *App) initViews() {
	a.views = make([]View, len(defaultViewFactories))
	for i, factory := range defaultViewFactories {
		if factory != nil {
			a.SetView(i, factory(a.service))
		}
	}
}

func (a *App) Init() tea.Cmd {
	ctx := a.viewContext()
	var cmds []tea.Cmd
	for _, v := range a.views {
		if v != nil {
			cmds = append(cmds, v.Init())
			if reloadable, ok := v.(ReloadableView); ok {
				cmds = append(cmds, reloadable.Reload(ctx))
			}
		}
	}
	return tea.Batch(cmds...)
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		return a, nil

	case tea.KeyMsg:
		if a.showDate {
			return a.handleDateKey(msg)
		}
		if a.showHelp {
			if msg.String() == "?" || msg.String() == "esc" || msg.String() == "q" {
				a.showHelp = false
			}
			return a, nil
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return a, tea.Quit
		case "?":
			a.showHelp = true
			return a, nil
		case "e":
			a.dateSaved = a.dateFilter
			a.showDate = true
			a.dateFilter.Active = true
			return a, nil
		case "r":
			if a.views[a.activeView] != nil {
				return a, a.reloadView(a.views[a.activeView], a.viewContext())
			}
		case "tab":
			a.activeView = (a.activeView + 1) % len(a.views)
			return a, a.reloadView(a.views[a.activeView], a.viewContext())
		case "shift+tab":
			a.activeView = (a.activeView - 1 + len(a.views)) % len(a.views)
			return a, a.reloadView(a.views[a.activeView], a.viewContext())
		case "1", "2", "3", "4", "5", "6", "7":
			idx := int(msg.String()[0] - '1')
			if idx < len(a.views) {
				a.activeView = idx
			}
			return a, a.reloadView(a.views[a.activeView], a.viewContext())
		}
	case LiveTickMsg:
		return a.handleLiveTick(msg)

	case spanCountMsg:
		a.spanCount = msg.count
		return a, nil

	case collectingMsg:
		a.collecting = bool(msg)
		return a, nil
	}

	if a.views[a.activeView] != nil {
		ctx := a.viewContext()
		updated, cmd := a.views[a.activeView].Update(msg, ctx)
		a.views[a.activeView] = updated
		return a, cmd
	}

	return a, nil
}

func (a *App) handleDateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.dateFilter = a.dateSaved
		a.closeDateFilter()
		return a, nil
	case "enter":
		if a.dateFilter.Range == RangeCustom && !a.dateFilter.HasValidCustomRange() {
			return a, nil
		}
		a.dateFilter.Update()
		a.closeDateFilter()
		return a, a.reloadAllViews(a.viewContext())
	case "1":
		a.dateFilter.Range = RangeToday
		a.dateFilter.Editing = 0
		return a, nil
	case "2":
		a.dateFilter.Range = Range7d
		a.dateFilter.Editing = 0
		return a, nil
	case "3":
		a.dateFilter.Range = Range30d
		a.dateFilter.Editing = 0
		return a, nil
	case "4":
		a.dateFilter.Range = RangeCustom
		a.dateFilter.Editing = 0
		return a, nil
	case "tab", "shift+tab":
		if a.dateFilter.Range == RangeCustom {
			a.dateFilter.ToggleField()
		}
		return a, nil
	case "backspace":
		if a.dateFilter.Range == RangeCustom {
			a.dateFilter.Backspace()
		}
		return a, nil
	}

	if a.dateFilter.Range == RangeCustom && msg.Type == tea.KeyRunes {
		for _, r := range msg.Runes {
			if (r >= '0' && r <= '9') || r == '-' {
				a.dateFilter.AppendRune(r)
				return a, nil
			}
		}
	}
	return a, nil
}

func (a *App) handleLiveTick(msg LiveTickMsg) (tea.Model, tea.Cmd) {
	ctx := a.viewContext()
	for _, idx := range a.liveTickViewOrder() {
		view := a.views[idx]
		if view == nil {
			continue
		}
		receiver, ok := view.(LiveTickView)
		if !ok {
			continue
		}
		updated, cmd := receiver.UpdateLiveTick(msg, ctx)
		a.views[idx] = updated
		return a, tea.Batch(cmd, a.spanCountCmd(), a.probeCmd())
	}
	return a, tea.Batch(a.spanCountCmd(), a.probeCmd())
}

type collectingMsg bool

// SetCollectorProbe installs the liveness check used to keep the status
// badge honest while the TUI runs.
func (a *App) SetCollectorProbe(probe func() bool) {
	a.collectorProbe = probe
}

// probeCmd rechecks collector liveness off the UI thread, at most every 2s.
func (a *App) probeCmd() tea.Cmd {
	if a.collectorProbe == nil {
		return nil
	}
	if time.Since(a.lastProbe) < 2*time.Second {
		return nil
	}
	a.lastProbe = time.Now()
	probe := a.collectorProbe
	return func() tea.Msg {
		return collectingMsg(probe())
	}
}

type spanCountMsg struct{ count int }

// spanCountCmd refreshes the status line's spans-today counter off the UI
// thread. Rides the existing live tick so it stays current in every view.
func (a *App) spanCountCmd() tea.Cmd {
	svc := a.service
	if svc == nil {
		return nil
	}
	return func() tea.Msg {
		now := time.Now()
		midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		count, err := svc.SpanCountInRange(midnight, now)
		if err != nil {
			return nil
		}
		return spanCountMsg{count: count}
	}
}

func (a *App) liveTickViewOrder() []int {
	order := []int{a.activeView}
	for idx := range a.views {
		if idx != a.activeView {
			order = append(order, idx)
		}
	}
	return order
}

func (a *App) closeDateFilter() {
	a.showDate = false
	a.dateFilter.Active = false
}

func (a *App) reloadAllViews(ctx ViewContext) tea.Cmd {
	var cmds []tea.Cmd
	for _, v := range a.views {
		cmds = append(cmds, a.reloadView(v, ctx))
	}
	return tea.Batch(cmds...)
}

func (a *App) reloadView(view View, ctx ViewContext) tea.Cmd {
	if view == nil {
		return nil
	}
	if reloadable, ok := view.(ReloadableView); ok {
		return reloadable.Reload(ctx)
	}
	return view.Init()
}

func (a *App) View() string {
	if a.width < 60 {
		return "Terminal too small (min 60 columns)"
	}

	contentHeight := a.height - TabBarHeight() - StatusLineHeight() - 2

	tabBar := RenderTabBar(a.activeView, a.theme)

	var content string
	if a.showDate {
		content = a.dateFilter.Render(a.theme)
	} else if a.showHelp {
		content = a.renderHelp()
	} else if a.views[a.activeView] != nil {
		content = a.views[a.activeView].Render(a.width, contentHeight, a.theme)
	} else {
		content = a.theme.Dim.Render(fmt.Sprintf("View %d not implemented yet", a.activeView+1))
	}

	statusInfo := StatusInfo{
		Collecting: a.collecting,
		SpanCount:  a.spanCount,
	}
	if a.views[a.activeView] != nil {
		statusInfo.Hints = a.views[a.activeView].Hints()
	}
	statusLine := RenderStatusLine(statusInfo, a.theme)

	header := a.theme.Header.Render(fmt.Sprintf("hcli — %s", a.dateFilter.Label()))

	return lipgloss.JoinVertical(lipgloss.Top,
		header,
		tabBar,
		content,
		statusLine,
	)
}

func (a *App) renderHelp() string {
	help := `hcli Keybindings

Global:
  1-7        Jump to tab
  Tab/Shift+Tab  Cycle tabs
  e          Date range filter
  r          Refresh
  ?          Help
  q/Ctrl+C   Quit

Table views:
  ↑↓         Scroll
  ←/→        Move cursor
  Enter      Focused detail
  Esc        Return to table

Cost view:
  ↑↓         Select day
  Enter      Drill into hourly
  Esc        Back to daily

Live view:
  p          Pause
  c          Clear
  f          Follow

Export view:
  p          Preview
  x          Execute export

CLI commands:
  hcli uninstall   Remove all hcli data
`
	return a.theme.Popup.Render(help)
}

func (a *App) viewContext() ViewContext {
	return ViewContext{
		From:   a.dateFilter.From,
		To:     a.dateFilter.To,
		Width:  a.width,
		Height: a.height - TabBarHeight() - StatusLineHeight() - 2,
	}
}

func (a *App) SetView(idx int, view View) {
	if idx >= 0 && idx < len(a.views) {
		a.views[idx] = view
	}
}

func RunApp(service *queries.Service, collecting bool, probe func() bool) error {
	app := NewApp(service, collecting)
	app.SetCollectorProbe(probe)
	p := tea.NewProgram(app, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
