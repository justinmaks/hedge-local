CREATE TABLE IF NOT EXISTS projects (
  id          INTEGER PRIMARY KEY,
  path        TEXT UNIQUE,
  name        TEXT,
  created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sessions (
  id          INTEGER PRIMARY KEY,
  external_id TEXT UNIQUE,
  agent       TEXT NOT NULL,
  project_id  INTEGER REFERENCES projects(id),
  started_at  TIMESTAMP NOT NULL,
  ended_at    TIMESTAMP,
  app_version TEXT,
  total_cost_usd      REAL DEFAULT 0,
  total_input_tokens  INTEGER DEFAULT 0,
  total_output_tokens INTEGER DEFAULT 0,
  total_cache_read_tokens  INTEGER DEFAULT 0,
  total_cache_write_tokens INTEGER DEFAULT 0,
  tool_call_count     INTEGER DEFAULT 0,
  message_count       INTEGER DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_sessions_agent_started ON sessions(agent, started_at);
CREATE INDEX IF NOT EXISTS idx_sessions_project_started ON sessions(project_id, started_at);

CREATE TABLE IF NOT EXISTS llm_calls (
  id              INTEGER PRIMARY KEY,
  session_id      INTEGER REFERENCES sessions(id),
  trace_id        TEXT,
  span_id         TEXT,
  parent_span_id  TEXT,
  started_at      TIMESTAMP NOT NULL,
  duration_ms     INTEGER,
  agent           TEXT NOT NULL,
  model           TEXT,
  provider        TEXT,
  input_tokens    INTEGER,
  output_tokens   INTEGER,
  cache_read_tokens  INTEGER,
  cache_write_tokens INTEGER,
  reasoning_tokens   INTEGER,
  cost_usd        REAL,
  ttft_ms         INTEGER,
  stop_reason     TEXT
);

CREATE INDEX IF NOT EXISTS idx_llm_calls_session ON llm_calls(session_id);
CREATE INDEX IF NOT EXISTS idx_llm_calls_started_model ON llm_calls(started_at, model);

CREATE TABLE IF NOT EXISTS tool_calls (
  id            INTEGER PRIMARY KEY,
  session_id    INTEGER REFERENCES sessions(id),
  llm_call_id   INTEGER REFERENCES llm_calls(id),
  trace_id      TEXT,
  span_id       TEXT,
  started_at    TIMESTAMP NOT NULL,
  duration_ms   INTEGER,
  agent         TEXT NOT NULL,
  tool_name     TEXT NOT NULL,
  success       INTEGER,
  error_message TEXT,
  input_summary  TEXT,
  output_summary TEXT
);

CREATE INDEX IF NOT EXISTS idx_tool_calls_session ON tool_calls(session_id);
CREATE INDEX IF NOT EXISTS idx_tool_calls_started_tool ON tool_calls(started_at, tool_name);

CREATE TABLE IF NOT EXISTS events (
  id          INTEGER PRIMARY KEY,
  session_id  INTEGER REFERENCES sessions(id),
  timestamp   TIMESTAMP NOT NULL,
  agent       TEXT NOT NULL,
  event_name  TEXT NOT NULL,
  payload     TEXT,
  trace_id    TEXT,
  span_id     TEXT
);

CREATE INDEX IF NOT EXISTS idx_events_session_time ON events(session_id, timestamp);

CREATE TABLE IF NOT EXISTS budgets (
  id          INTEGER PRIMARY KEY,
  scope       TEXT NOT NULL,
  scope_value TEXT,
  period      TEXT NOT NULL,
  limit_usd   REAL NOT NULL,
  created_at  TIMESTAMP,
  enabled     INTEGER DEFAULT 1
);

CREATE TABLE IF NOT EXISTS pricing (
  id           INTEGER PRIMARY KEY,
  provider     TEXT NOT NULL,
  model        TEXT NOT NULL,
  input_per_1m   REAL,
  output_per_1m  REAL,
  cache_read_per_1m    REAL,
  cache_write_per_1m   REAL,
  effective_from TIMESTAMP NOT NULL,
  effective_to   TIMESTAMP,
  source        TEXT,
  UNIQUE(provider, model, effective_from)
);

CREATE TABLE IF NOT EXISTS meta (
  key   TEXT PRIMARY KEY,
  value TEXT
);
