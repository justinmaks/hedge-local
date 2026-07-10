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

// Session cost/token/tool-count aggregates are updated only inside
// LLMCallInsert and ToolCallInsert transactions so they can never drift from
// the underlying rows. Do not add standalone mutators for them here.
