package tui

import (
	"strings"
	"testing"
)

func TestRenderStatusLineEmphasizesCollectingAndHints(t *testing.T) {
	line := RenderStatusLine(StatusInfo{Collecting: true, SpanCount: 13, Hints: "p preview  x export"}, NewTheme())

	plain := stripANSI(line)
	if !strings.Contains(plain, "COLLECTING") {
		t.Fatalf("status line should emphasize collecting state: %q", plain)
	}
	if !strings.Contains(plain, "13 spans today") {
		t.Fatalf("status line missing span count: %q", plain)
	}
	if !strings.Contains(plain, "KEYS") || !strings.Contains(plain, "p preview") || !strings.Contains(plain, "x export") {
		t.Fatalf("status line should label and include key hints: %q", plain)
	}
}

func TestRenderStatusLineReadOnlyModeShowsDatabaseLive(t *testing.T) {
	line := RenderStatusLine(StatusInfo{Collecting: false, SpanCount: 13, Hints: "q quit"}, NewTheme())

	plain := stripANSI(line)
	if !strings.Contains(plain, "DB LIVE") {
		t.Fatalf("read-only TUI should show DB LIVE, got %q", plain)
	}
	if strings.Contains(plain, "IDLE") {
		t.Fatalf("read-only TUI should not imply idle collection, got %q", plain)
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	inEscape := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inEscape {
			if c >= '@' && c <= '~' {
				inEscape = false
			}
			continue
		}
		if c == 0x1b {
			inEscape = true
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

func TestRenderStatusLineNudgesWhenNotCollecting(t *testing.T) {
	plain := stripANSI(RenderStatusLine(StatusInfo{Collecting: false, SpanCount: 0}, NewTheme()))
	if !strings.Contains(plain, "no collector") {
		t.Fatalf("read-only mode should nudge about missing collector: %q", plain)
	}
	plain = stripANSI(RenderStatusLine(StatusInfo{Collecting: true, SpanCount: 0}, NewTheme()))
	if strings.Contains(plain, "no collector") {
		t.Fatalf("collecting mode should not nudge: %q", plain)
	}
}
