package queries

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/justinmaks/hedge-local/internal/collect"
	"github.com/justinmaks/hedge-local/internal/normalize"
	"github.com/justinmaks/hedge-local/internal/store"
)

func seedTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := s.SeedBundledPricing(); err != nil {
		t.Fatalf("SeedBundledPricing: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	w := collect.NewWriter(s, false)
	baseTime := time.Now()
	pid, _ := s.ProjectUpsert("/tmp/test-project", "test-project")
	_, _ = s.SessionUpsert("sess-overview", "claude_code", pid, baseTime, "")

	w.Write([]normalize.Event{{
		Type:      normalize.EventLLMCall,
		Timestamp: baseTime,
		Agent:     "claude_code",
		SessionID: "sess-overview",
		LLMCall: &normalize.LLMCallData{
			StartedAt:    baseTime,
			Model:        "claude-sonnet-4",
			Provider:     "anthropic",
			InputTokens:  1000,
			OutputTokens: 500,
		},
	}})
	return s
}

func TestOverviewSummary(t *testing.T) {
	s := seedTestStore(t)
	svc := NewService(s)
	from := time.Now().Add(-24 * time.Hour)
	to := time.Now().Add(time.Hour)
	stats, err := svc.OverviewSummary(from, to)
	if err != nil {
		t.Fatalf("OverviewSummary: %v", err)
	}
	if stats.TodayCost <= 0 {
		t.Errorf("TodayCost: got %v, want > 0", stats.TodayCost)
	}
	if stats.TodaySessions < 1 {
		t.Errorf("TodaySessions: got %v, want >= 1", stats.TodaySessions)
	}
	if len(stats.ByAgent) == 0 {
		t.Errorf("ByAgent: expected at least 1 agent")
	}
}

func TestCostTrend(t *testing.T) {
	s := seedTestStore(t)
	svc := NewService(s)
	from := time.Now().Add(-7 * 24 * time.Hour)
	to := time.Now().Add(time.Hour)
	points, err := svc.CostTrend(from, to, "daily")
	if err != nil {
		t.Fatalf("CostTrend: %v", err)
	}
	if len(points) == 0 {
		t.Errorf("CostTrend: expected at least 1 point")
	}
}

func TestCostByDimension(t *testing.T) {
	s := seedTestStore(t)
	svc := NewService(s)
	from := time.Now().Add(-24 * time.Hour)
	to := time.Now().Add(time.Hour)
	breakdown, err := svc.CostByDimension(from, to, "agent")
	if err != nil {
		t.Fatalf("CostByDimension: %v", err)
	}
	if len(breakdown) == 0 {
		t.Errorf("CostByDimension: expected at least 1 row")
	}
	if breakdown[0].Name != "claude_code" {
		t.Errorf("CostByDimension[0].Name: got %q, want claude_code", breakdown[0].Name)
	}
}

func TestCostByDimension_AgentSpansProjects(t *testing.T) {
	s := seedTestStore(t)
	// Same agent active in a second project: the agent breakdown must
	// still collapse to one row, not split per project.
	baseTime := time.Now()
	pid2, _ := s.ProjectUpsert("/tmp/other-project", "other-project")
	_, _ = s.SessionUpsert("sess-other", "claude_code", pid2, baseTime, "")

	svc := NewService(s)
	from := time.Now().Add(-24 * time.Hour)
	to := time.Now().Add(time.Hour)
	breakdown, err := svc.CostByDimension(from, to, "agent")
	if err != nil {
		t.Fatalf("CostByDimension: %v", err)
	}
	if len(breakdown) != 1 {
		t.Fatalf("agent breakdown should have 1 row for 1 agent, got %d: %+v", len(breakdown), breakdown)
	}
	if breakdown[0].Sessions != 2 {
		t.Errorf("Sessions: got %d, want 2", breakdown[0].Sessions)
	}
}

func TestCostByDimension_Model(t *testing.T) {
	s := seedTestStore(t)
	svc := NewService(s)
	from := time.Now().Add(-24 * time.Hour)
	to := time.Now().Add(time.Hour)
	breakdown, err := svc.CostByDimension(from, to, "model")
	if err != nil {
		t.Fatalf("CostByDimension model: %v", err)
	}
	if len(breakdown) == 0 {
		t.Fatalf("CostByDimension model: expected at least 1 row")
	}
	if breakdown[0].Name != "claude-sonnet-4" {
		t.Errorf("CostByDimension model[0].Name: got %q, want claude-sonnet-4", breakdown[0].Name)
	}
	if breakdown[0].Cost <= 0 {
		t.Errorf("CostByDimension model[0].Cost: got %v, want > 0", breakdown[0].Cost)
	}
	if breakdown[0].Tokens != 1500 {
		t.Errorf("CostByDimension model[0].Tokens: got %v, want 1500", breakdown[0].Tokens)
	}
}

func TestCostByDimension_Project(t *testing.T) {
	s := seedTestStore(t)
	svc := NewService(s)
	from := time.Now().Add(-24 * time.Hour)
	to := time.Now().Add(time.Hour)
	breakdown, err := svc.CostByDimension(from, to, "project")
	if err != nil {
		t.Fatalf("CostByDimension project: %v", err)
	}
	if len(breakdown) == 0 {
		t.Fatalf("CostByDimension project: expected at least 1 row")
	}
	if breakdown[0].Name != "test-project" {
		t.Errorf("CostByDimension project[0].Name: got %q, want test-project", breakdown[0].Name)
	}
	if breakdown[0].Cost <= 0 {
		t.Errorf("CostByDimension project[0].Cost: got %v, want > 0", breakdown[0].Cost)
	}
}

func TestSpanCountInRange(t *testing.T) {
	s := seedTestStore(t)
	// Seed store starts with one llm_call; add a tool call and an event.
	baseTime := time.Now()
	if _, err := s.ToolCallInsert(store.ToolCallParams{
		SessionID: 1, SpanID: "span-count-tool", StartedAt: baseTime,
		Agent: "claude_code", ToolName: "bash", Success: true,
	}); err != nil {
		t.Fatalf("tool insert: %v", err)
	}
	if _, err := s.EventInsert(store.EventParams{
		SessionID: 1, Timestamp: baseTime, Agent: "claude_code",
		EventName: "claude_code.test", Payload: "{}",
	}); err != nil {
		t.Fatalf("event insert: %v", err)
	}

	svc := NewService(s)
	from := baseTime.Add(-time.Hour)
	to := baseTime.Add(time.Hour)
	count, err := svc.SpanCountInRange(from, to)
	if err != nil {
		t.Fatalf("SpanCountInRange: %v", err)
	}
	if count != 3 {
		t.Errorf("span count: got %d, want 3 (1 llm + 1 tool + 1 event)", count)
	}

	// A window with no spans counts zero.
	count, err = svc.SpanCountInRange(baseTime.Add(24*time.Hour), baseTime.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("SpanCountInRange empty window: %v", err)
	}
	if count != 0 {
		t.Errorf("empty window count: got %d, want 0", count)
	}
}

func TestModelSummary_nullModelAndProvider(t *testing.T) {
	s := seedTestStore(t)
	// Rows written by other tools may have NULL model/provider; the summary
	// must not error on them.
	if _, err := s.DB().Exec(
		`INSERT INTO llm_calls (session_id, started_at, agent, model, provider, cost_usd)
		 VALUES (1, ?, 'claude_code', NULL, NULL, 0.01)`,
		time.Now(),
	); err != nil {
		t.Fatalf("insert null-provider row: %v", err)
	}

	svc := NewService(s)
	from := time.Now().Add(-24 * time.Hour)
	to := time.Now().Add(time.Hour)
	stats, err := svc.ModelSummary(from, to)
	if err != nil {
		t.Fatalf("ModelSummary with NULL model/provider: %v", err)
	}
	if len(stats) < 2 {
		t.Errorf("expected seeded row plus NULL row, got %d", len(stats))
	}
}

func TestCostTrend_Hourly(t *testing.T) {
	s := seedTestStore(t)
	svc := NewService(s)
	from := time.Now().Add(-24 * time.Hour)
	to := time.Now().Add(time.Hour)
	points, err := svc.CostTrend(from, to, "hourly")
	if err != nil {
		t.Fatalf("CostTrend hourly: %v", err)
	}
	if len(points) == 0 {
		t.Fatalf("CostTrend hourly: expected at least 1 point")
	}
	if points[0].Timestamp.IsZero() {
		t.Errorf("CostTrend hourly[0].Timestamp: got zero, want valid time")
	}
	if points[0].Cost <= 0 {
		t.Errorf("CostTrend hourly[0].Cost: got %v, want > 0", points[0].Cost)
	}
}

func TestToolSummary(t *testing.T) {
	s := seedTestStore(t)
	projectID, err := s.ProjectUpsert("/tmp/tool-summary-project", "tool-summary-project")
	if err != nil {
		t.Fatalf("ProjectUpsert: %v", err)
	}
	oldTime := time.Now().Add(-48 * time.Hour)
	newTime := time.Now().Add(-30 * time.Minute)
	oldSession, err := s.SessionUpsert("sess-old-tool-summary", "claude_code", projectID, oldTime, "")
	if err != nil {
		t.Fatalf("SessionUpsert old: %v", err)
	}
	newSession, err := s.SessionUpsert("sess-new-tool-summary", "claude_code", projectID, newTime, "")
	if err != nil {
		t.Fatalf("SessionUpsert new: %v", err)
	}
	if _, err := s.ToolCallInsert(store.ToolCallParams{SessionID: oldSession, StartedAt: oldTime, Agent: "claude_code", ToolName: "bash", Success: false, ErrorMessage: "old error", DurationMs: 100}); err != nil {
		t.Fatalf("ToolCallInsert old: %v", err)
	}
	if _, err := s.ToolCallInsert(store.ToolCallParams{SessionID: newSession, StartedAt: newTime, Agent: "claude_code", ToolName: "bash", Success: false, ErrorMessage: "new error", DurationMs: 200}); err != nil {
		t.Fatalf("ToolCallInsert new: %v", err)
	}

	svc := NewService(s)
	from := time.Now().Add(-24 * time.Hour)
	to := time.Now().Add(time.Hour)
	tools, err := svc.ToolSummary(from, to)
	if err != nil {
		t.Fatalf("ToolSummary: %v", err)
	}
	if len(tools) == 0 {
		t.Fatal("expected at least one tool row")
	}
	for _, tool := range tools {
		if tool.ToolName == "" {
			t.Errorf("ToolName should not be empty")
		}
	}
	if tools[0].TopError != "new error" {
		t.Fatalf("TopError: got %q, want %q", tools[0].TopError, "new error")
	}
}

func TestModelSummary(t *testing.T) {
	s := seedTestStore(t)
	w := collect.NewWriter(s, false)
	ts := time.Now().Add(-30 * time.Minute)
	pid, err := s.ProjectUpsert("/tmp/opencode-project", "opencode-project")
	if err != nil {
		t.Fatalf("ProjectUpsert: %v", err)
	}
	if _, err := s.SessionUpsert("sess-opencode", "opencode", pid, ts, ""); err != nil {
		t.Fatalf("SessionUpsert: %v", err)
	}
	w.Write([]normalize.Event{{
		Type:      normalize.EventLLMCall,
		Timestamp: ts,
		Agent:     "opencode",
		SessionID: "sess-opencode",
		LLMCall: &normalize.LLMCallData{
			StartedAt:    ts,
			Model:        "claude-sonnet-4",
			Provider:     "anthropic",
			InputTokens:  250,
			OutputTokens: 125,
		},
	}})

	svc := NewService(s)
	from := time.Now().Add(-24 * time.Hour)
	to := time.Now().Add(time.Hour)
	models, err := svc.ModelSummary(from, to)
	if err != nil {
		t.Fatalf("ModelSummary: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 model rows split by agent, got %d", len(models))
	}
	agents := map[string]bool{}
	for _, model := range models {
		if model.Model != "claude-sonnet-4" {
			t.Errorf("model: got %q, want claude-sonnet-4", model.Model)
		}
		agents[model.Agent] = true
	}
	if !agents["claude_code"] || !agents["opencode"] {
		t.Fatalf("expected agents claude_code and opencode, got %#v", agents)
	}
}

func TestParseStoredTime(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		valid bool
	}{
		{"modernc String() with monotonic", "2026-06-22 15:34:09.839719267 -0400 EDT m=-17999.963875754", true},
		{"modernc String() no monotonic", "2026-06-22 15:16:00 -0400 EDT", true},
		{"rfc3339nano", "2026-06-22T15:34:09.839719267-04:00", true},
		{"rfc3339", "2026-06-22T15:34:09-04:00", true},
		{"empty", "", false},
		{"garbage", "not-a-time", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tm, ok := parseStoredTime(c.in)
			if ok != c.valid {
				t.Fatalf("parseStoredTime(%q) ok=%v, want %v", c.in, ok, c.valid)
			}
			if c.valid && tm.IsZero() {
				t.Errorf("parseStoredTime(%q) returned zero time", c.in)
			}
		})
	}
}

