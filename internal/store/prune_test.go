package store

import (
	"testing"
	"time"
)

func seedPruneData(t *testing.T) (*Store, time.Time) {
	t.Helper()
	s := tempDB(t)
	pid, _ := s.ProjectUpsert("/repo", "repo")
	now := time.Now()
	old := now.Add(-100 * 24 * time.Hour)

	oldSess, _ := s.SessionUpsert("ext-old", "claude_code", pid, old, "1.0")
	newSess, _ := s.SessionUpsert("ext-new", "claude_code", pid, now, "1.0")

	if _, err := s.LLMCallInsert(LLMCallParams{
		SessionID: oldSess, SpanID: "span-old", StartedAt: old, Agent: "claude_code", Model: "m", CostUSD: 0.1,
	}); err != nil {
		t.Fatalf("old llm: %v", err)
	}
	if _, err := s.LLMCallInsert(LLMCallParams{
		SessionID: newSess, SpanID: "span-new", StartedAt: now, Agent: "claude_code", Model: "m", CostUSD: 0.2,
	}); err != nil {
		t.Fatalf("new llm: %v", err)
	}
	if _, err := s.ToolCallInsert(ToolCallParams{
		SessionID: oldSess, SpanID: "tspan-old", StartedAt: old, Agent: "claude_code", ToolName: "bash",
	}); err != nil {
		t.Fatalf("old tool: %v", err)
	}
	if _, err := s.EventInsert(EventParams{
		SessionID: oldSess, Timestamp: old, Agent: "claude_code", EventName: "e", Payload: "{}",
	}); err != nil {
		t.Fatalf("old event: %v", err)
	}
	return s, now.Add(-90 * 24 * time.Hour)
}

func TestPruneBefore_deletesOldKeepsNew(t *testing.T) {
	s, cutoff := seedPruneData(t)

	counts, err := s.PruneCounts(cutoff)
	if err != nil {
		t.Fatalf("PruneCounts: %v", err)
	}
	if counts.LLMCalls != 1 || counts.ToolCalls != 1 || counts.Events != 1 || counts.Sessions != 1 {
		t.Errorf("counts: got %+v, want 1 of each", counts)
	}

	result, err := s.PruneBefore(cutoff)
	if err != nil {
		t.Fatalf("PruneBefore: %v", err)
	}
	if result != counts {
		t.Errorf("result %+v should match preview counts %+v", result, counts)
	}

	var sessions, llm, tools, events int
	s.db.QueryRow(`SELECT count(*) FROM sessions`).Scan(&sessions)
	s.db.QueryRow(`SELECT count(*) FROM llm_calls`).Scan(&llm)
	s.db.QueryRow(`SELECT count(*) FROM tool_calls`).Scan(&tools)
	s.db.QueryRow(`SELECT count(*) FROM events`).Scan(&events)
	if sessions != 1 || llm != 1 || tools != 0 || events != 0 {
		t.Errorf("after prune: sessions=%d llm=%d tools=%d events=%d, want 1/1/0/0",
			sessions, llm, tools, events)
	}

	var kept string
	s.db.QueryRow(`SELECT external_id FROM sessions`).Scan(&kept)
	if kept != "ext-new" {
		t.Errorf("kept session: got %q, want ext-new", kept)
	}
}

func TestPruneBefore_keepsOldSessionWithRecentSpans(t *testing.T) {
	s := tempDB(t)
	pid, _ := s.ProjectUpsert("/repo", "repo")
	now := time.Now()
	old := now.Add(-100 * 24 * time.Hour)

	// A session that started long ago but has recent activity.
	sess, _ := s.SessionUpsert("ext-longrunning", "claude_code", pid, old, "1.0")
	if _, err := s.LLMCallInsert(LLMCallParams{
		SessionID: sess, SpanID: "span-recent", StartedAt: now, Agent: "claude_code", Model: "m",
	}); err != nil {
		t.Fatalf("recent llm: %v", err)
	}

	cutoff := now.Add(-90 * 24 * time.Hour)
	if _, err := s.PruneBefore(cutoff); err != nil {
		t.Fatalf("PruneBefore: %v", err)
	}

	var sessions int
	s.db.QueryRow(`SELECT count(*) FROM sessions`).Scan(&sessions)
	if sessions != 1 {
		t.Errorf("long-running session with recent spans should survive, got %d sessions", sessions)
	}
}
