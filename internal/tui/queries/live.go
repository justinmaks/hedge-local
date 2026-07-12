package queries

import (
	"database/sql"
	"fmt"
	"time"
)

// LiveBucket aggregates activity within one waveform time slot.
type LiveBucket struct {
	OutputTokens int
	ToolCalls    int
	ToolErrors   int
}

// LiveWindow buckets the last (buckets * bucketDur) of activity ending at
// now, oldest first. Feeds the Pulse waveform and tool lane; rebuilt from
// the DB each refresh so it survives restarts and needs no in-view state.
func (s *Service) LiveWindow(now time.Time, buckets int, bucketDur time.Duration) ([]LiveBucket, error) {
	if buckets <= 0 {
		return nil, nil
	}
	result := make([]LiveBucket, buckets)
	from := now.Add(-time.Duration(buckets) * bucketDur)
	db := s.store.DB()

	slot := func(t time.Time) int {
		return int(t.Sub(from) / bucketDur)
	}

	rows, err := db.Query(
		`SELECT started_at, COALESCE(output_tokens, 0) FROM llm_calls WHERE started_at >= ?`,
		ts(from),
	)
	if err != nil {
		return nil, fmt.Errorf("live window llm: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var startedAt time.Time
		var outTokens int
		if err := rows.Scan(&startedAt, &outTokens); err != nil {
			return nil, err
		}
		if i := slot(startedAt); i >= 0 && i < buckets {
			result[i].OutputTokens += outTokens
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("live window llm rows: %w", err)
	}

	toolRows, err := db.Query(
		`SELECT started_at, success FROM tool_calls WHERE started_at >= ?`,
		ts(from),
	)
	if err != nil {
		return nil, fmt.Errorf("live window tools: %w", err)
	}
	defer toolRows.Close()
	for toolRows.Next() {
		var startedAt time.Time
		var success bool
		if err := toolRows.Scan(&startedAt, &success); err != nil {
			return nil, err
		}
		if i := slot(startedAt); i >= 0 && i < buckets {
			result[i].ToolCalls++
			if !success {
				result[i].ToolErrors++
			}
		}
	}
	if err := toolRows.Err(); err != nil {
		return nil, fmt.Errorf("live window tool rows: %w", err)
	}
	return result, nil
}

// LiveStats is the Pulse stat strip: burn rate, spend context, session
// rhythm, and the most recent model/tool activity.
type LiveStats struct {
	BurnPerHour  float64   // last 10 minutes of cost, scaled to an hour
	BurnHistory  []float64 // five 10-minute cost windows, oldest first
	TodayCost    float64
	Avg7dCost    float64   // mean daily cost over the 7 days before today
	SessionStart time.Time // started_at of the most recent session, zero if none
	LastSpanAt   time.Time // most recent llm/tool span, zero if none
	CachePct     float64   // cache read share of input today
	LastModel    string
	LastTool     string
	LastToolMs   int
	LastToolOK   bool
}

func (s *Service) LiveStats(now time.Time) (LiveStats, error) {
	var st LiveStats
	db := s.store.DB()

	// Burn history: five 10-minute windows ending now; the newest one,
	// scaled to an hour, is the headline burn rate.
	const window = 10 * time.Minute
	st.BurnHistory = make([]float64, 5)
	from := now.Add(-5 * window)
	rows, err := db.Query(
		`SELECT started_at, COALESCE(cost_usd, 0) FROM llm_calls WHERE started_at >= ?`,
		ts(from),
	)
	if err != nil {
		return st, fmt.Errorf("live stats burn: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var startedAt time.Time
		var cost float64
		if err := rows.Scan(&startedAt, &cost); err != nil {
			return st, err
		}
		if i := int(startedAt.Sub(from) / window); i >= 0 && i < 5 {
			st.BurnHistory[i] += cost
		}
	}
	if err := rows.Err(); err != nil {
		return st, fmt.Errorf("live stats burn rows: %w", err)
	}
	st.BurnPerHour = st.BurnHistory[4] * 6

	// Sum llm_calls by call time, not sessions by start time: a session
	// that crosses midnight would otherwise put this morning's spend on
	// yesterday and make "today" disagree with the burn rate.
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	_ = db.QueryRow(
		`SELECT COALESCE(SUM(cost_usd), 0) FROM llm_calls WHERE started_at >= ?`,
		ts(midnight),
	).Scan(&st.TodayCost)

	weekAgo := midnight.AddDate(0, 0, -7)
	var weekCost float64
	_ = db.QueryRow(
		`SELECT COALESCE(SUM(cost_usd), 0) FROM llm_calls WHERE started_at >= ? AND started_at < ?`,
		ts(weekAgo), ts(midnight),
	).Scan(&weekCost)
	st.Avg7dCost = weekCost / 7

	// MAX() strips the TIMESTAMP decltype, so these come back as raw
	// stored text and must be parsed by hand.
	var sessionStart sql.NullString
	_ = db.QueryRow(`SELECT MAX(started_at) FROM sessions`).Scan(&sessionStart)
	if sessionStart.Valid {
		if t, ok := parseStoredTime(sessionStart.String); ok {
			st.SessionStart = t
		}
	}

	var lastLLM, lastTool sql.NullString
	_ = db.QueryRow(`SELECT MAX(started_at) FROM llm_calls`).Scan(&lastLLM)
	_ = db.QueryRow(`SELECT MAX(started_at) FROM tool_calls`).Scan(&lastTool)
	for _, raw := range []sql.NullString{lastLLM, lastTool} {
		if !raw.Valid {
			continue
		}
		if t, ok := parseStoredTime(raw.String); ok && t.After(st.LastSpanAt) {
			st.LastSpanAt = t
		}
	}

	var cacheRead, input float64
	_ = db.QueryRow(
		`SELECT COALESCE(SUM(cache_read_tokens), 0), COALESCE(SUM(input_tokens), 0)
		 FROM llm_calls WHERE started_at >= ?`,
		ts(midnight),
	).Scan(&cacheRead, &input)
	if cacheRead+input > 0 {
		st.CachePct = cacheRead / (cacheRead + input) * 100
	}

	var model sql.NullString
	_ = db.QueryRow(
		`SELECT model FROM llm_calls ORDER BY started_at DESC LIMIT 1`,
	).Scan(&model)
	st.LastModel = model.String

	var toolName sql.NullString
	var toolMs sql.NullInt64
	var toolOK sql.NullBool
	_ = db.QueryRow(
		`SELECT tool_name, duration_ms, success FROM tool_calls ORDER BY started_at DESC LIMIT 1`,
	).Scan(&toolName, &toolMs, &toolOK)
	st.LastTool = toolName.String
	st.LastToolMs = int(toolMs.Int64)
	st.LastToolOK = toolOK.Bool

	return st, nil
}