func TestProjectSummary(t *testing.T) {
	s := seedTestStore(t)
	svc := NewService(s)
	from := time.Now().Add(-24 * time.Hour)
	to := time.Now().Add(time.Hour)
	projects, err := svc.ProjectSummary(from, to)
	if err != nil {
		t.Fatalf("ProjectSummary: %v", err)
	}
	if len(projects) == 0 {
		t.Fatalf("expected at least 1 project")
	}
	if projects[0].Name != "test-project" {
		t.Errorf("project name: got %q, want test-project", projects[0].Name)
	}
	if projects[0].LastActive.IsZero() {
		t.Errorf("LastActive should be populated from the session's started_at, got zero")
	}
}

func TestRecentSpans(t *testing.T) {
	s := seedTestStore(t)
	w := collect.NewWriter(s, false)
	ts := time.Now().Add(-15 * time.Minute)
	pid, err := s.ProjectUpsert("/tmp/opencode-live-project", "opencode-live-project")
	if err != nil {
		t.Fatalf("ProjectUpsert: %v", err)
	}
	if _, err := s.SessionUpsert("sess-opencode-live", "opencode", pid, ts, ""); err != nil {
		t.Fatalf("SessionUpsert: %v", err)
	}
	if err := w.Write([]normalize.Event{{
		Type:      normalize.EventLLMCall,
		Timestamp: ts,
		Agent:     "opencode",
		SessionID: "sess-opencode-live",
		LLMCall: &normalize.LLMCallData{
			StartedAt:    ts,
			Model:        "gpt-4.1",
			Provider:     "openai",
			InputTokens:  10,
			OutputTokens: 5,
		},
	}}); err != nil {
		t.Fatalf("Write opencode span: %v", err)
	}

	svc := NewService(s)
	spans, err := svc.RecentSpans(10, "all")
	if err != nil {
		t.Fatalf("RecentSpans: %v", err)
	}
	if len(spans) == 0 {
		t.Fatalf("expected at least 1 span")
	}
	claudeOnly, err := svc.RecentSpansInRange(time.Time{}, time.Time{}, 10, "all", "claude_code")
	if err != nil {
		t.Fatalf("RecentSpansInRange claude_code: %v", err)
	}
	for _, span := range claudeOnly {
		if span.Agent != "claude_code" {
			t.Fatalf("agent filter leaked %q row into claude_code filter", span.Agent)
		}
	}
}

