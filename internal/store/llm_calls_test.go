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

func TestLLMCallInsert_duplicateSpanIgnored(t *testing.T) {
	s := tempDB(t)
	pid, _ := s.ProjectUpsert("/repo", "repo")
	sid, _ := s.SessionUpsert("ext-dup", "claude_code", pid, time.Now(), "1.0")

	params := LLMCallParams{
		SessionID: sid, SpanID: "span-retry", StartedAt: time.Now(),
		Agent: "claude_code", Model: "claude-sonnet-4", Provider: "anthropic",
		InputTokens: 1000, OutputTokens: 500, CostUSD: 0.02,
	}

	id1, err := s.LLMCallInsert(params)
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if id1 == 0 {
		t.Fatal("first insert should return a real id")
	}

	// Same span again, as an OTLP exporter retry would send it.
	id2, err := s.LLMCallInsert(params)
	if err != nil {
		t.Fatalf("retry insert should not error: %v", err)
	}
	if id2 != 0 {
		t.Errorf("retry insert should return id 0, got %d", id2)
	}

	var count int
	s.db.QueryRow(`SELECT count(*) FROM llm_calls WHERE span_id = 'span-retry'`).Scan(&count)
	if count != 1 {
		t.Errorf("llm_calls rows: got %d, want 1", count)
	}

	var cost float64
	var msgCount int
	s.db.QueryRow(`SELECT total_cost_usd, message_count FROM sessions WHERE id = ?`, sid).Scan(&cost, &msgCount)
	if abs(cost-0.02) > 0.0001 {
		t.Errorf("session cost double-counted: got %v, want 0.02", cost)
	}
	if msgCount != 1 {
		t.Errorf("message_count double-counted: got %d, want 1", msgCount)
	}
}

func TestLLMCallInsert_emptySpanIDsAllDistinct(t *testing.T) {
	s := tempDB(t)
	pid, _ := s.ProjectUpsert("/repo", "repo")
	sid, _ := s.SessionUpsert("ext-nospan", "claude_code", pid, time.Now(), "1.0")

	// Calls without span IDs cannot be deduped and must all insert.
	for i := range 2 {
		if _, err := s.LLMCallInsert(LLMCallParams{
			SessionID: sid, StartedAt: time.Now(), Agent: "claude_code", Model: "m",
		}); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}
	var count int
	s.db.QueryRow(`SELECT count(*) FROM llm_calls WHERE session_id = ?`, sid).Scan(&count)
	if count != 2 {
		t.Errorf("empty-span rows: got %d, want 2", count)
	}
}
