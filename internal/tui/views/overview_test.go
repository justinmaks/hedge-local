package views

import (
	"strings"
	"testing"

	"github.com/justinmaks/hedge-local/internal/tui"
	"github.com/justinmaks/hedge-local/internal/tui/queries"
)

// The Overview cost/token cards are scoped to the selected date range (the
// active range is shown in the header), so their labels must not hardcode
// "Today" — that misleads when the range is, e.g., 30 days.
func TestOverviewView_cardsAreNotLabeledToday(t *testing.T) {
	v := &OverviewView{stats: queries.OverviewStats{
		TodayCost:     2.65,
		TodayTokens:   683854,
		TodaySessions: 14,
		WeekCost:      2.65,
	}}
	out := v.Render(120, 30, tui.NewTheme())

	if strings.Contains(out, "Today Cost") || strings.Contains(out, "Today Tokens") {
		t.Errorf("overview cards still use hardcoded 'Today' labels:\n%s", out)
	}
	if !strings.Contains(out, "Cost") || !strings.Contains(out, "Tokens") {
		t.Errorf("overview cards missing Cost/Tokens labels:\n%s", out)
	}
}
