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

func (s *Store) ToolCallInsert(p ToolCallParams) (int64, error) {
	successInt := 0
	if p.Success {
		successInt = 1
	}
	var llmCallID interface{}
	if p.LLMCallID != 0 {
		llmCallID = p.LLMCallID
	}
	res, err := s.db.Exec(
		`INSERT INTO tool_calls
		 (session_id, llm_call_id, trace_id, span_id, started_at, duration_ms,
		  agent, tool_name, success, error_message, input_summary, output_summary)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.SessionID, llmCallID, p.TraceID, p.SpanID, p.StartedAt, p.DurationMs,
		p.Agent, p.ToolName, successInt, p.ErrorMessage, p.InputSummary, p.OutputSummary,
	)
	if err != nil {
		return 0, fmt.Errorf("insert tool_call: %w", err)
	}
	id, _ := res.LastInsertId()

	if err := s.SessionIncrementToolCalls(p.SessionID); err != nil {
		return 0, fmt.Errorf("increment session tool count: %w", err)
	}
	return id, nil
}
