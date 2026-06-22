package views

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/justinmaks/hedge-local/internal/store"
	"github.com/justinmaks/hedge-local/internal/tui"
	"github.com/justinmaks/hedge-local/internal/tui/queries"
)

func TestToolsViewReloadRefreshesFocusedDetailRows(t *testing.T) {
	service, db := testQueryService(t)
	projectID, err := db.ProjectUpsert("/tmp/tools-project", "tools-project")
	if err != nil {
		t.Fatalf("ProjectUpsert: %v", err)
	}

	older := time.Date(2026, time.January, 9, 10, 0, 0, 0, time.Local)
	newer := time.Date(2026, time.January, 15, 12, 0, 0, 0, time.Local)
	insertToolCall(t, db, projectID, "sess-old-tool", older, "bash")
	insertToolCall(t, db, projectID, "sess-new-tool", newer, "bash")

	view := NewToolsView(service)
	olderCtx := tui.ViewContext{From: older.Add(-time.Hour), To: older.Add(time.Hour)}
	loadToolsView(t, view, olderCtx)
	view.Update(tea.KeyMsg{Type: tea.KeyEnter}, olderCtx)

	if len(view.detailRows) != 1 {
		t.Fatalf("initial detail rows: got %d, want 1", len(view.detailRows))
	}
	if got, want := view.detailRows[0][0], older.Format("01-02 15:04:05"); got != want {
		t.Fatalf("initial detail timestamp: got %q, want %q", got, want)
	}

	newerCtx := tui.ViewContext{From: newer.Add(-time.Hour), To: newer.Add(time.Hour)}
	loadToolsView(t, view, newerCtx)

	if got, want := view.detailRows[0][0], newer.Format("01-02 15:04:05"); got != want {
		t.Fatalf("detail timestamp after reload: got %q, want %q", got, want)
	}
}

func TestToolsViewFocusedDetailScrollUsesDetailTable(t *testing.T) {
	service, db := testQueryService(t)
	projectID, err := db.ProjectUpsert("/tmp/tools-scroll-project", "tools-scroll-project")
	if err != nil {
		t.Fatalf("ProjectUpsert: %v", err)
	}

	base := time.Date(2026, time.January, 20, 10, 0, 0, 0, time.Local)
	for i := 0; i < 3; i++ {
		insertToolCall(t, db, projectID, fmt.Sprintf("sess-scroll-%d", i), base.Add(time.Duration(i)*time.Minute), "bash")
	}

	view := NewToolsView(service)
	ctx := tui.ViewContext{From: base.Add(-time.Hour), To: base.Add(time.Hour)}
	loadToolsView(t, view, ctx)
	view.Update(tea.KeyMsg{Type: tea.KeyEnter}, ctx)
	if !view.focused {
		t.Fatal("expected focused detail mode")
	}
	view.Update(tea.KeyMsg{Type: tea.KeyDown}, ctx)
	if got, want := view.detailTable.Cursor(), 1; got != want {
		t.Fatalf("detail cursor after scroll: got %d, want %d", got, want)
	}
}

func TestToolsViewEnterUsesSortedRowSelection(t *testing.T) {
	service, db := testQueryService(t)
	projectID, err := db.ProjectUpsert("/tmp/tools-sort-project", "tools-sort-project")
	if err != nil {
		t.Fatalf("ProjectUpsert: %v", err)
	}
	base := time.Date(2026, time.January, 21, 10, 0, 0, 0, time.Local)
	insertToolCall(t, db, projectID, "sess-alpha", base, "alpha")
	insertToolCall(t, db, projectID, "sess-beta-1", base.Add(time.Minute), "beta")
	insertToolCall(t, db, projectID, "sess-beta-2", base.Add(2*time.Minute), "beta")

	view := NewToolsView(service)
	ctx := tui.ViewContext{From: base.Add(-time.Hour), To: base.Add(time.Hour)}
	loadToolsView(t, view, ctx)
	view.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}}, ctx)
	view.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}}, ctx)
	view.Update(tea.KeyMsg{Type: tea.KeyEnter}, ctx)
	if got, want := view.selected, "beta"; got != want {
		t.Fatalf("selected tool after sorted enter: got %q, want %q", got, want)
	}
}

