package store

import (
	"fmt"
	"time"
)

type EventParams struct {
	SessionID int64
	Timestamp time.Time
	Agent     string
	EventName string
	Payload   string
	TraceID   string
	SpanID    string
}

func (s *Store) EventInsert(p EventParams) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO events (session_id, timestamp, agent, event_name, payload, trace_id, span_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.SessionID, p.Timestamp, p.Agent, p.EventName, p.Payload, p.TraceID, p.SpanID,
	)
	if err != nil {
		return 0, fmt.Errorf("insert event: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}
