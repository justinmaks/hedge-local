package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbletea"
)

type DateRange int

const (
	RangeToday DateRange = iota
	Range7d
	Range30d
	RangeCustom
)

type DateFilter struct {
	Range      DateRange
	From       time.Time
	To         time.Time
	CustomFrom string
	CustomTo   string
	Editing    int // 0=from, 1=to
	Active     bool
}

func NewDateFilter() DateFilter {
	d := DateFilter{Range: RangeToday}
	d.Update()
	return d
}

func (d *DateFilter) Update() {
	now := time.Now()
	d.To = now.Add(time.Hour)
	switch d.Range {
	case RangeToday:
		d.From = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	case Range7d:
		d.From = now.Add(-7 * 24 * time.Hour)
	case Range30d:
		d.From = now.Add(-30 * 24 * time.Hour)
	case RangeCustom:
		if from, to, ok := d.customRange(); ok {
			d.From = from
			d.To = to
		}
	}
}

func (d *DateFilter) HasValidCustomRange() bool {
	_, _, ok := d.customRange()
	return ok
}

func (d *DateFilter) ToggleField() {
	if d.Editing == 0 {
		d.Editing = 1
		return
	}
	d.Editing = 0
}

func (d *DateFilter) AppendRune(r rune) {
	if d.Editing == 0 {
		d.CustomFrom += string(r)
		return
	}
	d.CustomTo += string(r)
}

func (d *DateFilter) Backspace() {
	if d.Editing == 0 {
		d.CustomFrom = trimLastRune(d.CustomFrom)
		return
	}
	d.CustomTo = trimLastRune(d.CustomTo)
}

func (d *DateFilter) Label() string {
	switch d.Range {
	case RangeToday:
		return "Today"
	case Range7d:
		return "Last 7 days"
	case Range30d:
		return "Last 30 days"
	case RangeCustom:
		return fmt.Sprintf("%s to %s", d.CustomFrom, d.CustomTo)
	}
	return ""
}

func (d *DateFilter) Toggle() tea.Cmd {
	d.Active = !d.Active
	return nil
}

func (d *DateFilter) Render(theme *Theme) string {
	presets := []string{"Today", "7d", "30d", "Custom"}
	var items []string
	for i, label := range presets {
		style := theme.InactiveTab
		if i == int(d.Range) {
			style = theme.ActiveTab
		}
		items = append(items, style.Render(label))
	}
	content := strings.Join(items, "  ")
	if d.Range == RangeCustom {
		fromPrefix := " "
		toPrefix := " "
		if d.Editing == 0 {
			fromPrefix = ">"
		} else {
			toPrefix = ">"
		}
		content += fmt.Sprintf("\n\n%s From: %s\n%s To:   %s", fromPrefix, d.CustomFrom, toPrefix, d.CustomTo)
	}
	return theme.Popup.Render(content)
}

func (d *DateFilter) customRange() (time.Time, time.Time, bool) {
	if d.CustomFrom == "" || d.CustomTo == "" {
		return time.Time{}, time.Time{}, false
	}
	from, err := time.ParseInLocation("2006-01-02", d.CustomFrom, time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	to, err := time.ParseInLocation("2006-01-02", d.CustomTo, time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	return from, to.Add(24 * time.Hour), true
}

func trimLastRune(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	return string(runes[:len(runes)-1])
}
