package views

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/justinmaks/hedge-local/internal/store"
	"github.com/justinmaks/hedge-local/internal/tui"
)

func TestExportViewSessionHeaders(t *testing.T) {
	v := NewExportView(nil)
	headers := v.getHeaders()
	want := []string{"session_id", "agent", "project", "started_at", "ended_at", "cost_usd", "input_tokens", "output_tokens", "tool_calls", "message_count"}

	if len(headers) != len(want) {
		t.Fatalf("header count = %d, want %d", len(headers), len(want))
	}
	for i, header := range want {
		if headers[i] != header {
			t.Fatalf("header %d = %q, want %q", i, headers[i], header)
		}
	}
}

func TestExportViewSelectorNavigation(t *testing.T) {
	v := NewExportView(nil)

	v.moveSelector(1)
	if v.activeSelector != 1 {
		t.Fatalf("active selector = %d, want 1", v.activeSelector)
	}

	v.cycleValue(1)
	if v.format != 1 {
		t.Fatalf("format = %d, want 1", v.format)
	}

	v.moveSelector(1)
	v.cycleValue(1)
	if v.destination != 1 {
		t.Fatalf("destination = %d, want 1", v.destination)
	}

	v.moveSelector(-1)
	v.moveSelector(-1)
	v.cycleValue(-1)
	if v.dataType != len(dataTypes)-1 {
		t.Fatalf("data type = %d, want %d", v.dataType, len(dataTypes)-1)
	}
}

func TestExportViewPreviewWeeklyMessage(t *testing.T) {
	v := NewExportView(nil)
	v.dataType = 4
	v.doPreview(time.Time{}, time.Time{})

	if !strings.Contains(v.preview, "Weekly report preview") {
		t.Fatalf("preview = %q", v.preview)
	}
}

func TestExportViewReloadAutoBuildsPreview(t *testing.T) {
	service, db := testQueryService(t)
	projectID, err := db.ProjectUpsert("/tmp/export-project", "export-project")
	if err != nil {
		t.Fatalf("ProjectUpsert: %v", err)
	}
	now := time.Now()
	insertSession(t, db, projectID, "sess-export-preview", now)

	view := NewExportView(service)
	ctx := tui.ViewContext{From: now.Add(-time.Hour), To: now.Add(time.Hour)}
	cmd := view.Reload(ctx)
	if cmd == nil {
		t.Fatal("ExportView.Reload returned nil command")
	}
	updated, _ := view.Update(cmd(), ctx)
	loaded := updated.(*ExportView)

	if !strings.Contains(loaded.preview, "session_id,agent,project") {
		t.Fatalf("preview missing sessions header: %q", loaded.preview)
	}
	if !strings.Contains(loaded.preview, "sess-export-preview") {
		t.Fatalf("preview missing session row: %q", loaded.preview)
	}
}

func TestExportViewChangingDataOrFormatRefreshesPreview(t *testing.T) {
	service, db := testQueryService(t)
	projectID, err := db.ProjectUpsert("/tmp/export-llm-project", "export-llm-project")
	if err != nil {
		t.Fatalf("ProjectUpsert: %v", err)
	}
	now := time.Now()
	sessionID := insertSession(t, db, projectID, "sess-export-llm", now)
	if _, err := db.LLMCallInsert(store.LLMCallParams{
		SessionID:    sessionID,
		StartedAt:    now,
		Agent:        "claude_code",
		Model:        "claude-sonnet-4-5",
		Provider:     "anthropic",
		InputTokens:  10,
		OutputTokens: 5,
		CostUSD:      0.01,
	}); err != nil {
		t.Fatalf("LLMCallInsert: %v", err)
	}

	view := NewExportView(service)
	ctx := tui.ViewContext{From: now.Add(-time.Hour), To: now.Add(time.Hour)}
	updated, _ := view.Update(view.Reload(ctx)(), ctx)
	view = updated.(*ExportView)
	if !strings.Contains(view.preview, "session_id") {
		t.Fatalf("initial preview should show sessions: %q", view.preview)
	}

	updated, cmd := view.Update(tea.KeyMsg{Type: tea.KeyDown}, ctx)
	view = updated.(*ExportView)
	if cmd == nil {
		t.Fatal("changing data type should return preview refresh command")
	}
	updated, _ = view.Update(cmd(), ctx)
	view = updated.(*ExportView)

	if !strings.Contains(view.preview, "started_at,agent,model") {
		t.Fatalf("preview missing llm header after data change: %q", view.preview)
	}
	if !strings.Contains(view.preview, "claude-sonnet-4-5") {
		t.Fatalf("preview missing llm row after data change: %q", view.preview)
	}
}
