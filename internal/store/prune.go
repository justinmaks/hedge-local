package store

import (
	"fmt"
	"time"
)

type PruneResult struct {
	LLMCalls  int64
	ToolCalls int64
	Events    int64
	Sessions  int64
}

// PruneCounts reports what PruneBefore would delete, without deleting.
func (s *Store) PruneCounts(cutoff time.Time) (PruneResult, error) {
	var r PruneResult
	c := FormatTime(cutoff)
	steps := []struct {
		dst   *int64
		query string
	}{
		{&r.LLMCalls, `SELECT COUNT(*) FROM llm_calls WHERE started_at < ?`},
		{&r.ToolCalls, `SELECT COUNT(*) FROM tool_calls WHERE started_at < ?`},
		{&r.Events, `SELECT COUNT(*) FROM events WHERE timestamp < ?`},
		{&r.Sessions, `SELECT COUNT(*) FROM sessions WHERE started_at < ?
			AND id NOT IN (SELECT DISTINCT session_id FROM llm_calls WHERE started_at >= ?)
			AND id NOT IN (SELECT DISTINCT session_id FROM tool_calls WHERE started_at >= ?)
			AND id NOT IN (SELECT DISTINCT session_id FROM events WHERE timestamp >= ?)`},
	}
	for _, step := range steps {
		args := []any{c}
		if step.dst == &r.Sessions {
			args = []any{c, c, c, c}
		}
		if err := s.db.QueryRow(step.query, args...).Scan(step.dst); err != nil {
			return r, fmt.Errorf("prune count: %w", err)
		}
	}
	return r, nil
}

// PruneBefore deletes spans and events older than cutoff, then sessions that
// started before cutoff and have no remaining spans. Runs in one transaction.
func (s *Store) PruneBefore(cutoff time.Time) (PruneResult, error) {
	var r PruneResult
	c := FormatTime(cutoff)

	tx, err := s.db.Begin()
	if err != nil {
		return r, fmt.Errorf("begin prune: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.Exec(`DELETE FROM llm_calls WHERE started_at < ?`, c)
	if err != nil {
		return r, fmt.Errorf("prune llm_calls: %w", err)
	}
	r.LLMCalls, _ = res.RowsAffected()

	res, err = tx.Exec(`DELETE FROM tool_calls WHERE started_at < ?`, c)
	if err != nil {
		return r, fmt.Errorf("prune tool_calls: %w", err)
	}
	r.ToolCalls, _ = res.RowsAffected()

	res, err = tx.Exec(`DELETE FROM events WHERE timestamp < ?`, c)
	if err != nil {
		return r, fmt.Errorf("prune events: %w", err)
	}
	r.Events, _ = res.RowsAffected()

	// Only sessions whose spans are all gone; a long-running session with
	// recent activity survives even if it started before the cutoff.
	res, err = tx.Exec(
		`DELETE FROM sessions WHERE started_at < ?
		 AND id NOT IN (SELECT DISTINCT session_id FROM llm_calls)
		 AND id NOT IN (SELECT DISTINCT session_id FROM tool_calls)
		 AND id NOT IN (SELECT DISTINCT session_id FROM events)`,
		c,
	)
	if err != nil {
		return r, fmt.Errorf("prune sessions: %w", err)
	}
	r.Sessions, _ = res.RowsAffected()

	if err := tx.Commit(); err != nil {
		return r, fmt.Errorf("commit prune: %w", err)
	}
	return r, nil
}

// Vacuum reclaims disk space after large deletions.
func (s *Store) Vacuum() error {
	_, err := s.db.Exec(`VACUUM`)
	return err
}
