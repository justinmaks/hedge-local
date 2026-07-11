package store

import (
	"testing"
	"time"
)

func TestSessionUpsert_new(t *testing.T) {
	s := tempDB(t)
	pid, _ := s.ProjectUpsert("/repo", "repo")
	started := time.Now()
	id, err := s.SessionUpsert("ext-123", "claude_code", pid, started, "1.0.0")
	if err != nil {
		t.Fatalf("SessionUpsert: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero session ID")
	}
}

func TestSessionUpsert_idempotent(t *testing.T) {
	s := tempDB(t)
	pid, _ := s.ProjectUpsert("/repo", "repo")
	started := time.Now()
	id1, _ := s.SessionUpsert("ext-123", "claude_code", pid, started, "1.0.0")
	id2, _ := s.SessionUpsert("ext-123", "claude_code", pid, started, "1.0.0")
	if id1 != id2 {
		t.Errorf("expected same ID on re-upsert: got %d then %d", id1, id2)
	}
}

func TestSessionSetEnded(t *testing.T) {
	s := tempDB(t)
	pid, _ := s.ProjectUpsert("/repo", "repo")
	started := time.Now()
	id, _ := s.SessionUpsert("ext-123", "claude_code", pid, started, "1.0.0")
	ended := started.Add(5 * time.Minute)
	if err := s.SessionSetEnded(id, ended); err != nil {
		t.Fatalf("SessionSetEnded: %v", err)
	}
	var got time.Time
	s.db.QueryRow(`SELECT ended_at FROM sessions WHERE id = ?`, id).Scan(&got)
	// Storage keeps millisecond precision.
	if diff := got.Sub(ended); diff > time.Millisecond || diff < -time.Millisecond {
		t.Errorf("ended_at: got %v, want %v (within 1ms)", got, ended)
	}
}

func TestSessionAggregates_accumulateAcrossInserts(t *testing.T) {
	s := tempDB(t)
	pid, _ := s.ProjectUpsert("/repo", "repo")
	id, _ := s.SessionUpsert("ext-tok", "claude_code", pid, time.Now(), "1.0")

	insert := func(span string, input, output, cacheRead, cacheWrite int, cost float64) {
		t.Helper()
		_, err := s.LLMCallInsert(LLMCallParams{
			SessionID: id, SpanID: span, StartedAt: time.Now(), Agent: "claude_code",
			Model: "claude-sonnet-4", Provider: "anthropic",
			InputTokens: input, OutputTokens: output,
			CacheReadTokens: cacheRead, CacheWriteTokens: cacheWrite, CostUSD: cost,
		})
		if err != nil {
			t.Fatalf("LLMCallInsert %s: %v", span, err)
		}
	}
	insert("span-agg-1", 1000, 500, 200, 50, 0.50)
	insert("span-agg-2", 2000, 1000, 400, 100, 0.25)

	var inTok, outTok, crTok, cwTok, msgCount int
	var cost float64
	s.db.QueryRow(
		`SELECT total_input_tokens, total_output_tokens, total_cache_read_tokens, total_cache_write_tokens, message_count, total_cost_usd FROM sessions WHERE id = ?`,
		id,
	).Scan(&inTok, &outTok, &crTok, &cwTok, &msgCount, &cost)

	if inTok != 3000 {
		t.Errorf("input tokens: got %d, want 3000", inTok)
	}
	if outTok != 1500 {
		t.Errorf("output tokens: got %d, want 1500", outTok)
	}
	if crTok != 600 {
		t.Errorf("cache read tokens: got %d, want 600", crTok)
	}
	if cwTok != 150 {
		t.Errorf("cache write tokens: got %d, want 150", cwTok)
	}
	if msgCount != 2 {
		t.Errorf("message_count: got %d, want 2", msgCount)
	}
	if abs(cost-0.75) > 0.0001 {
		t.Errorf("total_cost_usd: got %v, want 0.75", cost)
	}
}