func TestExportSessions(t *testing.T) {
	s := seedTestStore(t)
	svc := NewService(s)
	from := time.Now().Add(-24 * time.Hour)
	to := time.Now().Add(time.Hour)
	rows, err := svc.ExportSessions(from, to)
	if err != nil {
		t.Fatalf("ExportSessions: %v", err)
	}
	if len(rows) == 0 {
		t.Fatalf("expected at least 1 row")
	}
}

func TestLiveWindow_buckets(t *testing.T) {
	s := seedTestStore(t)
	now := time.Now()

	// Two llm calls and two tool calls at known offsets inside a
	// 10-bucket x 2s window (20s total).
	insertLLM := func(span string, ago time.Duration, outTokens int) {
		if _, err := s.LLMCallInsert(store.LLMCallParams{
			SessionID: 1, SpanID: span, StartedAt: now.Add(-ago),
			Agent: "claude_code", Model: "m", OutputTokens: outTokens,
		}); err != nil {
			t.Fatalf("llm insert: %v", err)
		}
	}
	insertTool := func(span string, ago time.Duration, ok bool) {
		if _, err := s.ToolCallInsert(store.ToolCallParams{
			SessionID: 1, SpanID: span, StartedAt: now.Add(-ago),
			Agent: "claude_code", ToolName: "bash", Success: ok,
		}); err != nil {
			t.Fatalf("tool insert: %v", err)
		}
	}
	insertLLM("lw-1", 19*time.Second, 100) // oldest bucket
	insertLLM("lw-2", 1*time.Second, 900)  // newest bucket (joins the seed row)
	insertTool("lw-t1", 1*time.Second, true)
	insertTool("lw-t2", 1500*time.Millisecond, false)

	svc := NewService(s)
	buckets, err := svc.LiveWindow(now, 10, 2*time.Second)
	if err != nil {
		t.Fatalf("LiveWindow: %v", err)
	}
	if len(buckets) != 10 {
		t.Fatalf("bucket count: got %d, want 10", len(buckets))
	}
	if buckets[0].OutputTokens != 100 {
		t.Errorf("oldest bucket tokens: got %d, want 100", buckets[0].OutputTokens)
	}
	last := buckets[9]
	// 900 from lw-2 plus 500 from seedTestStore's baseline call at "now".
	if last.OutputTokens != 1400 {
		t.Errorf("newest bucket tokens: got %d, want 1400", last.OutputTokens)
	}
	if last.ToolCalls != 2 || last.ToolErrors != 1 {
		t.Errorf("newest bucket tools: got calls=%d errors=%d, want 2/1", last.ToolCalls, last.ToolErrors)
	}
}

