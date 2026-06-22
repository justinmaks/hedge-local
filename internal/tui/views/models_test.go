package views

import (
	"testing"

	"github.com/justinmaks/hedge-local/internal/tui/queries"
)

func TestModelsViewRefreshTableFiltersByAgent(t *testing.T) {
	v := NewModelsView(nil)
	v.stats = []queries.ModelStats{
		{Agent: "claude_code", Model: "claude-sonnet-4", Provider: "anthropic", Calls: 3, Cost: 1.25},
		{Agent: "opencode", Model: "gpt-4.1", Provider: "openai", Calls: 2, Cost: 0.75},
	}
	v.agentFilter = 1

	v.refreshTable()

	if len(v.table.Rows) != 1 {
		t.Fatalf("row count: got %d, want 1", len(v.table.Rows))
	}
	if v.table.Rows[0][0] != "claude-sonnet-4" {
		t.Fatalf("filtered model: got %q, want claude-sonnet-4", v.table.Rows[0][0])
	}
}
