# Architecture

This document explains how hcli is put together and, more importantly, the
non-obvious decisions and footguns — especially **cost attribution**, which has
several subtleties worth writing down.

## Data flow

```
Coding agent (Claude Code / OpenCode)
  │  OTLP/HTTP (protobuf) on 127.0.0.1:4318
  ▼
Receiver            internal/collect/receiver.go
  │  parses ExportTrace/Metrics/Logs ServiceRequest
  ▼
Normalizer          internal/normalize/*.go
  │  per-agent adapters → unified []Event
  ▼
Writer              internal/collect/writer.go
  │  computes cost, upserts project/session, inserts rows
  ▼
SQLite (WAL)        internal/store/*.go
  ▲
  │  read-only queries
TUI / CLI           internal/tui/*, internal/cli/*
```

Everything is one Go binary with no CGO (pure-Go SQLite via
`modernc.org/sqlite`). The receiver and the TUI can run in the same process
(`hcli`) or separately (`hcli collect -d` daemon + `hcli tui`).

## Components

- **Receiver** (`internal/collect/receiver.go`) — an `http.Server` bound to
  `127.0.0.1` only, with `/v1/traces`, `/v1/metrics`, `/v1/logs`, `/health`.
  Each handler caps the body with `http.MaxBytesReader` (16 MiB) and the server
  sets `ReadHeaderTimeout`. It unmarshals the OTLP protobuf and hands the request
  to the normalizer.
- **Normalizers** (`internal/normalize/`) — translate agent-specific OTLP into a
  unified `Event` stream (`EventLLMCall`, `EventToolCall`, `EventLog`). One
  adapter per agent (`claude_code.go`, `opencode.go`) plus a `CompositeNormalizer`.
- **Writer** (`internal/collect/writer.go`) — derives cost, upserts the project
  and session, and inserts `llm_calls` / `tool_calls` / `events`.
- **Store** (`internal/store/`) — schema (embedded migrations), pricing, and all
  SQL. Single writer connection (`SetMaxOpenConns(1)`) in WAL mode.
- **TUI / queries** (`internal/tui/`) — Bubble Tea views reading through a
  read-only query layer.

## Agent attribution & normalizer self-selection

The `CompositeNormalizer` runs **every** child normalizer on **every** OTLP
request and concatenates the results (`composite.go`). It does not pre-route by
agent. This only works because each normalizer **self-selects** by signal and
ignores everything else:

- `ClaudeCodeNormalizer` only emits for spans named `claude_code.*` (e.g.
  `claude_code.llm_request`, `claude_code.tool`).
- `OpenCodeNormalizer` only emits for spans named `opencode.*` and, for logs,
  explicitly skips `claude_code.*` records.

**Footgun:** if you add a normalizer, it must recognize *only* its own signals.
Two normalizers claiming the same span would produce duplicate rows. The `agent`
column on every row records which adapter produced it (`claude_code`,
`opencode`).

## Cost attribution (the important part)

Cost lives on `llm_calls`. Tool calls have no cost. A session's cost and token
totals are **the running sum of its `llm_calls`** — `LLMCallInsert` calls
`SessionAddCost` / `SessionAddTokens` on every insert (`store/llm_calls.go`).
That design has one critical consequence: **a duplicate `llm_call` doubles the
session totals.** Avoiding duplicates is therefore a correctness requirement,
not a nicety.

### One source of truth per call

Agents often report the same LLM call through multiple OTLP signals. We pick a
single canonical source per agent:

| Agent       | Canonical source for `llm_calls` | Why |
|-------------|----------------------------------|-----|
| Claude Code | **trace spans** (`claude_code.llm_request`) | Metrics (`claude_code.cost.usage` / `token.usage`) report the *same* calls but carry **no `span_id`**, and their data-point timestamps don't line up with span start times — so there is **no reliable key to join a metric to a specific call**. `NormalizeMetrics` therefore emits nothing; deriving rows from both streams double-counted every call. |
| OpenCode    | **trace spans** (`opencode.*` LLM spans) | The plugin puts tokens and explicit cost on the span. Metrics emit nothing; logs are stored raw (see below). |

