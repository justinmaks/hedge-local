package queries

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/justinmaks/hedge-local/internal/store"
)

type Service struct {
	store *store.Store
}

func NewService(s *store.Store) *Service {
	return &Service{store: s}
}

func (s *Service) Store() *store.Store {
	return s.store
}

type OverviewStats struct {
	TodayCost      float64
	TodayTokens    int
	TodaySessions  int
	WeekCost       float64
	CostDeltaPct   float64
	ByAgent        []AgentBreakdown
	RecentSessions []SessionRow
}

type AgentBreakdown struct {
	Agent  string
	Cost   float64
	Tokens int
}

type SessionRow struct {
	ExternalID  string
	Agent       string
	StartedAt   time.Time
	Cost        float64
	Tokens      int
	ToolCount   int
	ProjectPath string
}

type CostPoint struct {
	Timestamp time.Time
	Cost      float64
}

type CostBreakdown struct {
	Name     string
	Cost     float64
	Pct      float64
	Sessions int
	Tokens   int
}

func (s *Service) OverviewSummary(from, to time.Time) (OverviewStats, error) {
	db := s.store.DB()
	var stats OverviewStats

	err := db.QueryRow(
		`SELECT COALESCE(SUM(total_cost_usd), 0), COALESCE(SUM(total_input_tokens + total_output_tokens), 0), COUNT(*)
		 FROM sessions WHERE started_at BETWEEN ? AND ?`,
		from, to,
	).Scan(&stats.TodayCost, &stats.TodayTokens, &stats.TodaySessions)
	if err != nil {
		return stats, fmt.Errorf("overview summary: %w", err)
	}

	weekAgo := to.Add(-7 * 24 * time.Hour)
	twoWeeksAgo := to.Add(-14 * 24 * time.Hour)
	var prevWeekCost float64
	_ = db.QueryRow(
		`SELECT COALESCE(SUM(total_cost_usd), 0) FROM sessions WHERE started_at BETWEEN ? AND ?`,
		twoWeeksAgo, weekAgo,
	).Scan(&prevWeekCost)

	_ = db.QueryRow(
		`SELECT COALESCE(SUM(total_cost_usd), 0) FROM sessions WHERE started_at BETWEEN ? AND ?`,
		weekAgo, to,
	).Scan(&stats.WeekCost)

	if prevWeekCost > 0 {
		stats.CostDeltaPct = (stats.WeekCost - prevWeekCost) / prevWeekCost * 100
	}

	rows, err := db.Query(
		`SELECT agent, COALESCE(SUM(total_cost_usd), 0), COALESCE(SUM(total_input_tokens + total_output_tokens), 0)
		 FROM sessions WHERE started_at BETWEEN ? AND ? GROUP BY agent ORDER BY SUM(total_cost_usd) DESC`,
		from, to,
	)
	if err != nil {
		return stats, fmt.Errorf("overview by agent: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var ab AgentBreakdown
		if err := rows.Scan(&ab.Agent, &ab.Cost, &ab.Tokens); err != nil {
			return stats, err
		}
		stats.ByAgent = append(stats.ByAgent, ab)
	}
	if err := rows.Err(); err != nil {
		return stats, fmt.Errorf("overview by agent rows: %w", err)
	}

	sessions, err := s.RecentSessionsInRange("", 5, from, to)
	if err != nil {
		return stats, fmt.Errorf("overview recent sessions: %w", err)
	}
	stats.RecentSessions = sessions

	return stats, nil
}

func (s *Service) CostTrend(from, to time.Time, granularity string) ([]CostPoint, error) {
	db := s.store.DB()
	var periodExpr, parseFmt string
	if granularity == "hourly" {
		periodExpr = "substr(started_at, 1, 10) || ' ' || substr(started_at, 12, 2) || ':00'"
		parseFmt = "2006-01-02 15:00"
	} else {
		periodExpr = "substr(started_at, 1, 10)"
		parseFmt = "2006-01-02"
	}
	rows, err := db.Query(
		`SELECT `+periodExpr+` as period, COALESCE(SUM(total_cost_usd), 0)
		 FROM sessions WHERE started_at BETWEEN ? AND ?
		 GROUP BY period ORDER BY period`,
		from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("cost trend: %w", err)
	}
	defer rows.Close()
	var points []CostPoint
	for rows.Next() {
		var p CostPoint
		var periodStr string
		if err := rows.Scan(&periodStr, &p.Cost); err != nil {
			return nil, err
		}
		ts, err := time.ParseInLocation(parseFmt, periodStr, time.Local)
		if err != nil {
			return nil, fmt.Errorf("cost trend parse %q: %w", periodStr, err)
		}
		p.Timestamp = ts
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cost trend rows: %w", err)
	}
	return points, nil
}

func (s *Service) CostByDimension(from, to time.Time, dim string) ([]CostBreakdown, error) {
	db := s.store.DB()

	if dim == "model" {
		return s.costByModel(from, to)
	}

	var groupCol string
	switch dim {
	case "project":
		groupCol = "p.name"
	case "agent":
		fallthrough
	default:
		groupCol = "agent"
	}

	var totalCost float64
	_ = db.QueryRow(
		`SELECT COALESCE(SUM(total_cost_usd), 0) FROM sessions WHERE started_at BETWEEN ? AND ?`,
		from, to,
	).Scan(&totalCost)

	query := fmt.Sprintf(
		`SELECT %s as name, COALESCE(SUM(s.total_cost_usd), 0) as cost,
		 COUNT(DISTINCT s.id) as sessions,
		 COALESCE(SUM(s.total_input_tokens + s.total_output_tokens), 0) as tokens
		 FROM sessions s
		 LEFT JOIN projects p ON s.project_id = p.id
		 WHERE s.started_at BETWEEN ? AND ?
		 GROUP BY name ORDER BY cost DESC`,
		groupCol,
	)
	rows, err := db.Query(query, from, to)
	if err != nil {
		return nil, fmt.Errorf("cost by dimension: %w", err)
	}
	defer rows.Close()
	var result []CostBreakdown
	for rows.Next() {
		var b CostBreakdown
		if err := rows.Scan(&b.Name, &b.Cost, &b.Sessions, &b.Tokens); err != nil {
			return nil, err
		}
		if b.Name == "" {
			b.Name = "(unknown)"
		}
		if totalCost > 0 {
			b.Pct = b.Cost / totalCost * 100
		}
		result = append(result, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cost by dimension rows: %w", err)
	}
	return result, nil
}

func (s *Service) costByModel(from, to time.Time) ([]CostBreakdown, error) {
	db := s.store.DB()

	var totalCost float64
	_ = db.QueryRow(
		`SELECT COALESCE(SUM(cost_usd), 0) FROM llm_calls lc
		 JOIN sessions s ON lc.session_id = s.id
		 WHERE s.started_at BETWEEN ? AND ?`,
		from, to,
	).Scan(&totalCost)

	rows, err := db.Query(
		`SELECT lc.model as name, COALESCE(SUM(lc.cost_usd), 0) as cost,
		 COUNT(DISTINCT s.id) as sessions,
		 COALESCE(SUM(lc.input_tokens + lc.output_tokens), 0) as tokens
		 FROM llm_calls lc
		 JOIN sessions s ON lc.session_id = s.id
		 WHERE s.started_at BETWEEN ? AND ?
		 GROUP BY lc.model ORDER BY cost DESC`,
		from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("cost by model: %w", err)
	}
	defer rows.Close()
	var result []CostBreakdown
	for rows.Next() {
		var b CostBreakdown
		var name sql.NullString
		if err := rows.Scan(&name, &b.Cost, &b.Sessions, &b.Tokens); err != nil {
			return nil, err
		}
		b.Name = name.String
		if b.Name == "" {
			b.Name = "(unknown)"
		}
		if totalCost > 0 {
			b.Pct = b.Cost / totalCost * 100
		}
		result = append(result, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cost by model rows: %w", err)
	}
	return result, nil
}

func (s *Service) RecentSessions(projectPath string, limit int) ([]SessionRow, error) {
	return s.recentSessions(projectPath, limit, time.Time{}, time.Time{}, false)
}

func (s *Service) RecentSessionsInRange(projectPath string, limit int, from, to time.Time) ([]SessionRow, error) {
	return s.recentSessions(projectPath, limit, from, to, true)
}

func (s *Service) recentSessions(projectPath string, limit int, from, to time.Time, hasRange bool) ([]SessionRow, error) {
	db := s.store.DB()
	var rows *sql.Rows
	var err error
	if projectPath != "" {
		if hasRange {
			rows, err = db.Query(
				`SELECT s.external_id, s.agent, s.started_at, s.total_cost_usd,
				 s.total_input_tokens + s.total_output_tokens, s.tool_call_count, p.path
				 FROM sessions s LEFT JOIN projects p ON s.project_id = p.id
				 WHERE p.path = ? AND s.started_at BETWEEN ? AND ? ORDER BY s.started_at DESC LIMIT ?`,
				projectPath, from, to, limit,
			)
		} else {
			rows, err = db.Query(
				`SELECT s.external_id, s.agent, s.started_at, s.total_cost_usd,
				 s.total_input_tokens + s.total_output_tokens, s.tool_call_count, p.path
				 FROM sessions s LEFT JOIN projects p ON s.project_id = p.id
				 WHERE p.path = ? ORDER BY s.started_at DESC LIMIT ?`,
				projectPath, limit,
			)
		}
	} else {
		if hasRange {
			rows, err = db.Query(
				`SELECT s.external_id, s.agent, s.started_at, s.total_cost_usd,
				 s.total_input_tokens + s.total_output_tokens, s.tool_call_count, p.path
				 FROM sessions s LEFT JOIN projects p ON s.project_id = p.id
				 WHERE s.started_at BETWEEN ? AND ? ORDER BY s.started_at DESC LIMIT ?`,
				from, to, limit,
			)
		} else {
			rows, err = db.Query(
				`SELECT s.external_id, s.agent, s.started_at, s.total_cost_usd,
				 s.total_input_tokens + s.total_output_tokens, s.tool_call_count, p.path
				 FROM sessions s LEFT JOIN projects p ON s.project_id = p.id
				 ORDER BY s.started_at DESC LIMIT ?`,
				limit,
			)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("recent sessions: %w", err)
	}
	defer rows.Close()
	var result []SessionRow
	for rows.Next() {
		var r SessionRow
		var projectPath sql.NullString
		if err := rows.Scan(&r.ExternalID, &r.Agent, &r.StartedAt, &r.Cost, &r.Tokens, &r.ToolCount, &projectPath); err != nil {
			return nil, err
		}
		r.ProjectPath = projectPath.String
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("recent sessions rows: %w", err)
	}
	return result, nil
}

type ToolStats struct {
	ToolName     string
	Calls        int
	SuccessRate  float64
	AvgLatencyMs float64
	TotalCost    float64
	TopError     string
}

func (s *Service) ToolSummary(from, to time.Time) ([]ToolStats, error) {
	db := s.store.DB()
	rows, err := db.Query(
		`SELECT tool_name, COUNT(*) as calls,
		 COALESCE(AVG(CASE WHEN success = 1 THEN 100.0 ELSE 0.0 END), 0) as success_rate,
		 COALESCE(AVG(duration_ms), 0) as avg_latency,
		 0 as cost,
		 COALESCE((SELECT error_message FROM tool_calls tc2 WHERE tc2.tool_name = tool_calls.tool_name AND tc2.started_at BETWEEN ? AND ? AND error_message != '' ORDER BY tc2.started_at DESC LIMIT 1), '') as top_error
		 FROM tool_calls WHERE started_at BETWEEN ? AND ?
		 GROUP BY tool_name ORDER BY calls DESC`,
		from, to, from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("tool summary: %w", err)
	}
	defer rows.Close()
	var result []ToolStats
	for rows.Next() {
		var ts ToolStats
		if err := rows.Scan(&ts.ToolName, &ts.Calls, &ts.SuccessRate, &ts.AvgLatencyMs, &ts.TotalCost, &ts.TopError); err != nil {
			return nil, err
		}
		result = append(result, ts)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tool summary rows: %w", err)
	}
	return result, nil
}

type ModelStats struct {
	Agent            string
	Model            string
	Provider         string
	Calls            int
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
	CachePct         float64
	Cost             float64
	AvgTTFTMs        float64
}

func (s *Service) ModelSummary(from, to time.Time) ([]ModelStats, error) {
	db := s.store.DB()
	rows, err := db.Query(
		`SELECT agent, model, provider, COUNT(*) as calls,
		 COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0),
		 COALESCE(SUM(cache_read_tokens), 0), COALESCE(SUM(cache_write_tokens), 0),
		 COALESCE(SUM(cost_usd), 0), COALESCE(AVG(ttft_ms), 0)
		 FROM llm_calls WHERE started_at BETWEEN ? AND ?
		 GROUP BY agent, model, provider ORDER BY COALESCE(SUM(cost_usd), 0) DESC`,
		from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("model summary: %w", err)
	}
	defer rows.Close()
	var result []ModelStats
	for rows.Next() {
		var ms ModelStats
		if err := rows.Scan(&ms.Agent, &ms.Model, &ms.Provider, &ms.Calls, &ms.InputTokens, &ms.OutputTokens,
			&ms.CacheReadTokens, &ms.CacheWriteTokens, &ms.Cost, &ms.AvgTTFTMs); err != nil {
			return nil, err
		}
		totalTokens := ms.InputTokens + ms.OutputTokens + ms.CacheReadTokens + ms.CacheWriteTokens
		if totalTokens > 0 {
			ms.CachePct = float64(ms.CacheReadTokens+ms.CacheWriteTokens) / float64(totalTokens) * 100
		}
		result = append(result, ms)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("model summary rows: %w", err)
	}
	return result, nil
}

type ProjectStats struct {
	Path       string
	Name       string
	Sessions   int
	Cost       float64
	Tokens     int
	LastActive time.Time
	DailyTrend []float64
}

func (s *Service) ProjectSummary(from, to time.Time) ([]ProjectStats, error) {
	db := s.store.DB()
	rows, err := db.Query(
		`SELECT p.path, p.name, COUNT(s.id) as sessions,
		 COALESCE(SUM(s.total_cost_usd), 0), COALESCE(SUM(s.total_input_tokens + s.total_output_tokens), 0),
		 MAX(s.started_at)
		 FROM projects p LEFT JOIN sessions s ON s.project_id = p.id
		 WHERE s.started_at IS NULL OR s.started_at BETWEEN ? AND ?
		 GROUP BY p.id ORDER BY COALESCE(SUM(s.total_cost_usd), 0) DESC`,
		from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("project summary: %w", err)
	}
	defer rows.Close()
	var result []ProjectStats
	for rows.Next() {
		var ps ProjectStats
		var lastActive sql.NullString
		if err := rows.Scan(&ps.Path, &ps.Name, &ps.Sessions, &ps.Cost, &ps.Tokens, &lastActive); err != nil {
			return nil, err
		}
		if lastActive.Valid {
			if t, ok := parseStoredTime(lastActive.String); ok {
				ps.LastActive = t
			}
		}
		result = append(result, ps)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("project summary rows: %w", err)
	}
	return result, nil
}

type SpanRow struct {
	Timestamp time.Time
	Agent     string
	SpanType  string
	Detail    string
	Tokens    int
	Cost      float64
}

func (s *Service) RecentSpans(limit int, filter string) ([]SpanRow, error) {
	return s.RecentSpansInRange(time.Time{}, time.Time{}, limit, filter, "all")
}

func (s *Service) RecentSpansInRange(from, to time.Time, limit int, filter, agent string) ([]SpanRow, error) {
	db := s.store.DB()
	hasRange := !from.IsZero() && !to.IsZero()
	hasAgent := agent != "" && agent != "all"
	query := `SELECT ts, agent, span_type, detail, tokens, cost FROM (
		SELECT started_at as ts, agent, 'llm' as span_type, model as detail, input_tokens + output_tokens as tokens, cost_usd as cost
		FROM llm_calls` + spanFilterClause(hasRange, hasAgent, "started_at", "agent") + `
		UNION ALL
		SELECT started_at as ts, agent, 'tool' as span_type, tool_name as detail, 0 as tokens, 0 as cost
		FROM tool_calls` + spanFilterClause(hasRange, hasAgent, "started_at", "agent") + `
		UNION ALL
		SELECT timestamp as ts, agent, 'event' as span_type, event_name as detail, 0 as tokens, 0 as cost
		FROM events` + spanFilterClause(hasRange, hasAgent, "timestamp", "agent") + `
		) ORDER BY ts DESC LIMIT ?`
	switch filter {
	case "llm":
		query = `SELECT started_at, agent, 'llm' as type, model, input_tokens + output_tokens, cost_usd FROM llm_calls` + spanFilterClause(hasRange, hasAgent, "started_at", "agent") + ` ORDER BY started_at DESC LIMIT ?`
	case "tool":
		query = `SELECT started_at, agent, 'tool' as type, tool_name, 0, 0 FROM tool_calls` + spanFilterClause(hasRange, hasAgent, "started_at", "agent") + ` ORDER BY started_at DESC LIMIT ?`
	case "event":
		query = `SELECT timestamp, agent, 'event' as type, event_name, 0, 0 FROM events` + spanFilterClause(hasRange, hasAgent, "timestamp", "agent") + ` ORDER BY timestamp DESC LIMIT ?`
	}
	args := spanQueryArgs(hasRange, hasAgent, from, to, agent, filter, limit)
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("recent spans: %w", err)
	}
	defer rows.Close()
	var result []SpanRow
	for rows.Next() {
		var r SpanRow
		if err := rows.Scan(&r.Timestamp, &r.Agent, &r.SpanType, &r.Detail, &r.Tokens, &r.Cost); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("recent spans rows: %w", err)
	}
	return result, nil
}

// parseStoredTime parses a timestamp as stored by the modernc SQLite driver.
// The driver persists time.Time via its String() representation, e.g.
// "2006-01-02 15:04:05.999999999 -0700 MST" optionally followed by a monotonic
// clock reading (" m=..."), which standard layouts cannot parse. This is only
// needed when reading aggregate results (e.g. MAX(started_at)) as raw strings;
// direct column scans are converted by the driver. RFC3339 is also accepted as
// a fallback.
func parseStoredTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	if i := strings.Index(s, " m="); i != -1 {
		s = s[:i]
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05 -0700 MST",
		time.RFC3339Nano,
		time.RFC3339,
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func spanFilterClause(hasRange, hasAgent bool, timeColumn, agentColumn string) string {
	if !hasRange && !hasAgent {
		return ""
	}
	clauses := make([]string, 0, 2)
	if hasRange {
		clauses = append(clauses, timeColumn+" BETWEEN ? AND ?")
	}
	if hasAgent {
		clauses = append(clauses, agentColumn+" = ?")
	}
	return " WHERE " + strings.Join(clauses, " AND ")
}

func spanQueryArgs(hasRange, hasAgent bool, from, to time.Time, agent, filter string, limit int) []any {
	appendArgs := func(dst []any) []any {
		if hasRange {
			dst = append(dst, from, to)
		}
		if hasAgent {
			dst = append(dst, agent)
		}
		return dst
	}
	if filter != "all" {
		args := appendArgs(nil)
		return append(args, limit)
	}
	args := appendArgs(nil)
	args = appendArgs(args)
	args = appendArgs(args)
	return append(args, limit)
}

type SessionExportRow struct {
	ExternalID    string
	Agent         string
	ProjectPath   string
	StartedAt     time.Time
	EndedAt       *time.Time
	TotalCostUSD  float64
	InputTokens   int
	OutputTokens  int
	ToolCallCount int
	MessageCount  int
}

func (s *Service) ExportSessions(from, to time.Time) ([]SessionExportRow, error) {
	db := s.store.DB()
	rows, err := db.Query(
		`SELECT s.external_id, s.agent, p.path, s.started_at, s.ended_at,
		 s.total_cost_usd, s.total_input_tokens, s.total_output_tokens, s.tool_call_count, s.message_count
		 FROM sessions s LEFT JOIN projects p ON s.project_id = p.id
		 WHERE s.started_at BETWEEN ? AND ? ORDER BY s.started_at`,
		from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("export sessions: %w", err)
	}
	defer rows.Close()
	var result []SessionExportRow
	for rows.Next() {
		var r SessionExportRow
		var endedAt sql.NullTime
		var projectPath sql.NullString
		if err := rows.Scan(&r.ExternalID, &r.Agent, &projectPath, &r.StartedAt, &endedAt,
			&r.TotalCostUSD, &r.InputTokens, &r.OutputTokens, &r.ToolCallCount, &r.MessageCount); err != nil {
			return nil, err
		}
		if projectPath.Valid {
			r.ProjectPath = projectPath.String
		}
		if endedAt.Valid {
			r.EndedAt = &endedAt.Time
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("export sessions rows: %w", err)
	}
	return result, nil
}

type LLMCallExportRow struct {
	StartedAt        time.Time
	Agent            string
	Model            string
	Provider         string
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
	CostUSD          float64
	DurationMs       int
	StopReason       string
}

func (s *Service) ExportLLMCalls(from, to time.Time) ([]LLMCallExportRow, error) {
	db := s.store.DB()
	rows, err := db.Query(
		`SELECT started_at, agent, model, provider, input_tokens, output_tokens,
		 cache_read_tokens, cache_write_tokens, cost_usd, duration_ms, stop_reason
		 FROM llm_calls WHERE started_at BETWEEN ? AND ? ORDER BY started_at`,
		from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("export llm calls: %w", err)
	}
	defer rows.Close()
	var result []LLMCallExportRow
	for rows.Next() {
		var r LLMCallExportRow
		var model, provider, stopReason sql.NullString
		if err := rows.Scan(&r.StartedAt, &r.Agent, &model, &provider, &r.InputTokens, &r.OutputTokens,
			&r.CacheReadTokens, &r.CacheWriteTokens, &r.CostUSD, &r.DurationMs, &stopReason); err != nil {
			return nil, err
		}
		r.Model = model.String
		r.Provider = provider.String
		r.StopReason = stopReason.String
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("export llm calls rows: %w", err)
	}
	return result, nil
}

type ToolCallExportRow struct {
	StartedAt    time.Time
	Agent        string
	ToolName     string
	Success      bool
	DurationMs   int
	ErrorMessage string
}

func (s *Service) ExportToolCalls(from, to time.Time) ([]ToolCallExportRow, error) {
	db := s.store.DB()
	rows, err := db.Query(
		`SELECT started_at, agent, tool_name, success, duration_ms, error_message
		 FROM tool_calls WHERE started_at BETWEEN ? AND ? ORDER BY started_at`,
		from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("export tool calls: %w", err)
	}
	defer rows.Close()
	var result []ToolCallExportRow
	for rows.Next() {
		var r ToolCallExportRow
		var errMsg sql.NullString
		if err := rows.Scan(&r.StartedAt, &r.Agent, &r.ToolName, &r.Success, &r.DurationMs, &errMsg); err != nil {
			return nil, err
		}
		if errMsg.Valid {
			r.ErrorMessage = errMsg.String
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("export tool calls rows: %w", err)
	}
	return result, nil
}

type EventExportRow struct {
	Timestamp time.Time
	Agent     string
	EventName string
	Payload   string
}

func (s *Service) ExportEvents(from, to time.Time) ([]EventExportRow, error) {
	db := s.store.DB()
	rows, err := db.Query(
		`SELECT timestamp, agent, event_name, payload
		 FROM events WHERE timestamp BETWEEN ? AND ? ORDER BY timestamp`,
		from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("export events: %w", err)
	}
	defer rows.Close()
	var result []EventExportRow
	for rows.Next() {
		var r EventExportRow
		var payload sql.NullString
		if err := rows.Scan(&r.Timestamp, &r.Agent, &r.EventName, &payload); err != nil {
			return nil, err
		}
		if payload.Valid {
			r.Payload = payload.String
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("export events rows: %w", err)
	}
	return result, nil
}
