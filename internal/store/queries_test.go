package store

import (
	"testing"
	"time"
)

func TestQueryRaw_select(t *testing.T) {
	s := tempDB(t)
	pid, _ := s.ProjectUpsert("/repo", "repo")
	s.SessionUpsert("ext-1", "claude_code", pid, time.Now(), "1.0")

	cols, rows, err := s.QueryRaw("SELECT id, external_id, agent FROM sessions")
	if err != nil {
		t.Fatalf("QueryRaw: %v", err)
	}
	if len(cols) != 3 {
		t.Errorf("expected 3 columns, got %d", len(cols))
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0][1] != "ext-1" {
		t.Errorf("external_id: got %v, want ext-1", rows[0][1])
	}
	if rows[0][2] != "claude_code" {
		t.Errorf("agent: got %v, want claude_code", rows[0][2])
	}
}

func TestQueryRaw_nullColumns(t *testing.T) {
	s := tempDB(t)
	pid, _ := s.ProjectUpsert("/repo", "repo")
	s.SessionUpsert("ext-1", "claude_code", pid, time.Now(), "1.0")

	// ended_at is NULL for a session that has not ended.
	cols, rows, err := s.QueryRaw("SELECT external_id, ended_at FROM sessions")
	if err != nil {
		t.Fatalf("QueryRaw with NULL column: %v", err)
	}
	if len(cols) != 2 || len(rows) != 1 {
		t.Fatalf("expected 2 cols 1 row, got %d cols %d rows", len(cols), len(rows))
	}
	if rows[0][1] != "NULL" {
		t.Errorf("NULL column should render as NULL, got %q", rows[0][1])
	}
}

func TestQueryRaw_rejectsNonSelect(t *testing.T) {
	s := tempDB(t)
	_, _, err := s.QueryRaw("DELETE FROM sessions")
	if err == nil {
		t.Error("expected error for non-SELECT, got nil")
	}
}