func TestLiveStats_burnAndRhythm(t *testing.T) {
	s := seedTestStore(t)
	now := time.Now()

	if _, err := s.LLMCallInsert(store.LLMCallParams{
		SessionID: 1, SpanID: "ls-recent", StartedAt: now.Add(time.Second),
		Agent: "claude_code", Model: "claude-sonnet-5", CostUSD: 0.50,
		InputTokens: 1000, CacheReadTokens: 3000, OutputTokens: 100,
	}); err != nil {
		t.Fatalf("llm insert: %v", err)
	}
	if _, err := s.ToolCallInsert(store.ToolCallParams{
		SessionID: 1, SpanID: "ls-tool", StartedAt: now.Add(-time.Minute),
		Agent: "claude_code", ToolName: "Edit", DurationMs: 1200, Success: true,
	}); err != nil {
		t.Fatalf("tool insert: %v", err)
	}

	svc := NewService(s)
	// Query from a clock slightly ahead of the newest row so it falls in
	// the newest burn window deterministically.
	st, err := svc.LiveStats(now.Add(2 * time.Second))
	if err != nil {
		t.Fatalf("LiveStats: %v", err)
	}
	// $0.50 in the newest 10-minute window scales to $3/hr (seed store
	// adds a small cost too, so allow a little slack above).
	if st.BurnPerHour < 3.0 || st.BurnPerHour > 3.5 {
		t.Errorf("burn: got %v, want ~3.0/hr", st.BurnPerHour)
	}
	if len(st.BurnHistory) != 5 {
		t.Fatalf("burn history: got %d windows, want 5", len(st.BurnHistory))
	}
	if st.LastModel != "claude-sonnet-5" {
		t.Errorf("last model: got %q", st.LastModel)
	}
	if st.LastTool != "Edit" || !st.LastToolOK || st.LastToolMs != 1200 {
		t.Errorf("last tool: got %q ok=%v ms=%d", st.LastTool, st.LastToolOK, st.LastToolMs)
	}
	if st.LastSpanAt.IsZero() {
		t.Error("LastSpanAt should be set")
	}
	if time.Since(st.LastSpanAt) > 5*time.Minute {
		t.Errorf("LastSpanAt too old: %v", st.LastSpanAt)
	}
	if st.SessionStart.IsZero() {
		t.Error("SessionStart should be set")
	}
	// 3000 cache reads vs 1000 (mine) + 1000 (seed) input = 60%.
	if st.CachePct < 55 || st.CachePct > 65 {
		t.Errorf("cache pct: got %v, want ~60", st.CachePct)
	}
	if st.TodayCost <= 0 {
		t.Errorf("today cost should be positive, got %v", st.TodayCost)
	}
}
