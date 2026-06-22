package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	migrationfiles "github.com/justinmaks/hedge-local/migrations"
)

var migrationFS = migrationfiles.FS

func (s *Store) migrate() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := migrationFS.ReadDir(".")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	var files []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		version := strings.TrimSuffix(name, ".sql")
		var applied bool
		err := s.db.QueryRow(`SELECT 1 FROM schema_migrations WHERE version = ?`, version).Scan(&applied)
		if errors.Is(err, sql.ErrNoRows) {
			// not applied yet, proceed
		} else if err != nil {
			return fmt.Errorf("check migration %s: %w", version, err)
		} else {
			continue
		}

		content, err := migrationFS.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}

		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", version, err)
		}
		if _, err := tx.Exec(string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("exec %s: %w", version, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, version); err != nil {
			tx.Rollback()
			return fmt.Errorf("record %s: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit %s: %w", version, err)
		}
	}
	return nil
}
