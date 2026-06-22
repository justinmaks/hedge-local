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
- `OpenCodeNormalizer` only emits from OpenCode's log events and explicitly skips
  `claude_code.*` log records (see "One source of truth per call" below for why
  OpenCode is log-driven).

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

Agents often report the same LLM call through multiple OTLP signals (traces,
metrics, logs). The rule is: **pick exactly one canonical signal per agent** —
the one that carries complete, non-duplicated, per-call data — and make every
other signal inert for row creation. The two agents we support deliberately use
**different** canonical signals, and that is fine: the normalizer layer exists
precisely to absorb these per-agent differences and hand the writer one uniform
`Event` stream. Downstream (writer, store, TUI) is identical regardless.

| Agent       | Canonical signal for rows | Why this one |
|-------------|---------------------------|--------------|
| **Claude Code** | **trace spans** (`claude_code.llm_request`, `claude_code.tool`) | Spans carry full per-call usage. The metrics (`claude_code.cost.usage` / `token.usage`) report the *same* calls but have **no `span_id`** and timestamps that don't line up with span starts — no reliable key to join them, so using both double-counts. `NormalizeMetrics` returns `nil`. |
| **OpenCode** | **log events** (`api_request` → `llm_call`, `tool_result` → `tool_call`) | The `@devtheops` plugin reliably emits these logs with full token counts **and explicit cost** (`cost_usd`). Its LLM/tool *trace spans* are not reliably exported (e.g. in `opencode run`), so traces would miss calls. `NormalizeTraces` and `NormalizeMetrics` return `nil`; only logs create rows. Other log events (`session.created`, `user_prompt`, `session.idle`, …) are stored raw under `--with-logs`. |

**Why mixed signals is correct, not a smell:** the canonical signal is whichever
one a given agent emits completely and exactly once per call. For Claude Code
that's traces (metrics duplicate them); for OpenCode's plugin that's logs (spans
are unreliable). Each normalizer makes its non-canonical signals return `nil`,
upholding the single rule that matters: **never derive billable rows from a
second signal that duplicates the first.**

### How the dollar figure is derived

In `Writer.writeLLMCall`:

1. If the normalized event already carries an explicit `CostUSD` (OpenCode's
   `api_request` log exposes `cost_usd`), use it verbatim.
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

## Logs: raw storage vs. canonical rows

How a log record is treated is **per-agent**, decided by that agent's normalizer:

- **Claude Code:** logs are raw-only. They duplicate trace/metric content, so
  they are stored verbatim in `events` (under `--with-logs`) and never become
  `llm_calls`/`tool_calls`.
- **OpenCode:** the `api_request` and `tool_result` logs **are** the canonical
  signal, so the normalizer turns them into `llm_calls` / `tool_calls` (always,
  regardless of `--with-logs` — they're structured, not prompt bodies). Its other
  log events are stored raw under `--with-logs`.

The `events` table only stores **raw** log records and is gated by `--with-logs`
(off by default, to keep the DB small and avoid storing prompt content unless
asked). Rows derived from logs (OpenCode's calls) go to `llm_calls`/`tool_calls`,
not `events`, so they persist even with logs off.

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
