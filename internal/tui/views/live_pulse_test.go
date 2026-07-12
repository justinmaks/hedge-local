package views

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/justinmaks/hedge-local/internal/tui"
	"github.com/justinmaks/hedge-local/internal/tui/queries"
)

func pulseTestView() *LiveView {
	v := NewLiveView(nil)
	buckets := make([]queries.LiveBucket, pulseBuckets)
	// A burst near the end of the window, with tool activity and one error.
	for i := pulseBuckets - 20; i < pulseBuckets; i++ {
		buckets[i].OutputTokens = (i % 10) * 120
	}
	buckets[pulseBuckets-3].ToolCalls = 2
	buckets[pulseBuckets-2].ToolCalls = 1
	buckets[pulseBuckets-2].ToolErrors = 1
	v.buckets = buckets
	v.stats = queries.LiveStats{
		BurnPerHour:  2.41,
		BurnHistory:  []float64{0.1, 0.2, 0.4, 0.3, 0.4},
		TodayCost:    12.70,
		Avg7dCost:    13.05,
		SessionStart: time.Now().Add(-42 * time.Minute),
		LastSpanAt:   time.Now().Add(-3 * time.Second),
		CachePct:     87,
		LastModel:    "claude-sonnet-5",
		LastTool:     "Edit",
		LastToolMs:   1200,
		LastToolOK:   true,
	}
	v.statsAt = time.Now()
	return v
}

func TestPulse_isDefaultFace(t *testing.T) {
	v := NewLiveView(nil)
	if v.face != facePulse {
		t.Fatal("pulse should be the Live tab's default face")
	}
	if !strings.Contains(v.Hints(), "t table") {
		t.Errorf("pulse hints should offer table toggle: %q", v.Hints())
	}
}

func TestPulse_renderContainsAllElements(t *testing.T) {
	v := pulseTestView()
	for _, width := range []int{80, 180} {
		out := v.Render(width, 30, tui.NewTheme())
		for _, want := range []string{
			"PULSE",
			"burn", "$2.41", "/hr",
			"today $12.70", "7d avg",
			"session 00:42:", "idle 3s", "cache hit 87%",
			"claude-sonnet-5", "Edit", "1.2s ok",
			"✗", // the tool error in the lane
		} {
			if !strings.Contains(out, want) {
				t.Errorf("width %d: pulse render missing %q:\n%s", width, want, out)
			}
		}
		// The waveform must show activity (block characters).
		if !strings.ContainsAny(out, "▁▂▃▄▅▆▇█") {
			t.Errorf("width %d: waveform has no blocks:\n%s", width, out)
		}
	}
}

func TestPulse_toggleToTableAndBack(t *testing.T) {
	v := pulseTestView()
	ctx := tui.ViewContext{}

	updated, _ := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}}, ctx)
	lv := updated.(*LiveView)
	if lv.face != faceTable {
		t.Fatal("t should switch to the table face")
	}
	if !strings.Contains(lv.Hints(), "t pulse") {
		t.Errorf("table hints should offer pulse toggle: %q", lv.Hints())
	}

	updated, _ = lv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}}, ctx)
	lv = updated.(*LiveView)
	if lv.face != facePulse {
		t.Fatal("t should switch back to pulse")
	}
}

func TestPulse_pauseStopsDataReload(t *testing.T) {
	v := pulseTestView()
	ctx := tui.ViewContext{}

	updated, _ := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}}, ctx)
	lv := updated.(*LiveView)
	if !lv.paused {
		t.Fatal("p should pause")
	}
	if !strings.Contains(lv.Hints(), "PAUSED") {
		t.Errorf("hints should show paused state: %q", lv.Hints())
	}
	// While paused the tick only reschedules itself: a single command, not
	// a batch with a data load.
	_, cmd := lv.UpdateLiveTick(tui.LiveTickMsg(time.Now()), ctx)
	if cmd == nil {
		t.Fatal("tick must reschedule while paused")
	}
	if _, isBatch := cmd().(tea.BatchMsg); isBatch {
		t.Error("paused tick should not batch a data reload")
	}
}

func TestPulse_emptyStateRendersQuietly(t *testing.T) {
	v := NewLiveView(nil)
	out := v.Render(100, 30, tui.NewTheme())
	if !strings.Contains(out, "waiting for activity") {
		t.Errorf("empty pulse should say it is waiting:\n%s", out)
	}
}
