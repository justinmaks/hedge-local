# hcli — Project Memory

## What this is

hcli is a local-only TUI/CLI tool that collects OpenTelemetry (OTEL) metrics from coding agents (Claude Code, OpenCode in MVP; Codex CLI in v0.2), stores them in a local SQLite database, and visualizes cost/tokens/tool usage/latency in a terminal UI.

- **Binary name:** `hcli` (avoids collision with another `hedge` CLI the user has)
- **Config dir:** `~/.hedge/` (distinct from binary name)
- **Repo:** `github.com/justinmaks/hedge-local`
- **License:** MIT
- **Local-only:** No usage data leaves the machine. Optional pricing fetch + upgrade check, both toggleable off.

## Tech Stack

- Go 1.25+ (single binary, no CGO)
- spf13/cobra for CLI
- Bubble Tea + Lipgloss for TUI (Plan 3)
- modernc.org/sqlite (pure Go SQLite, no CGO) — easy cross-compilation
- BurntSushi/toml for config
- go.opentelemetry.io/proto/otlp for OTLP parsing

## Key Design Decisions

- **Single Go binary, two modes**: `hcli collect` (daemon) and `hcli tui` (foreground UI). Both link the same SQLite reader/writer code.
- **OTLP/HTTP only for MVP** (port 4318, protobuf + JSON). gRPC (4317) deferred to v0.2.
- **Normalizer layer** abstracts per-agent differences so the TUI queries one unified schema.
- **SQLite WAL mode** for concurrent read (TUI) + write (daemon).
- **TUI can optionally embed the receiver** as a goroutine for users who skip the daemon.
- **Pricing**: bundled in binary + optional fetch from `dist/pricing/pricing.json` in main repo. Separate `hedge-pricing` repo deferred.
- **OpenCode support**: use `@devtheops/opencode-plugin-otel` npm package as-is for MVP. `hcli setup opencode` adds it to `opencode.json` + sets env vars. Custom embedded JS plugin deferred to v0.2 (see `docs/roadmap.md`). Plugin traces/logs carry usable token and explicit cost fields; aggregate metrics are accepted but only persisted when they can be mapped safely without double-counting.
- **Claude Code support**: native OTEL, pass-through `claude_code.cost.usage` metric. `hcli setup claude` writes env vars to `~/.hedge/env.sh`.
- **Logs optional**: off by default, enable with `hcli collect --with-logs`. Reduces DB size.
- **Budget tracker UI + OS notifications deferred to v0.2** per user decision. Budgets table in schema but unused in MVP.
- **Distribution**: Homebrew (macOS), shell installer + .deb + .rpm + `go install` + GitHub Releases (Linux primary). GoReleaser handles all from one config.
- **TUI status badge**: `hcli tui` checks the daemon PID and shows `COLLECTING` when the external daemon is alive; read-only/no-daemon mode shows `DB LIVE` instead of misleading `IDLE`.
- **No GitHub push until MVP is working** per user instruction (2026-06-21).

## Agent OTEL Feasibility (verified 2026-06-21)

| Agent | OTEL | Cost metric | MVP effort |
|---|---|---|---|
| Claude Code | Full, documented | `claude_code.cost.usage` (USD) built-in | Zero — set env vars |
| OpenCode | Via `@devtheops/opencode-plugin-otel` | Explicit cost in plugin traces/logs; pricing fallback from tokens | Setup + normalizer |
| Codex CLI | Full but undocumented; metrics ON by default to OpenAI's Statsig | No | Deferred to v0.2 |

## Key References

- **Original Spec:** `docs/superpowers/specs/2026-06-21-hcli-design.md` (487 lines, committed)
- **Phase 2 Spec:** `docs/superpowers/specs/2026-06-21-hcli-phase-2-opencode-pricing-design.md`
- **Phase 3 Spec:** `docs/superpowers/specs/2026-06-21-hcli-phase-3-tui-design.md`
- **Plan 1:** `docs/superpowers/plans/2026-06-21-hcli-plan-1-foundation.md` (17 tasks, TDD)
- **Plan 2:** `docs/superpowers/plans/2026-06-21-hcli-plan-2-opencode-pricing.md` (10 tasks, TDD)
- **Plan 3:** `docs/superpowers/plans/2026-06-21-hcli-plan-3-tui.md` (18 tasks, TDD)
- **Claude Code OTEL docs:** https://code.claude.com/docs/en/monitoring-usage
- **OpenCode plugin docs:** https://opencode.ai/docs/plugins/
- **Plugin inspiration:** https://github.com/DEVtheOPS/opencode-plugin-otel (MPL-2.0, fresh code, no copy)

## Roadmap — 4 Planned Phases

