package store

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNew_restrictsPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits not meaningful on Windows")
	}
	dir := filepath.Join(t.TempDir(), "hedge")
	dbFile := filepath.Join(dir, "hedge.db")
	s, err := New(dbFile)
	if err != nil {
		t.Fatalf("New store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Errorf("data dir perm = %o, want 700", perm)
	}

	fileInfo, err := os.Stat(dbFile)
	if err != nil {
		t.Fatalf("stat db: %v", err)
	}
	if perm := fileInfo.Mode().Perm(); perm != 0o600 {
		t.Errorf("db file perm = %o, want 600", perm)
	}
}

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

func TestNewReadOnly_rejectsWrites(t *testing.T) {
	dbFile := filepath.Join(t.TempDir(), "hedge.db")
	// Create schema with a normal read-write store, then close it.
	rw, err := New(dbFile)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rw.Close()

	ro, err := NewReadOnly(dbFile)
	if err != nil {
		t.Fatalf("NewReadOnly: %v", err)
	}
	t.Cleanup(func() { ro.Close() })

	// Reads work.
	if _, _, err := ro.QueryRaw("SELECT count(*) FROM sessions"); err != nil {
		t.Errorf("read-only SELECT failed: %v", err)
	}

	// Writes are refused at the connection level.
	if _, err := ro.DB().Exec("INSERT INTO meta(key, value) VALUES ('x', 'y')"); err == nil {
		t.Errorf("expected write to be rejected on read-only connection")
	}
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
