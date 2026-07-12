package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

type StatusInfo struct {
	Collecting bool
	SpanCount  int
	Hints      string
}

func RenderStatusLine(info StatusInfo, theme *Theme) string {
	// Read-only DB mode gets a vivid blue badge; active collection gets
	// green. Both use black text for contrast on the bright backgrounds.
	label := "◆ DB LIVE"
	fg := lipgloss.Color("16")
	bg := lipgloss.Color("39")
	if info.Collecting {
		label = "● COLLECTING"
		fg = lipgloss.Color("16")
		bg = lipgloss.Color("46")
	}
	status := lipgloss.NewStyle().Bold(true).Foreground(fg).Background(bg).Padding(0, 1).Render(label)
	hints := info.Hints
	if hints == "" {
		hints = "↑↓ scroll  ⇧ tab  ? help  q quit"
	}
	keyLabel := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220")).Render("KEYS")
	keyHints := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("117")).Render(hints)
	nudge := ""
	if !info.Collecting {
		// Exporters do not buffer: no collector means telemetry is lost.
		nudge = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).
			Render(" · no collector (hcli service install)")
	}
	return fmt.Sprintf("%s %d spans today%s · %s %s", status, info.SpanCount, nudge, keyLabel, keyHints)
}

func StatusLineHeight() int {
	return 1
}
