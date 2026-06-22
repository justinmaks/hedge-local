package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type Theme struct {
	ActiveTab   lipgloss.Style
	InactiveTab lipgloss.Style
	Header      lipgloss.Style
	StatusLine  lipgloss.Style
	Card        lipgloss.Style
	CardTitle   lipgloss.Style
	CardValue   lipgloss.Style
	TableHeader lipgloss.Style
	TableRow    lipgloss.Style
	TableRowAlt lipgloss.Style
	Selected    lipgloss.Style
	ErrorMsg    lipgloss.Style
	HelpText    lipgloss.Style
	Popup       lipgloss.Style
	Focused     lipgloss.Style
	Dim         lipgloss.Style
}

func NewTheme() *Theme {
	return &Theme{
		ActiveTab: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("63")).
			Padding(0, 1),
		InactiveTab: lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Padding(0, 1),
		Header: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("86")).
			Padding(0, 1),
		StatusLine: lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")),
		Card: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(0, 1),
		CardTitle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")),
		CardValue: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("86")),
		TableHeader: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("86")),
		TableRow:    lipgloss.NewStyle(),
		TableRowAlt: lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		Selected: lipgloss.NewStyle().
			Bold(true).
			Background(lipgloss.Color("236")),
		ErrorMsg: lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")),
		HelpText: lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")),
		Popup: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(1, 2),
		Focused: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("86")),
		Dim: lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")),
	}
}

var blockChars = []string{"▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}

func Sparkline(values []float64, maxVal float64) string {
	if maxVal <= 0 {
		maxVal = 1
	}
	var s string
	for _, v := range values {
		idx := int(v / maxVal * float64(len(blockChars)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(blockChars) {
			idx = len(blockChars) - 1
		}
		s += blockChars[idx]
	}
	return s
}

func Bar(width int, pct float64) string {
	if width < 0 {
		width = 0
	}
	full := int(float64(width) * pct / 100)
	if full < 0 {
		full = 0
	}
	if full > width {
		full = width
	}
	return strings.Repeat("█", full) + strings.Repeat("░", width-full)
}