This is why `ClaudeCodeNormalizer.NormalizeMetrics` and
`OpenCodeNormalizer.NormalizeMetrics` both return `nil`. It mirrors the
"logs are raw-only" rule: **never derive billable rows from a second signal that
duplicates the first.**

### How the dollar figure is derived

In `Writer.writeLLMCall`:

1. If the normalized event already carries an explicit `CostUSD` (OpenCode spans
   expose `llm.cost.total`), use it verbatim.
2. Otherwise look up the pricing row for `(provider, model)` effective at the
   call's start time and compute:

   ```
   cost = input_tokens      / 1e6 * input_per_1m
        + output_tokens     / 1e6 * output_per_1m
        + cache_read_tokens / 1e6 * cache_read_per_1m
        + cache_write_tokens/ 1e6 * cache_write_per_1m
   ```

Claude Code spans don't include a cost attribute, so Claude Code cost is
**always** pricing-derived. OpenCode prefers its explicit cost and falls back to
pricing.

**Token buckets are separate and must not overlap.** `input_tokens` is *uncached*
input; cache reads and cache writes are billed at their own (much lower / higher)
rates. The agents report these as distinct fields and we cost them
independently, matching Anthropic's billing. Summing cache tokens into
`input_tokens` would over-bill.

### Pricing lookup & gotchas

Pricing lives in `dist/pricing/pricing.json`, embedded at build time and seeded
into the DB on first `collect`. `PricingFor` does an **exact** `(provider,
model)` match and takes the most recent row with `effective_from <= call time`.

- **Exact model match.** Claude Code reports point-release IDs like
  `claude-opus-4-8`, not `claude-opus-4`. Each needs its own pricing row — there
  is intentionally **no prefix/family matching**, because point releases have had
  different prices (Opus 4.0/4.1 were \$15/\$75; Opus 4.5+ are \$5/\$25). Guessing
  by family would produce wrong costs.
- **Missing model ⇒ \$0.** If a model isn't in the table, pricing returns no row
  and cost is `$0`. Keep `pricing.json` current; users can also run
  `hcli pricing fetch` / `hcli pricing import`.
- `cache_write_per_1m` is the **5-minute** cache-write tier (1.25× base input).
  The 1-hour tier is not modeled.

## Logs are raw-only

When `--with-logs` is enabled, log records are stored verbatim in `events` and
are **never** turned into `llm_calls` or `tool_calls`. Logs frequently duplicate
trace/metric content; treating them as raw-only is what prevents double-counting
across signals. Logs are off by default to keep the DB small and avoid storing
prompt content unless asked.

## Sessions & projects

- `session.id` from the agent keys a session; `SessionUpsert` creates it lazily
  on the first event and is idempotent.
- Project attribution comes from the `hcli.project_path` resource attribute (set
  via `OTEL_RESOURCE_ATTRIBUTES`). Absent that, work is grouped under
  `(ungrouped)`.

## Concurrency & storage

- SQLite in WAL mode with a single writer connection and `busy_timeout`, so the
  daemon writes while the TUI reads.
- `hcli query` opens a separate **read-only** connection (`query_only` PRAGMA) so
  arbitrary user SQL cannot write, on top of the `SELECT`/`WITH` prefix check.

## Security posture

- Receiver binds `127.0.0.1` only; bodies are size-capped; the server has a read
  header timeout.
- `~/.hedge/` is created `0700`; the database (+ WAL/SHM) and daemon logs are
  `0600`, so captured telemetry is owner-only on shared machines.
- The only outbound call in normal operation is the explicit `hcli pricing fetch`.

See [HARDENING decisions in the roadmap](docs/roadmap.md#design-decisions-log)
for the rationale behind these.

## Adding a new agent

1. Add a normalizer in `internal/normalize/` implementing `Normalizer`, emitting
   events **only** for that agent's own span/log signals.
2. Set the `Agent()` string; it becomes the `agent` column value.
3. Register it in the `CompositeNormalizer` (see `internal/cli/collect.go`).
4. Decide the canonical signal for `llm_calls` and make the other signals inert
   (return `nil`) to avoid double-counting.
5. Add a `hcli setup <agent>` command if it needs env/config wiring.
6. Add pricing rows if cost is pricing-derived rather than reported on-span.
