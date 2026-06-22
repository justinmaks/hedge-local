# hcli Roadmap

Living document tracking what's deferred, what's planned, and what's done.
Last updated: 2026-06-21

## MVP (v0.1) — 4 Phases

### Phase 1: Foundation + Claude Code Collection (COMPLETE)
- [x] Spec written
- [x] Plan written (17 tasks, TDD)
- [x] Implementation complete — 30 tests pass, integration test proves full pipeline
- **Scope:** Go skeleton, SQLite store, OTLP receiver, Claude Code normalizer, `hcli collect`, `hcli setup claude`, `hcli query`

### Phase 2: OpenCode Support + Pricing (COMPLETE)
- [x] Spec written
- [x] Plan written (10 tasks, TDD)
- [x] Implementation complete — 61 tests pass, integration test proves OpenCode OTLP pipeline
- **Scope:** `hcli setup opencode` (adds `@devtheops/opencode-plugin-otel` to `opencode.json` + env vars), OpenCode normalizer, pricing table fetch, `hcli pricing` commands
- **Key design decision:** Logs are raw-only to avoid double-counting with traces. Traces carry explicit cost/token fields; aggregate metrics accepted but not persisted unless safely mappable.

### Phase 3: TUI (COMPLETE)
- [x] Spec written
- [x] Plan written (18 tasks, TDD)
- [x] Implementation complete — 104 tests pass, integration test proves export pipeline
- **Scope:** Bubble Tea framework, 7 views (Overview, Cost, Tools, Models, Projects, Live, Export), `hcli tui` with embedded receiver fallback, `hcli export`
- **Key design decisions:** Date range filter drives all views via ReloadableView interface; numeric-aware table sorting; live polling with SQL-level agent filtering before LIMIT; logs raw-only to avoid double-counting; clipboard export via platform shell commands; Unicode block characters for charts (no charting library)

### Phase 4: Distribution + Polish (COMPLETE)
- [x] Spec written
- [x] Plan written (12 tasks, TDD)
- [x] Implementation complete
- **Scope:** `hcli status/stop/logs`, `hcli collect -d`, GoReleaser, GitHub Actions CI, README, CONTRIBUTING, LICENSE, first GitHub push + release

---

## Deferred to v0.2+

### Custom OpenCode Plugin (deferred from MVP)
**Originally planned:** Write a minimal embedded JS plugin (~150-200 lines) via `go:embed` that subscribes to OpenCode's plugin events and emits OTLP/HTTP JSON to `localhost:4318`.

**Why deferred:** [`@devtheops/opencode-plugin-otel`](https://github.com/DEVtheOPS/opencode-plugin-otel) already does this and more — 13 metrics including `opencode.cost.usage`, all 4 token types, tool duration, session duration, logs+traces+metrics via OTLP. Actively maintained (v1.2.0 shipped 2026-06-20, 78+ stars, 14 releases, MPL-2.0).

**When to revisit:**
- If we need Hedge-specific signals not covered by the community plugin
- If the community plugin is abandoned or breaks compatibility
- If users request tighter integration (e.g., session IDs that match our TUI's session concept)

**Escape hatch:** Fork the community plugin as `hedge-opencode-plugin`, publish to npm under our org. Lock-in is low because the data shape is OTEL-standard — our Go receiver treats it like any other OTLP stream.

### Budget Tracker UI + OS Notifications (deferred from MVP)
**Why deferred:** Per user decision (2026-06-21). The `budgets` table exists in the SQLite schema (ready for v0.2 without migration), but the TUI budget tracker panel and OS notification alerts are not built in MVP.

### Homebrew Tap (deferred from Phase 4)
**Why deferred:** No need for a separate tap repo yet. Shell installer covers macOS. Homebrew can be added later as a thin layer on existing GitHub Releases.

### systemd/launchd Service Integration (deferred from Phase 4)
**Why deferred:** Simple fork + PID file covers MVP. systemd unit (Linux) and launchd plist (macOS) for auto-restart and reboot survival deferred to v0.2.

### GIF Demo (deferred from Phase 4)
**Why deferred:** Recording instructions in `docs/demo.md`. Actual GIF created post-release with real data.

### Docker Images (deferred from Phase 4)
**Why deferred:** Single binary distribution is simpler. Docker images for containerized workflows deferred to v0.2.

### Other v0.2+ items
- Session drill-down with per-turn tree view
- Codex CLI support (needs `config.toml` setup automation + disable Statsig)
- OTLP/gRPC receiver (port 4317)
- Trace visualization (span tree / waterfall)
- Prometheus pull exporter
- Auto-archival / pruning of old data
- Theming / color customization
- Multi-user / shared DB
- Cloud sync (explicitly never — local-only is the product)

---

## Design Decisions Log

| Date | Decision | Rationale |
|---|---|---|
| 2026-06-21 | Use `@devtheops/opencode-plugin-otel` instead of custom plugin | Avoids reinventing working code; community plugin is more complete than our planned minimal version |
| 2026-06-21 | Defer budget tracker + notifications to v0.2 | Reduces MVP scope; schema is ready for v0.2 |
| 2026-06-21 | Go + Bubble Tea for tech stack | Single binary, great TUI, easy distribution |
| 2026-06-21 | OTLP/HTTP only for MVP (no gRPC) | Simpler; gRPC additive later |
| 2026-06-21 | Config dir `~/.hedge/` distinct from binary name `hcli` | Avoids collision with another `hedge` CLI the user has |
| 2026-06-21 | modernc.org/sqlite (pure Go, no CGO) | Easy cross-compilation for distribution |
| 2026-06-21 | OpenCode logs raw-only (no derived LLM/tool rows) | Prevents double-counting when traces and logs are both enabled |
| 2026-06-21 | Unicode block characters for charts | No charting library dependency; fits Lipgloss styling model |
| 2026-06-21 | ReloadableView interface for date-aware views | Date range filter drives all views through one reload path |
| 2026-06-21 | Numeric-aware table sorting | Strip $/%/ms and parse as float for correct numeric column ordering |
| 2026-06-21 | Live agent filter at SQL level (before LIMIT) | Prevents agent-filtered rows from being pushed out by other agents' traffic |
| 2026-06-21 | Simple fork daemon (not systemd) for MVP | Cross-platform, no root, no extra repos. systemd/launchd deferred to v0.2 |
| 2026-06-21 | Tag-triggered releases (not auto-release on main push) | Deliberate release control, no accidental releases |
| 2026-06-21 | Deferred Homebrew to v0.2 | Shell installer covers macOS; no separate tap repo needed yet |
