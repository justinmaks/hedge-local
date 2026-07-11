package store

import (
	"fmt"
	"time"
)

type LLMCallParams struct {
	SessionID        int64
	TraceID          string
	SpanID           string
	ParentSpanID     string
	StartedAt        time.Time
	DurationMs       int
	Agent            string
	Model            string
	Provider         string
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
	ReasoningTokens  int
	CostUSD          float64
	TTFTMs           int
	StopReason       string
}

// LLMCallInsert inserts the call and updates the owning session's aggregates
// in a single transaction. A call whose span_id already exists (an OTLP
// exporter retry) is ignored entirely, including the aggregate updates, and
// returns id 0 with no error.
func (s *Store) LLMCallInsert(p LLMCallParams) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin llm_call tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`INSERT OR IGNORE INTO llm_calls
		 (session_id, trace_id, span_id, parent_span_id, started_at, duration_ms,
		  agent, model, provider, input_tokens, output_tokens,
		  cache_read_tokens, cache_write_tokens, reasoning_tokens,
		  cost_usd, ttft_ms, stop_reason)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.SessionID, p.TraceID, p.SpanID, p.ParentSpanID, FormatTime(p.StartedAt), p.DurationMs,
		p.Agent, p.Model, p.Provider, p.InputTokens, p.OutputTokens,
		p.CacheReadTokens, p.CacheWriteTokens, p.ReasoningTokens,
		p.CostUSD, p.TTFTMs, p.StopReason,
	)
	if err != nil {
		return 0, fmt.Errorf("insert llm_call: %w", err)
	}
	inserted, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("insert llm_call rows affected: %w", err)
	}
	if inserted == 0 {
		return 0, nil
	}
	id, _ := res.LastInsertId()

	if _, err := tx.Exec(
		`UPDATE sessions SET
		 total_cost_usd = total_cost_usd + ?,
		 total_input_tokens = total_input_tokens + ?,
		 total_output_tokens = total_output_tokens + ?,
		 total_cache_read_tokens = total_cache_read_tokens + ?,
		 total_cache_write_tokens = total_cache_write_tokens + ?,
		 message_count = message_count + 1
		 WHERE id = ?`,
		p.CostUSD, p.InputTokens, p.OutputTokens, p.CacheReadTokens, p.CacheWriteTokens, p.SessionID,
	); err != nil {
		return 0, fmt.Errorf("update session aggregates: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit llm_call tx: %w", err)
	}
	return id, nil
}
