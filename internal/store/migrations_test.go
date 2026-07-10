package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// buildPre002DB creates a database with only migration 001 applied and
// returns its path, simulating a store written by an older hcli where OTLP
// retries could duplicate spans and inflate session aggregates.
func buildPre002DB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	defer db.Close()

	initSQL, err := migrationFS.ReadFile("001_init.sql")
	if err != nil {
		t.Fatalf("read 001_init.sql: %v", err)
	}
	if _, err := db.Exec(string(initSQL)); err != nil {
		t.Fatalf("apply 001: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES ('001_init')`); err != nil {
		t.Fatalf("mark 001 applied: %v", err)
	}

	// One session whose aggregates were inflated by a retried batch: the
	// same llm span and tool span landed twice.
	if _, err := db.Exec(`INSERT INTO projects (id, path, name) VALUES (1, '/repo', 'repo')`); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO sessions (id, external_id, agent, project_id, started_at,
		 total_cost_usd, total_input_tokens, total_output_tokens, message_count, tool_call_count)
		 VALUES (1, 'ext-old', 'claude_code', 1, '2026-07-01 10:00:00', 0.04, 2000, 1000, 2, 2)`,
	); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	for range 2 {
		if _, err := db.Exec(
			`INSERT INTO llm_calls (session_id, span_id, started_at, agent, model,
			 input_tokens, output_tokens, cost_usd)
			 VALUES (1, 'span-dup', '2026-07-01 10:00:01', 'claude_code', 'claude-sonnet-4', 1000, 500, 0.02)`,
		); err != nil {
			t.Fatalf("insert llm dup: %v", err)
		}
		if _, err := db.Exec(
			`INSERT INTO tool_calls (session_id, span_id, started_at, agent, tool_name, success)
			 VALUES (1, 'span-tool-dup', '2026-07-01 10:00:02', 'claude_code', 'bash', 1)`,
		); err != nil {
			t.Fatalf("insert tool dup: %v", err)
		}
	}

	// A session whose cost has no llm_calls rows behind it (older hcli
	// versions credited totals from sources that left none). The migration
	// must not touch it.
	if _, err := db.Exec(
		`INSERT INTO sessions (id, external_id, agent, project_id, started_at,
		 total_cost_usd, total_input_tokens, message_count)
		 VALUES (2, 'ext-legacy', 'claude_code', 1, '2026-06-21 09:00:00', 0.29, 500, 3)`,
	); err != nil {
		t.Fatalf("insert legacy session: %v", err)
	}
	return path
}

func TestMigration002_dedupsAndRecomputesAggregates(t *testing.T) {
	path := buildPre002DB(t)

	// Opening the store runs pending migrations, including 002.
	s, err := New(path)
	if err != nil {
		t.Fatalf("New (migrating): %v", err)
	}
	t.Cleanup(func() { s.Close() })

	var llmCount, toolCount int
	s.db.QueryRow(`SELECT count(*) FROM llm_calls WHERE span_id = 'span-dup'`).Scan(&llmCount)
	s.db.QueryRow(`SELECT count(*) FROM tool_calls WHERE span_id = 'span-tool-dup'`).Scan(&toolCount)
	if llmCount != 1 {
		t.Errorf("llm duplicates not removed: got %d rows, want 1", llmCount)
	}
	if toolCount != 1 {
		t.Errorf("tool duplicates not removed: got %d rows, want 1", toolCount)
	}

	var cost float64
	var inTok, msgCount, sessToolCount int
	s.db.QueryRow(
		`SELECT total_cost_usd, total_input_tokens, message_count, tool_call_count FROM sessions WHERE id = 1`,
	).Scan(&cost, &inTok, &msgCount, &sessToolCount)
	if abs(cost-0.02) > 0.0001 {
		t.Errorf("session cost not recomputed: got %v, want 0.02", cost)
	}
	if inTok != 1000 {
		t.Errorf("input tokens not recomputed: got %d, want 1000", inTok)
	}
	if msgCount != 1 {
		t.Errorf("message_count not recomputed: got %d, want 1", msgCount)
	}
	if sessToolCount != 1 {
		t.Errorf("tool_call_count not recomputed: got %d, want 1", sessToolCount)
	}

	// The legacy session without llm_calls rows keeps its aggregates.
	var legacyCost float64
	var legacyTok, legacyMsg int
	s.db.QueryRow(
		`SELECT total_cost_usd, total_input_tokens, message_count FROM sessions WHERE id = 2`,
	).Scan(&legacyCost, &legacyTok, &legacyMsg)
	if abs(legacyCost-0.29) > 0.0001 || legacyTok != 500 || legacyMsg != 3 {
		t.Errorf("legacy session was modified: cost %v tokens %d msgs %d, want 0.29 500 3",
			legacyCost, legacyTok, legacyMsg)
	}

	// The unique index must now block new duplicates.
	if _, err := s.db.Exec(
		`INSERT INTO llm_calls (session_id, span_id, started_at, agent) VALUES (1, 'span-dup', '2026-07-01 11:00:00', 'claude_code')`,
	); err == nil {
		t.Error("expected plain INSERT of duplicate span to violate unique index")
	}
}
