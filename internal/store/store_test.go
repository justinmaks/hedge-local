package store

import (
	"path/filepath"
	"testing"
)

func tempDB(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("New store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMigrations_createTables(t *testing.T) {
	s := tempDB(t)
	var count int
	err := s.db.QueryRow(
		`SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('projects','sessions','llm_calls','tool_calls','events','budgets','pricing','meta')`,
	).Scan(&count)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 8 {
		t.Errorf("expected 8 tables, got %d", count)
	}
}

func TestMigrations_idempotent(t *testing.T) {
	s := tempDB(t)
	if err := s.migrate(); err != nil {
		t.Errorf("second migration run failed: %v", err)
	}
}
