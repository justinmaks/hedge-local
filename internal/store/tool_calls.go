package store

import (
	"fmt"
	"time"
)

type ToolCallParams struct {
	SessionID     int64
	LLMCallID     int64
	TraceID       string
	SpanID        string
	StartedAt     time.Time
	DurationMs    int
	Agent         string
	ToolName      string
	Success       bool
	ErrorMessage  string
	InputSummary  string
	OutputSummary string
}

// ToolCallInsert inserts the call and bumps the owning session's tool count
// in a single transaction. A call whose span_id already exists (an OTLP
// exporter retry) is ignored entirely, including the counter bump, and
// returns id 0 with no error.
func (s *Store) ToolCallInsert(p ToolCallParams) (int64, error) {
	successInt := 0
	if p.Success {
		successInt = 1
	}
	var llmCallID any
	if p.LLMCallID != 0 {
		llmCallID = p.LLMCallID
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin tool_call tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`INSERT OR IGNORE INTO tool_calls
		 (session_id, llm_call_id, trace_id, span_id, started_at, duration_ms,
		  agent, tool_name, success, error_message, input_summary, output_summary)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.SessionID, llmCallID, p.TraceID, p.SpanID, FormatTime(p.StartedAt), p.DurationMs,
		p.Agent, p.ToolName, successInt, p.ErrorMessage, p.InputSummary, p.OutputSummary,
	)
	if err != nil {
		return 0, fmt.Errorf("insert tool_call: %w", err)
	}
	inserted, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("insert tool_call rows affected: %w", err)
	}
	if inserted == 0 {
		return 0, nil
	}
	id, _ := res.LastInsertId()

	if _, err := tx.Exec(`UPDATE sessions SET tool_call_count = tool_call_count + 1 WHERE id = ?`, p.SessionID); err != nil {
		return 0, fmt.Errorf("increment session tool count: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit tool_call tx: %w", err)
	}
	return id, nil
}
