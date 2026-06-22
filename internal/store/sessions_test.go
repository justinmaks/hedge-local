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
	if !got.Equal(ended) {
		t.Errorf("ended_at: got %v, want %v", got, ended)
	}
}

func TestSessionAddTokens(t *testing.T) {
	s := tempDB(t)
	pid, _ := s.ProjectUpsert("/repo", "repo")
	id, _ := s.SessionUpsert("ext-tok", "claude_code", pid, time.Now(), "1.0")

	if err := s.SessionAddTokens(id, 1000, 500, 200, 50); err != nil {
		t.Fatalf("SessionAddTokens: %v", err)
	}
	if err := s.SessionAddTokens(id, 2000, 1000, 400, 100); err != nil {
		t.Fatalf("SessionAddTokens second: %v", err)
	}

	var inTok, outTok, crTok, cwTok, msgCount int
	s.db.QueryRow(
		`SELECT total_input_tokens, total_output_tokens, total_cache_read_tokens, total_cache_write_tokens, message_count FROM sessions WHERE id = ?`,
		id,
	).Scan(&inTok, &outTok, &crTok, &cwTok, &msgCount)

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
}

func TestSessionAddCost(t *testing.T) {
	s := tempDB(t)
	pid, _ := s.ProjectUpsert("/repo", "repo")
	id, _ := s.SessionUpsert("ext-cost", "claude_code", pid, time.Now(), "1.0")

	if err := s.SessionAddCost(id, 0.50); err != nil {
		t.Fatalf("SessionAddCost: %v", err)
	}
	if err := s.SessionAddCost(id, 0.25); err != nil {
		t.Fatalf("SessionAddCost second: %v", err)
	}

	var cost float64
	s.db.QueryRow(`SELECT total_cost_usd FROM sessions WHERE id = ?`, id).Scan(&cost)
	if abs(cost-0.75) > 0.0001 {
		t.Errorf("total_cost_usd: got %v, want 0.75", cost)
	}
}
