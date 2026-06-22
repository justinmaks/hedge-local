package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type stubReloadView struct {
	inits   int
	reloads []ViewContext
}

type stubLiveTickView struct {
	ticks int
	ctxs  []ViewContext
}

func (v *stubReloadView) Title() string { return "stub" }

func (v *stubReloadView) Init() tea.Cmd {
	v.inits++
	return nil
}

func (v *stubReloadView) Reload(ctx ViewContext) tea.Cmd {
	v.reloads = append(v.reloads, ctx)
	return nil
}

func (v *stubReloadView) Update(msg tea.Msg, ctx ViewContext) (View, tea.Cmd) {
	return v, nil
}

func (v *stubReloadView) Render(width, height int, theme *Theme) string {
	return ""
}

func (v *stubReloadView) Hints() string { return "" }

func (v *stubLiveTickView) Title() string { return "live" }

func (v *stubLiveTickView) Init() tea.Cmd { return nil }

func (v *stubLiveTickView) Update(msg tea.Msg, ctx ViewContext) (View, tea.Cmd) {
	return v, nil
}

func (v *stubLiveTickView) Render(width, height int, theme *Theme) string {
	return ""
}

func (v *stubLiveTickView) Hints() string { return "" }

func (v *stubLiveTickView) UpdateLiveTick(msg LiveTickMsg, ctx ViewContext) (View, tea.Cmd) {
	v.ticks++
	v.ctxs = append(v.ctxs, ctx)
	return v, func() tea.Msg { return nil }
}

func TestAppInitReloadsReloadableViews(t *testing.T) {
	view := &stubReloadView{}
	app := &App{
		views:      []View{view},
		dateFilter: NewDateFilter(),
		width:      120,
		height:     40,
	}

	app.Init()

	if view.inits != 1 {
		t.Fatalf("Init count: got %d, want 1", view.inits)
	}
	if len(view.reloads) != 1 {
		t.Fatalf("reload count: got %d, want 1", len(view.reloads))
	}
	if view.reloads[0].From.IsZero() || view.reloads[0].To.IsZero() {
		t.Fatalf("reload context should include date range, got %+v", view.reloads[0])
	}
	if view.reloads[0].Width != 120 {
		t.Fatalf("reload width: got %d, want 120", view.reloads[0].Width)
	}
}

func TestAppDateFilterCustomEntryAndApply(t *testing.T) {
	view := &stubReloadView{}
	app := &App{
		views:      []View{view},
		dateFilter: NewDateFilter(),
		showDate:   true,
		width:      100,
		height:     30,
	}
	app.dateFilter.Active = true
	app.dateFilter.Range = RangeCustom
	app.dateFilter.CustomFrom = "2026-01-0"
	app.dateFilter.CustomTo = "2026-01-1"

	for _, key := range []tea.KeyMsg{
		keyRunes("9"),
		{Type: tea.KeyTab},
		keyRunes("5"),
		{Type: tea.KeyEnter},
	} {
		if _, cmd := app.Update(key); cmd != nil {
			_ = cmd()
		}
	}

	if app.dateFilter.Range != RangeCustom {
		t.Fatalf("range: got %v, want %v", app.dateFilter.Range, RangeCustom)
	}
	if app.dateFilter.CustomFrom != "2026-01-09" {
		t.Fatalf("CustomFrom: got %q, want 2026-01-09", app.dateFilter.CustomFrom)
	}
	if app.dateFilter.CustomTo != "2026-01-15" {
		t.Fatalf("CustomTo: got %q, want 2026-01-15", app.dateFilter.CustomTo)
	}
	if app.dateFilter.Editing != 1 {
		t.Fatalf("Editing: got %d, want 1 after tab", app.dateFilter.Editing)
	}
	if app.showDate {
		t.Fatalf("date popup should close after apply")
	}
	from := time.Date(2026, time.January, 9, 0, 0, 0, 0, time.Local)
	to := time.Date(2026, time.January, 16, 0, 0, 0, 0, time.Local)
	if !app.dateFilter.From.Equal(from) {
		t.Fatalf("From: got %v, want %v", app.dateFilter.From, from)
	}
	if !app.dateFilter.To.Equal(to) {
		t.Fatalf("To: got %v, want %v", app.dateFilter.To, to)
	}
	if len(view.reloads) != 1 {
		t.Fatalf("reload count after apply: got %d, want 1", len(view.reloads))
	}
	if !view.reloads[0].From.Equal(from) || !view.reloads[0].To.Equal(to) {
		t.Fatalf("reload context: got %+v, want from=%v to=%v", view.reloads[0], from, to)
	}
	if app.dateFilter.Active {
		t.Fatalf("date filter should be inactive after apply")
	}
}

func TestAppDateFilterBackspaceRemovesCharacters(t *testing.T) {
	app := &App{dateFilter: NewDateFilter(), showDate: true}
	app.dateFilter.Active = true
	app.dateFilter.Range = RangeCustom
	app.dateFilter.CustomFrom = "25"

	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyBackspace},
	} {
		app.Update(key)
	}

	if app.dateFilter.CustomFrom != "2" {
		t.Fatalf("CustomFrom after backspace: got %q, want %q", app.dateFilter.CustomFrom, "2")
	}
}

func TestAppLiveTickContinuesWhenLiveViewIsInactive(t *testing.T) {
	overview := &stubReloadView{}
	live := &stubLiveTickView{}
	app := &App{
		views:      []View{overview, live},
		activeView: 0,
		dateFilter: NewDateFilter(),
		width:      100,
		height:     30,
	}

	_, cmd := app.Update(LiveTickMsg(time.Now()))
	if cmd == nil {
		t.Fatal("live tick should reschedule polling even when Live is inactive")
	}
	if live.ticks != 1 {
		t.Fatalf("inactive live ticks: got %d, want 1", live.ticks)
	}
}

func TestAppReloadsViewWhenSwitchingTabs(t *testing.T) {
	first := &stubReloadView{}
	second := &stubReloadView{}
	app := &App{
		views:      []View{first, second},
		activeView: 0,
		dateFilter: NewDateFilter(),
		width:      100,
		height:     30,
	}

	app.Update(keyRunes("2"))
	if app.activeView != 1 {
		t.Fatalf("activeView: got %d, want 1", app.activeView)
	}
	if len(second.reloads) != 1 {
		t.Fatalf("second reloads: got %d, want 1", len(second.reloads))
	}
	if len(first.reloads) != 0 {
		t.Fatalf("first reloads: got %d, want 0", len(first.reloads))
	}
}

func TestAppDateFilterPresetKeysWorkInCustomMode(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want DateRange
	}{
		{name: "today", key: "1", want: RangeToday},
		{name: "last 7 days", key: "2", want: Range7d},
		{name: "last 30 days", key: "3", want: Range30d},
		{name: "custom", key: "4", want: RangeCustom},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &App{dateFilter: NewDateFilter(), showDate: true}
			app.dateFilter.Active = true
			app.dateFilter.Range = RangeCustom
			app.dateFilter.CustomFrom = "2026-01-09"
			app.dateFilter.CustomTo = "2026-01-15"

			app.Update(keyRunes(tt.key))

			if app.dateFilter.Range != tt.want {
				t.Fatalf("range: got %v, want %v", app.dateFilter.Range, tt.want)
			}
		})
	}
}

func keyRunes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}
