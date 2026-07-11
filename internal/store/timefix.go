package store

import (
	"database/sql"
	"errors"
	"fmt"
)

const timestampsMetaKey = "timestamps_utc_v1"

// timestampColumns lists every column that normalizeTimestamps rewrites into
// TimeLayout. Keep in sync with the schema.
var timestampColumns = []struct {
	table  string
	column string
}{
	{"sessions", "started_at"},
	{"sessions", "ended_at"},
	{"llm_calls", "started_at"},
	{"tool_calls", "started_at"},
	{"events", "timestamp"},
	{"pricing", "effective_from"},
	{"pricing", "effective_to"},
}

// normalizeTimestamps rewrites timestamps written by older versions (the
// driver's Go time.String() serialization, local timezone) into TimeLayout.
// Runs once, gated by a meta key. Values that cannot be parsed are left
// alone rather than destroyed.
func (s *Store) normalizeTimestamps() error {
	var done string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, timestampsMetaKey).Scan(&done)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check timestamp conversion flag: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin timestamp conversion: %w", err)
	}
	defer tx.Rollback()

	for _, tc := range timestampColumns {
		// CAST defeats the driver's decltype-based time parsing so we see
		// the raw stored text.
		q := fmt.Sprintf(
			`SELECT rowid, CAST(%s AS TEXT) FROM %s WHERE %s IS NOT NULL`,
			tc.column, tc.table, tc.column,
		)
		rows, err := tx.Query(q)
		if err != nil {
			return fmt.Errorf("read %s.%s: %w", tc.table, tc.column, err)
		}
		type fix struct {
			rowid int64
			value string
		}
		var fixes []fix
		for rows.Next() {
			var rowid int64
			var raw string
			if err := rows.Scan(&rowid, &raw); err != nil {
				rows.Close()
				return fmt.Errorf("scan %s.%s: %w", tc.table, tc.column, err)
			}
			t, ok := ParseTime(raw)
			if !ok {
				continue
			}
			formatted := FormatTime(t)
			if formatted != raw {
				fixes = append(fixes, fix{rowid: rowid, value: formatted})
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("iterate %s.%s: %w", tc.table, tc.column, err)
		}
		rows.Close()

		update := fmt.Sprintf(`UPDATE %s SET %s = ? WHERE rowid = ?`, tc.table, tc.column)
		for _, f := range fixes {
			if _, err := tx.Exec(update, f.value, f.rowid); err != nil {
				return fmt.Errorf("rewrite %s.%s: %w", tc.table, tc.column, err)
			}
		}
	}

	if _, err := tx.Exec(`INSERT INTO meta (key, value) VALUES (?, '1')`, timestampsMetaKey); err != nil {
		return fmt.Errorf("record timestamp conversion: %w", err)
	}
	return tx.Commit()
}