func TestToolsViewDetailReloadPreservesCursor(t *testing.T) {
	service, db := testQueryService(t)
	projectID, err := db.ProjectUpsert("/tmp/tools-preserve-project", "tools-preserve-project")
	if err != nil {
		t.Fatalf("ProjectUpsert: %v", err)
	}
	older := time.Date(2026, time.January, 22, 10, 0, 0, 0, time.Local)
	newer := time.Date(2026, time.January, 22, 12, 0, 0, 0, time.Local)
	for i := 0; i < 3; i++ {
		insertToolCall(t, db, projectID, fmt.Sprintf("sess-old-%d", i), older.Add(time.Duration(i)*time.Minute), "bash")
		insertToolCall(t, db, projectID, fmt.Sprintf("sess-new-%d", i), newer.Add(time.Duration(i)*time.Minute), "bash")
	}

	view := NewToolsView(service)
	olderCtx := tui.ViewContext{From: older.Add(-time.Hour), To: older.Add(time.Hour)}
	loadToolsView(t, view, olderCtx)
	view.Update(tea.KeyMsg{Type: tea.KeyEnter}, olderCtx)
	view.Update(tea.KeyMsg{Type: tea.KeyDown}, olderCtx)
	if got, want := view.detailTable.Cursor(), 1; got != want {
		t.Fatalf("detail cursor before reload: got %d, want %d", got, want)
	}

	newerCtx := tui.ViewContext{From: newer.Add(-time.Hour), To: newer.Add(time.Hour)}
	loadToolsView(t, view, newerCtx)
	if got, want := view.detailTable.Cursor(), 1; got != want {
		t.Fatalf("detail cursor after reload: got %d, want %d", got, want)
	}
}

func TestProjectsViewReloadRefreshesFocusedDetailRows(t *testing.T) {
	service, db := testQueryService(t)
	projectID, err := db.ProjectUpsert("/tmp/projects-project", "projects-project")
	if err != nil {
		t.Fatalf("ProjectUpsert: %v", err)
	}

	older := time.Date(2026, time.January, 9, 10, 0, 0, 0, time.Local)
	newer := time.Date(2026, time.January, 15, 12, 0, 0, 0, time.Local)
	insertSession(t, db, projectID, "sess-old-project", older)
	insertSession(t, db, projectID, "sess-new-project", newer)

	view := NewProjectsView(service)
	olderCtx := tui.ViewContext{From: older.Add(-time.Hour), To: older.Add(time.Hour)}
	loadProjectsView(t, view, olderCtx)
	view.Update(tea.KeyMsg{Type: tea.KeyEnter}, olderCtx)

	if len(view.detailTable.Rows) != 1 {
		t.Fatalf("initial detail rows: got %d, want 1", len(view.detailTable.Rows))
	}
	if got, want := view.detailTable.Rows[0][0], older.Format("01-02 15:04"); got != want {
		t.Fatalf("initial detail timestamp: got %q, want %q", got, want)
	}

	newerCtx := tui.ViewContext{From: newer.Add(-time.Hour), To: newer.Add(time.Hour)}
	loadProjectsView(t, view, newerCtx)

	if got, want := view.detailTable.Rows[0][0], newer.Format("01-02 15:04"); got != want {
		t.Fatalf("detail timestamp after reload: got %q, want %q", got, want)
	}
}

func testQueryService(t *testing.T) (*queries.Service, *store.Store) {
	t.Helper()
	db, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return queries.NewService(db), db
}

func loadToolsView(t *testing.T, view *ToolsView, ctx tui.ViewContext) {
	t.Helper()
	cmd := view.Reload(ctx)
	if cmd == nil {
		t.Fatal("ToolsView.Reload returned nil command")
	}
	updated, _ := view.Update(cmd(), ctx)
	loaded, ok := updated.(*ToolsView)
	if !ok {
		t.Fatalf("updated view type: got %T, want *ToolsView", updated)
	}
	*view = *loaded
}

func loadProjectsView(t *testing.T, view *ProjectsView, ctx tui.ViewContext) {
	t.Helper()
	cmd := view.Reload(ctx)
	if cmd == nil {
		t.Fatal("ProjectsView.Reload returned nil command")
	}
	updated, _ := view.Update(cmd(), ctx)
	loaded, ok := updated.(*ProjectsView)
	if !ok {
		t.Fatalf("updated view type: got %T, want *ProjectsView", updated)
	}
	*view = *loaded
}

func insertSession(t *testing.T, db *store.Store, projectID int64, externalID string, startedAt time.Time) int64 {
	t.Helper()
	sessionID, err := db.SessionUpsert(externalID, "claude_code", projectID, startedAt, "")
	if err != nil {
		t.Fatalf("SessionUpsert: %v", err)
	}
	return sessionID
}

func insertToolCall(t *testing.T, db *store.Store, projectID int64, externalID string, startedAt time.Time, toolName string) {
	t.Helper()
	sessionID := insertSession(t, db, projectID, externalID, startedAt)
	if _, err := db.ToolCallInsert(store.ToolCallParams{
		SessionID:  sessionID,
		StartedAt:  startedAt,
		DurationMs: 250,
		Agent:      "claude_code",
		ToolName:   toolName,
		Success:    true,
	}); err != nil {
		t.Fatalf("ToolCallInsert: %v", err)
	}
}
