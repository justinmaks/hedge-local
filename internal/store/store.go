package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func New(path string) (*Store, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	// MkdirAll is a no-op on an existing dir, so enforce 0700 explicitly to
	// downgrade directories created by older versions (which used 0755).
	_ = os.Chmod(dir, 0700)
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.normalizeTimestamps(); err != nil {
		db.Close()
		return nil, fmt.Errorf("normalize timestamps: %w", err)
	}
	// The database can contain prompt/log content with --with-logs, so keep it
	// owner-only. WAL/SHM sidecars are created by migrate's writes.
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if _, err := os.Stat(path + suffix); err == nil {
			_ = os.Chmod(path+suffix, 0600)
		}
	}
	return s, nil
}

// NewReadOnly opens an existing database for read-only access. The connection
// sets the query_only PRAGMA so any write (DML, DDL, or PRAGMA write) is
// refused at the SQLite level. It does not run migrations and assumes the
// database already exists. Used by `hcli query` as defense-in-depth behind the
// SELECT/WITH prefix check.
func NewReadOnly(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=query_only(true)&_pragma=busy_timeout(5000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite (read-only): %w", err)
	}
	db.SetMaxOpenConns(1)
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	return s.db
}
