package store

import (
	"testing"
	"time"
)

func TestLLMCallInsert(t *testing.T) {
	s := tempDB(t)
	pid, _ := s.ProjectUpsert("/repo", "repo")
	sid, _ := s.SessionUpsert("ext-1", "claude_code", pid, time.Now(), "1.0")

	params := LLMCallParams{
		SessionID:        sid,
		TraceID:          "trace-abc",
		SpanID:           "span-1",
		StartedAt:        time.Now(),
		DurationMs:       1200,
		Agent:            "claude_code",
		Model:            "claude-sonnet-4",
		Provider:         "anthropic",
		InputTokens:      1000,
		OutputTokens:     500,
		CacheReadTokens:  200,
		CacheWriteTokens: 50,
		CostUSD:          0.012,
		TTFTMs:           840,
		StopReason:       "end_turn",
	}
	id, err := s.LLMCallInsert(params)
	if err != nil {
		t.Fatalf("LLMCallInsert: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero llm_call ID")
	}

	var gotModel string
	var gotCost float64
	s.db.QueryRow(`SELECT model, cost_usd FROM llm_calls WHERE id = ?`, id).Scan(&gotModel, &gotCost)
	if gotModel != "claude-sonnet-4" {
		t.Errorf("model: got %q, want claude-sonnet-4", gotModel)
	}
	if gotCost != 0.012 {
		t.Errorf("cost: got %v, want 0.012", gotCost)
	}

	var sessionCost float64
	s.db.QueryRow(`SELECT total_cost_usd FROM sessions WHERE id = ?`, sid).Scan(&sessionCost)
	if sessionCost != 0.012 {
		t.Errorf("session aggregate cost: got %v, want 0.012", sessionCost)
	}
}
