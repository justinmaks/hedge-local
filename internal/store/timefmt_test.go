package store

import (
	"database/sql"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

func TestFormatTime_canonicalUTC(t *testing.T) {
	loc := time.FixedZone("EDT", -4*60*60)
	local := time.Date(2026, 7, 10, 21, 15, 0, 123_000_000, loc)
	got := FormatTime(local)
	want := "2026-07-11 01:15:00.123+00:00"
	if got != want {
		t.Errorf("FormatTime: got %q, want %q", got, want)
	}
}

func TestParseTime_roundTripAndLegacy(t *testing.T) {
	orig := time.Date(2026, 7, 10, 6, 59, 2, 123_000_000, time.UTC)
	parsed, ok := ParseTime(FormatTime(orig))
	if !ok || !parsed.Equal(orig) {
		t.Errorf("canonical round trip: got %v ok=%v, want %v", parsed, ok, orig)
	}

	// Legacy driver format (Go time.String() shape, local offset).
	legacy := "2026-06-21 23:13:24.988 -0400 EDT"
	parsed, ok = ParseTime(legacy)
	if !ok {
		t.Fatalf("legacy format should parse: %q", legacy)
	}
	wantUTC := time.Date(2026, 6, 22, 3, 13, 24, 988_000_000, time.UTC)
	if !parsed.Equal(wantUTC) {
		t.Errorf("legacy parse: got %v, want %v", parsed.UTC(), wantUTC)
	}

	if _, ok := ParseTime("not a time"); ok {
		t.Error("garbage should not parse")
	}
}

// canonicalTimestampRe matches store.TimeLayout output exactly.
var canonicalTimestampRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3}\+00:00$`)

// TestTimestamps_allWritesCanonical is a tripwire: seed the store through the
// public write paths, then assert every stored timestamp string is in the
// canonical UTC layout. Fails if any write path bypasses FormatTime.
func TestTimestamps_allWritesCanonical(t *testing.T) {
	s := tempDB(t)
	if err := s.SeedBundledPricing(); err != nil {
		t.Fatalf("seed pricing: %v", err)
	}
	pid, _ := s.ProjectUpsert("/repo", "repo")
	now := time.Now()
	sid, _ := s.SessionUpsert("ext-canon", "claude_code", pid, now, "1.0")
	if err := s.SessionSetEnded(sid, now.Add(time.Minute)); err != nil {
		t.Fatalf("set ended: %v", err)
	}
	if _, err := s.LLMCallInsert(LLMCallParams{
		SessionID: sid, SpanID: "span-canon", StartedAt: now, Agent: "claude_code", Model: "m",
	}); err != nil {
		t.Fatalf("llm insert: %v", err)
	}
	if _, err := s.ToolCallInsert(ToolCallParams{
		SessionID: sid, SpanID: "span-canon-tool", StartedAt: now, Agent: "claude_code", ToolName: "bash",
	}); err != nil {
		t.Fatalf("tool insert: %v", err)
	}
	if _, err := s.EventInsert(EventParams{
		SessionID: sid, Timestamp: now, Agent: "claude_code", EventName: "e", Payload: "{}",
	}); err != nil {
		t.Fatalf("event insert: %v", err)
	}

	for _, tc := range timestampColumns {
		q := "SELECT CAST(" + tc.column + " AS TEXT) FROM " + tc.table + " WHERE " + tc.column + " IS NOT NULL"
		rows, err := s.db.Query(q)
		if err != nil {
			t.Fatalf("query %s.%s: %v", tc.table, tc.column, err)
		}
		for rows.Next() {
			var raw string
			if err := rows.Scan(&raw); err != nil {
				t.Fatalf("scan %s.%s: %v", tc.table, tc.column, err)
			}
			if !canonicalTimestampRe.MatchString(raw) {
				t.Errorf("%s.%s has non-canonical timestamp %q", tc.table, tc.column, raw)
			}
		}
		rows.Close()
	}
}

func TestNormalizeTimestamps_convertsLegacyRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")

	// Build a DB with legacy-format timestamps written as raw strings, the
	// way the old driver serialization left them.
	{
		db, err := sql.Open("sqlite", "file:"+path)
		if err != nil {
			t.Fatalf("open raw: %v", err)
		}
		initSQL, err := migrationFS.ReadFile("001_init.sql")
		if err != nil {
			t.Fatalf("read 001: %v", err)
		}
		if _, err := db.Exec(string(initSQL)); err != nil {
			t.Fatalf("apply 001: %v", err)
		}
		dedupSQL, err := migrationFS.ReadFile("002_span_dedup.sql")
		if err != nil {
			t.Fatalf("read 002: %v", err)
		}
		if _, err := db.Exec(string(dedupSQL)); err != nil {
			t.Fatalf("apply 002: %v", err)
		}
		if _, err := db.Exec(`CREATE TABLE schema_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP)`); err != nil {
			t.Fatalf("schema_migrations: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES ('001_init'), ('002_span_dedup')`); err != nil {
			t.Fatalf("mark applied: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO projects (id, path, name) VALUES (1, '/repo', 'repo')`); err != nil {
			t.Fatalf("project: %v", err)
		}
		if _, err := db.Exec(
			`INSERT INTO sessions (id, external_id, agent, project_id, started_at, total_cost_usd)
			 VALUES (1, 'ext-legacy', 'claude_code', 1, '2026-06-21 23:13:24.988 -0400 EDT', 0.29)`,
		); err != nil {
			t.Fatalf("session: %v", err)
		}
		if _, err := db.Exec(
			`INSERT INTO llm_calls (session_id, span_id, started_at, agent, model, cost_usd)
			 VALUES (1, 'span-legacy', '2026-06-21 23:14:01.5 -0400 EDT', 'claude_code', 'claude-sonnet-4', 0.29)`,
		); err != nil {
			t.Fatalf("llm: %v", err)
		}
		db.Close()
	}

	// Opening the store converts everything once.
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	var rawSession, rawLLM string
	s.db.QueryRow(`SELECT CAST(started_at AS TEXT) FROM sessions WHERE id = 1`).Scan(&rawSession)
	s.db.QueryRow(`SELECT CAST(started_at AS TEXT) FROM llm_calls WHERE span_id = 'span-legacy'`).Scan(&rawLLM)
	if rawSession != "2026-06-22 03:13:24.988+00:00" {
		t.Errorf("session started_at: got %q, want UTC canonical", rawSession)
	}
	if rawLLM != "2026-06-22 03:14:01.500+00:00" {
		t.Errorf("llm started_at: got %q, want UTC canonical", rawLLM)
	}

	// A BETWEEN range bound with FormatTime must find the converted row.
	var n int
	from := FormatTime(time.Date(2026, 6, 22, 3, 0, 0, 0, time.UTC))
	to := FormatTime(time.Date(2026, 6, 22, 4, 0, 0, 0, time.UTC))
	s.db.QueryRow(`SELECT count(*) FROM sessions WHERE started_at BETWEEN ? AND ?`, from, to).Scan(&n)
	if n != 1 {
		t.Errorf("range query after conversion: got %d rows, want 1", n)
	}

	// Conversion is one-shot: the meta flag is set.
	var flag string
	if err := s.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, timestampsMetaKey).Scan(&flag); err != nil {
		t.Errorf("conversion flag missing: %v", err)
	}
}
