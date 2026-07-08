package views

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/justinmaks/hedge-local/internal/tui"
	"github.com/justinmaks/hedge-local/internal/tui/queries"
)

func TestCostView_hintsChangeByMode(t *testing.T) {
	v := &CostView{mode: costModeDaily}
	hints := v.Hints()
	if !containsStr(hints, "Enter") {
		t.Errorf("daily hints should mention Enter: %s", hints)
	}

	v.mode = costModeHourly
	hints = v.Hints()
	if !containsStr(hints, "esc") {
		t.Errorf("hourly hints should mention Esc: %s", hints)
	}
}

func TestCostView_startsInDailyMode(t *testing.T) {
	v := NewCostView(nil)
	if v.mode != costModeDaily {
		t.Errorf("expected daily mode, got %d", v.mode)
	}
}

func TestCostView_dailyRender_showsBars(t *testing.T) {
	v := &CostView{
		service: nil,
		mode:    costModeDaily,
		trend: []queries.CostPoint{
			{Timestamp: time.Date(2026, 7, 6, 0, 0, 0, 0, time.Local), Cost: 1.50},
			{Timestamp: time.Date(2026, 7, 7, 0, 0, 0, 0, time.Local), Cost: 3.20},
			{Timestamp: time.Date(2026, 7, 8, 0, 0, 0, 0, time.Local), Cost: 0.75},
		},
		cursor: 1,
	}
	out := v.Render(120, 30, tui.NewTheme())
	if !contains(out, "07-06") {
		t.Errorf("missing date 07-06:\n%s", out)
	}
	if !contains(out, "07-07") {
		t.Errorf("missing date 07-07:\n%s", out)
	}
	if !contains(out, "$3.20") {
		t.Errorf("missing cost $3.20:\n%s", out)
	}
}

func TestCostView_enterDrillsIntoHourly(t *testing.T) {
	v := &CostView{
		service: nil,
		mode:    costModeDaily,
		trend: []queries.CostPoint{
			{Timestamp: time.Date(2026, 7, 7, 0, 0, 0, 0, time.Local), Cost: 3.20},
			{Timestamp: time.Date(2026, 7, 8, 0, 0, 0, 0, time.Local), Cost: 0.75},
		},
		cursor: 0,
	}
	ctx := tui.ViewContext{From: time.Now().Add(-7 * 24 * time.Hour), To: time.Now()}
	updated, _ := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'\r'}}, ctx)
	cv := updated.(*CostView)
	if cv.mode != costModeHourly {
		t.Errorf("expected hourly mode after enter, got %d", cv.mode)
	}
	if cv.drillDay != v.trend[0].Timestamp {
		t.Errorf("expected drillDay to match selected day")
	}
}

func TestCostView_escReturnsToDaily(t *testing.T) {
	v := &CostView{
		service: nil,
		mode:    costModeHourly,
		trend: []queries.CostPoint{
			{Timestamp: time.Date(2026, 7, 7, 0, 0, 0, 0, time.Local), Cost: 3.20},
		},
		cursor:   0,
		drillDay: time.Date(2026, 7, 7, 0, 0, 0, 0, time.Local),
	}
	ctx := tui.ViewContext{From: time.Now().Add(-7 * 24 * time.Hour), To: time.Now()}
	updated, _ := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'\x1b'}}, ctx)
	cv := updated.(*CostView)
	if cv.mode != costModeDaily {
		t.Errorf("expected daily mode after esc, got %d", cv.mode)
	}
}

func TestCostView_cursorMovesUpDown(t *testing.T) {
	v := &CostView{
		mode: costModeDaily,
		trend: []queries.CostPoint{
			{Timestamp: time.Date(2026, 7, 6, 0, 0, 0, 0, time.Local), Cost: 1.50},
			{Timestamp: time.Date(2026, 7, 7, 0, 0, 0, 0, time.Local), Cost: 3.20},
			{Timestamp: time.Date(2026, 7, 8, 0, 0, 0, 0, time.Local), Cost: 0.75},
		},
		cursor: 0,
	}
	ctx := tui.ViewContext{}

	updated, _ := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}, ctx)
	cv := updated.(*CostView)
	if cv.cursor != 1 {
		t.Errorf("expected cursor 1 after down, got %d", cv.cursor)
	}

	updated, _ = cv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}, ctx)
	cv = updated.(*CostView)
	if cv.cursor != 0 {
		t.Errorf("expected cursor 0 after up, got %d", cv.cursor)
	}
}

func TestCostView_cursorBounds(t *testing.T) {
	v := &CostView{
		mode: costModeDaily,
		trend: []queries.CostPoint{
			{Timestamp: time.Date(2026, 7, 6, 0, 0, 0, 0, time.Local), Cost: 1.50},
		},
		cursor: 0,
	}
	ctx := tui.ViewContext{}

	updated, _ := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}, ctx)
	cv := updated.(*CostView)
	if cv.cursor != 0 {
		t.Errorf("expected cursor 0 (clamped), got %d", cv.cursor)
	}
}

func TestCostView_hourlyLoadMsg_updatesHourlyData(t *testing.T) {
	v := &CostView{
		mode:     costModeHourly,
		drillDay: time.Date(2026, 7, 7, 0, 0, 0, 0, time.Local),
	}
	msg := costHourlyLoadedMsg{
		result: costHourlyResult{
			points: []queries.CostPoint{
				{Timestamp: time.Date(2026, 7, 7, 9, 0, 0, 0, time.Local), Cost: 1.20},
				{Timestamp: time.Date(2026, 7, 7, 10, 0, 0, 0, time.Local), Cost: 2.00},
			},
		},
	}
	ctx := tui.ViewContext{}
	updated, _ := v.Update(msg, ctx)
	cv := updated.(*CostView)
	if len(cv.hourly) != 2 {
		t.Errorf("expected 2 hourly points, got %d", len(cv.hourly))
	}
	if cv.hourly[1].Cost != 2.00 {
		t.Errorf("expected $2.00 at 10:00, got $%.2f", cv.hourly[1].Cost)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
