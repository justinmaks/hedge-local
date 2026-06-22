package store

import (
	"fmt"
	"time"
)

func (s *Store) SessionUpsert(externalID, agent string, projectID int64, startedAt time.Time, appVersion string) (int64, error) {
	_, err := s.db.Exec(
		`INSERT INTO sessions (external_id, agent, project_id, started_at, app_version)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(external_id) DO NOTHING`,
		externalID, agent, projectID, startedAt, appVersion,
	)
	if err != nil {
		return 0, fmt.Errorf("upsert session: %w", err)
	}
	var id int64
	err = s.db.QueryRow(`SELECT id FROM sessions WHERE external_id = ?`, externalID).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("get session id: %w", err)
	}
	return id, nil
}

func (s *Store) SessionSetEnded(id int64, endedAt time.Time) error {
	_, err := s.db.Exec(`UPDATE sessions SET ended_at = ? WHERE id = ?`, endedAt, id)
	return err
}

func (s *Store) SessionAddCost(id int64, cost float64) error {
	_, err := s.db.Exec(`UPDATE sessions SET total_cost_usd = total_cost_usd + ? WHERE id = ?`, cost, id)
	return err
}

func (s *Store) SessionAddTokens(id int64, input, output, cacheRead, cacheWrite int) error {
	_, err := s.db.Exec(
		`UPDATE sessions SET
		 total_input_tokens = total_input_tokens + ?,
		 total_output_tokens = total_output_tokens + ?,
		 total_cache_read_tokens = total_cache_read_tokens + ?,
		 total_cache_write_tokens = total_cache_write_tokens + ?,
		 message_count = message_count + 1
		 WHERE id = ?`,
		input, output, cacheRead, cacheWrite, id,
	)
	return err
}

func (s *Store) SessionIncrementToolCalls(id int64) error {
	_, err := s.db.Exec(`UPDATE sessions SET tool_call_count = tool_call_count + 1 WHERE id = ?`, id)
	return err
}