### Phase 1: Foundation + Claude Code Collection (COMPLETE)
**Plan:** `docs/superpowers/plans/2026-06-21-hcli-plan-1-foundation.md` (17 tasks)
**Goal:** Project skeleton, SQLite store, OTLP receiver, Claude Code normalizer, `hcli collect`, `hcli setup claude`, `hcli query`.
**Ship state:** Claude Code telemetry flows into SQLite, queryable via SQL.
**Status:** All 17 tasks complete. 30 tests pass. Integration test proves full pipeline (OTLP POST → receiver → normalizer → writer → SQLite with cost computation).

### Phase 2: OpenCode Support + Pricing (COMPLETE)
**Plan:** `docs/superpowers/plans/2026-06-21-hcli-plan-2-opencode-pricing.md` (10 tasks)
**Goal:** `hcli setup opencode` (adds @devtheops/opencode-plugin-otel to opencode.json + env vars), OpenCode normalizer, pricing table fetch, `hcli pricing` commands.
**Ship state:** Both Claude Code and OpenCode collect through the same OTLP/HTTP receiver via composite normalizer. OpenCode cost from explicit plugin trace cost or pricing fallback. Pricing can be listed, imported, and explicitly fetched.
**Status:** All 10 tasks complete. 61 tests pass. Integration test proves OpenCode OTLP traces flow through the full pipeline. Logs are raw-only to avoid double-counting with traces.

### Phase 3: TUI (COMPLETE)
**Plan:** `docs/superpowers/plans/2026-06-21-hcli-plan-3-tui.md` (18 tasks)
**Goal:** Bubble Tea framework, all 7 views (Overview, Cost, Tools, Models, Projects, Live, Export), `hcli tui` with embedded receiver fallback, `hcli export`.
**Ship state:** Full TUI works end-to-end.
**Status:** All 18 tasks complete. 104 tests pass. Integration test proves export pipeline. Date range filter drives all views via ReloadableView interface. Numeric-aware table sorting. Live polling with SQL-level agent filtering. Clipboard export via platform shell commands.

### Phase 4: Distribution + Polish (COMPLETE)
**Goal:** `hcli status/stop/logs`, GoReleaser (binaries, .deb, .rpm, install.sh), GitHub Actions CI, README + CONTRIBUTING, LICENSE, first GitHub push + release.
**Ship state:** Distributable to others. Ready for GitHub push.
**Status:** All tasks complete. 131 tests pass. Daemon lifecycle integration test passes. GoReleaser snapshot builds. GitHub Actions CI and release workflows configured.

## Development Workflow

- **TDD strictly**: write failing test → run (fail) → implement → run (pass) → commit
- **Subagent-driven execution**: fresh subagent per task, two-stage review (spec + code quality) after each
- **Frequent commits**: one commit per task, clear messages
- **No GitHub push until MVP works**: all 4 phases complete locally first

## Repo Structure (current)

```
hedge-local/
├── AGENTS.md                       # this file
├── cmd/hcli/main.go                # entry point
├── go.mod, go.sum
├── internal/
│   ├── cli/                        # cobra commands (collect, query, setup, pricing, tui, export, version)
│   ├── config/                     # config.toml load
│   ├── store/                      # SQLite schema, migrations, queries, pricing
│   ├── collect/                    # OTLP receiver + writer
│   ├── normalize/                  # per-agent adapters (Claude Code, OpenCode, composite)
│   └── tui/                        # Bubble Tea views + query layer
│       ├── queries/                # read-only store queries for all views
│       └── views/                  # 7 view implementations
├── migrations/                     # SQL files, embedded via go:embed
├── dist/
│   └── pricing/pricing.json        # bundled pricing fallback
├── docs/
│   └── superpowers/
│       ├── specs/                  # design specs (3 specs)
│       └── plans/                  # implementation plans (3 plans)
└── .github/workflows/              # CI (Phase 4)
```

## Success Criteria (MVP — all 4 phases)

1. Fresh user: `curl ... | sh && hcli setup && hcli collect -d && hcli tui` → telemetry in 60s
2. Cost accuracy within 1¢ of Claude Code's metric; OpenCode cost matches manual calc
3. All 7 TUI views functional with date range filtering (Today/7d/30d/Custom)
4. Export: CSV/JSON/Markdown produces valid files
5. TUI renders <200ms with 10k spans; daemon <50MB RAM idle
6. Local-only verified — no outbound calls except pricing fetch + upgrade check
7. Homebrew + shell installer + .deb + .rpm + `go install` + GitHub Releases all work
8. README with quickstart + GIF + troubleshooting

## Out of Scope (v0.2+)

Budget tracker UI + OS notifications, systemd/launchd service integration, session drill-down tree, Codex CLI, OTLP/gRPC, trace visualization, Prometheus, auto-pruning, theming, multi-user, cloud sync (never).
