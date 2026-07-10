package store

import (
	"testing"
	"time"
)

func TestToolCallInsert(t *testing.T) {
	s := tempDB(t)
	pid, _ := s.ProjectUpsert("/repo", "repo")
	sid, _ := s.SessionUpsert("ext-1", "claude_code", pid, time.Now(), "1.0")
	llmID, _ := s.LLMCallInsert(LLMCallParams{
		SessionID: sid, StartedAt: time.Now(), Agent: "claude_code", Model: "x",
	})

	params := ToolCallParams{
		SessionID:  sid,
		LLMCallID:  llmID,
		TraceID:    "trace-abc",
		SpanID:     "span-tool-1",
		StartedAt:  time.Now(),
		DurationMs: 240,
		Agent:      "claude_code",
		ToolName:   "bash",
		Success:    true,
	}
	id, err := s.ToolCallInsert(params)
	if err != nil {
		t.Fatalf("ToolCallInsert: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero tool_call ID")
	}

	var toolCount int
	s.db.QueryRow(`SELECT tool_call_count FROM sessions WHERE id = ?`, sid).Scan(&toolCount)
	if toolCount != 1 {
		t.Errorf("session tool_call_count: got %d, want 1", toolCount)
	}
}

func TestToolCallInsert_duplicateSpanIgnored(t *testing.T) {
	s := tempDB(t)
	pid, _ := s.ProjectUpsert("/repo", "repo")
	sid, _ := s.SessionUpsert("ext-tooldup", "claude_code", pid, time.Now(), "1.0")

	params := ToolCallParams{
		SessionID: sid, SpanID: "span-tool-retry", StartedAt: time.Now(),
		Agent: "claude_code", ToolName: "bash", Success: true,
	}
	if _, err := s.ToolCallInsert(params); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	id2, err := s.ToolCallInsert(params)
	if err != nil {
		t.Fatalf("retry insert should not error: %v", err)
	}
	if id2 != 0 {
		t.Errorf("retry insert should return id 0, got %d", id2)
	}

	var count, toolCount int
	s.db.QueryRow(`SELECT count(*) FROM tool_calls WHERE span_id = 'span-tool-retry'`).Scan(&count)
	s.db.QueryRow(`SELECT tool_call_count FROM sessions WHERE id = ?`, sid).Scan(&toolCount)
	if count != 1 {
		t.Errorf("tool_calls rows: got %d, want 1", count)
	}
	if toolCount != 1 {
		t.Errorf("tool_call_count double-counted: got %d, want 1", toolCount)
	}
}

func TestToolCallInsert_failed(t *testing.T) {
	s := tempDB(t)
	pid, _ := s.ProjectUpsert("/repo", "repo")
	sid, _ := s.SessionUpsert("ext-2", "claude_code", pid, time.Now(), "1.0")

	params := ToolCallParams{
		SessionID:    sid,
		StartedAt:    time.Now(),
		Agent:        "claude_code",
		ToolName:     "bash",
		Success:      false,
		ErrorMessage: "exit 1",
	}
	_, err := s.ToolCallInsert(params)
	if err != nil {
		t.Fatalf("ToolCallInsert failed: %v", err)
	}

	var errMsg string
	s.db.QueryRow(`SELECT error_message FROM tool_calls WHERE session_id = ?`, sid).Scan(&errMsg)
	if errMsg != "exit 1" {
		t.Errorf("error_message: got %q, want %q", errMsg, "exit 1")
	}
}
