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

func (s *Store) LLMCallInsert(p LLMCallParams) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO llm_calls
		 (session_id, trace_id, span_id, parent_span_id, started_at, duration_ms,
		  agent, model, provider, input_tokens, output_tokens,
		  cache_read_tokens, cache_write_tokens, reasoning_tokens,
		  cost_usd, ttft_ms, stop_reason)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.SessionID, p.TraceID, p.SpanID, p.ParentSpanID, p.StartedAt, p.DurationMs,
		p.Agent, p.Model, p.Provider, p.InputTokens, p.OutputTokens,
		p.CacheReadTokens, p.CacheWriteTokens, p.ReasoningTokens,
		p.CostUSD, p.TTFTMs, p.StopReason,
	)
	if err != nil {
		return 0, fmt.Errorf("insert llm_call: %w", err)
	}
	id, _ := res.LastInsertId()

	if err := s.SessionAddCost(p.SessionID, p.CostUSD); err != nil {
		return 0, fmt.Errorf("update session cost: %w", err)
	}
	if err := s.SessionAddTokens(p.SessionID, p.InputTokens, p.OutputTokens, p.CacheReadTokens, p.CacheWriteTokens); err != nil {
		return 0, fmt.Errorf("update session tokens: %w", err)
	}
	return id, nil
}
