package collect

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/justinmaks/hedge-local/internal/normalize"
	"github.com/justinmaks/hedge-local/internal/store"
)

func writerTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := s.SeedBundledPricing(); err != nil {
		t.Fatalf("SeedBundledPricing: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestWriter_WriteLLMCall(t *testing.T) {
	s := writerTestStore(t)
	w := NewWriter(s, true)

	events := []normalize.Event{{
		Type:        normalize.EventLLMCall,
		Agent:       "claude_code",
		SessionID:   "sess-1",
		ProjectPath: "/home/user/repo",
		LLMCall: &normalize.LLMCallData{
			StartedAt:        time.Now(),
			DurationMs:       1200,
			Model:            "claude-sonnet-4",
			Provider:         "anthropic",
			InputTokens:      100000,
			OutputTokens:     50000,
			CacheReadTokens:  20000,
			CacheWriteTokens: 5000,
			StopReason:       "end_turn",
		},
	}}

	if err := w.Write(events); err != nil {
		t.Fatalf("Write: %v", err)
	}

	var count int
	s.DB().QueryRow("SELECT count(*) FROM llm_calls").Scan(&count)
	if count != 1 {
		t.Errorf("llm_calls count: got %d, want 1", count)
	}

	var sessionCost float64
	s.DB().QueryRow("SELECT total_cost_usd FROM sessions WHERE external_id = ?", "sess-1").Scan(&sessionCost)
	if sessionCost <= 0 {
		t.Errorf("session cost should be > 0, got %v", sessionCost)
	}
}

func TestWriter_WriteToolCall(t *testing.T) {
	s := writerTestStore(t)
	w := NewWriter(s, true)

	events := []normalize.Event{
		{
			Type:      normalize.EventLLMCall,
			Agent:     "claude_code",
			SessionID: "sess-1",
			LLMCall: &normalize.LLMCallData{
				StartedAt: time.Now(),
				Model:     "claude-sonnet-4",
				Provider:  "anthropic",
			},
		},
		{
			Type:      normalize.EventToolCall,
			Agent:     "claude_code",
			SessionID: "sess-1",
			ToolCall: &normalize.ToolCallData{
				StartedAt:  time.Now(),
				DurationMs: 200,
				ToolName:   "bash",
				Success:    true,
			},
		},
	}

	if err := w.Write(events); err != nil {
		t.Fatalf("Write: %v", err)
	}

	var count int
	s.DB().QueryRow("SELECT count(*) FROM tool_calls").Scan(&count)
	if count != 1 {
		t.Errorf("tool_calls count: got %d, want 1", count)
	}
}

func TestWriter_WriteLog_disabled(t *testing.T) {
	s := writerTestStore(t)
	w := NewWriter(s, false)

	events := []normalize.Event{{
		Type:      normalize.EventLog,
		Agent:     "claude_code",
		SessionID: "sess-1",
		Log: &normalize.LogData{
			Name:    "claude_code.tool_result",
			Payload: []byte("{}"),
		},
	}}

	if err := w.Write(events); err != nil {
		t.Fatalf("Write: %v", err)
	}

	var count int
	s.DB().QueryRow("SELECT count(*) FROM events").Scan(&count)
	if count != 0 {
		t.Errorf("events count with logs disabled: got %d, want 0", count)
	}
}

func TestWriter_usesExplicitLLMCost(t *testing.T) {
	s := writerTestStore(t)
	w := NewWriter(s, false)
	started := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)

	err := w.Write([]normalize.Event{{
		Type:        normalize.EventLLMCall,
		Timestamp:   started,
		Agent:       "opencode",
		SessionID:   "explicit-cost-session",
		ProjectPath: "/tmp/opencode-project",
		LLMCall: &normalize.LLMCallData{
			TraceID:      "trace-explicit",
			SpanID:       "span-explicit",
			StartedAt:    started,
			DurationMs:   900,
			Model:        "unknown-model",
			Provider:     "unknown-provider",
			InputTokens:  100,
			OutputTokens: 50,
			CostUSD:      0.0123,
		},
	}})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	var callCost float64
	if err := s.DB().QueryRow(`SELECT cost_usd FROM llm_calls WHERE span_id = 'span-explicit'`).Scan(&callCost); err != nil {
		t.Fatalf("query llm cost: %v", err)
	}
	if callCost < 0.012299 || callCost > 0.012301 {
		t.Fatalf("llm cost: got %v, want 0.0123", callCost)
	}

	var sessionCost float64
	if err := s.DB().QueryRow(`SELECT total_cost_usd FROM sessions WHERE external_id = 'explicit-cost-session'`).Scan(&sessionCost); err != nil {
		t.Fatalf("query session cost: %v", err)
	}
	if sessionCost < 0.012299 || sessionCost > 0.012301 {
		t.Fatalf("session cost: got %v, want 0.0123", sessionCost)
	}
}
