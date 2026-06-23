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
