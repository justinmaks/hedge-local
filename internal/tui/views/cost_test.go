package views

import (
	"strings"
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

func TestCostView_dailyRender_respectsHeight(t *testing.T) {
	var trend []queries.CostPoint
	for i := 0; i < 30; i++ {
		trend = append(trend, queries.CostPoint{
			Timestamp: time.Date(2026, 6, 1+i, 0, 0, 0, 0, time.Local),
			Cost:      float64(i + 1),
		})
	}
	v := &CostView{mode: costModeDaily, trend: trend, cursor: 29}
	out := v.Render(100, 15, tui.NewTheme())
	gotLines := len(strings.Split(out, "\n"))
	if gotLines > 15 {
		t.Errorf("render exceeds height: %d lines > 15", gotLines)
	}
	// The cursor row (last day, 06-30) must remain visible.
	if !strings.Contains(out, "06-30") {
		t.Errorf("cursor row should stay visible in window:\n%s", out)
	}
	if !strings.Contains(out, "earlier") {
		t.Errorf("clipped window should show an earlier marker:\n%s", out)
	}
}

func TestCostView_hourlyRender_respectsHeight(t *testing.T) {
	var hourly []queries.CostPoint
	for h := 0; h < 24; h++ {
		hourly = append(hourly, queries.CostPoint{
			Timestamp: time.Date(2026, 7, 7, h, 0, 0, 0, time.Local),
			Cost:      1,
		})
	}
	v := &CostView{mode: costModeHourly, hourly: hourly, drillDay: hourly[0].Timestamp}
	out := v.Render(100, 12, tui.NewTheme())
	gotLines := len(strings.Split(out, "\n"))
	if gotLines > 13 {
		t.Errorf("hourly render exceeds height: %d lines", gotLines)
	}
	if !strings.Contains(out, "more hours") {
		t.Errorf("clipped hourly should indicate more rows:\n%s", out)
	}
}

func TestWindowRange(t *testing.T) {
	cases := []struct {
		total, avail, cursor, wantStart, wantEnd int
	}{
		{5, 10, 0, 0, 5},     // fits entirely
		{30, 10, 0, 0, 10},   // cursor at top
		{30, 10, 29, 20, 30}, // cursor at bottom
		{30, 10, 15, 10, 20}, // centered
	}
	for _, c := range cases {
		s, e := windowRange(c.total, c.avail, c.cursor)
		if s != c.wantStart || e != c.wantEnd {
			t.Errorf("windowRange(%d,%d,%d): got [%d,%d), want [%d,%d)",
				c.total, c.avail, c.cursor, s, e, c.wantStart, c.wantEnd)
		}
		if c.cursor < c.total && (c.cursor < s || c.cursor >= e) && c.total > c.avail {
			t.Errorf("cursor %d outside window [%d,%d)", c.cursor, s, e)
		}
	}
}
