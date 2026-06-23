# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.1] - 2026-06-22

### Fixed
- Overview cost/token cards were labeled "Today Cost"/"Today Tokens" but show
  totals for the selected date range; relabeled to "Cost"/"Tokens".
- Projects "Last Active" was always blank — `MAX(started_at)` is returned by the
  SQLite driver as a Go `time.String()` value (with a monotonic-clock suffix)
  that RFC3339 parsing rejected. Now parsed correctly.

### Added
- README screenshots: a demo GIF plus Overview/Cost/Tools/Models/Projects stills.

### Security
- Upgraded the transitive `google.golang.org/grpc` to v1.81.1 to clear a critical
  advisory (gRPC-Go authorization bypass). hcli is HTTP-only and does not run a
  gRPC server, so it was not reachable (govulncheck confirms), but the dependency
  is updated regardless.

## [0.1.0] - 2026-06-22

First public release.

### Added
- Local-only OTLP/HTTP receiver (port 4318) for Claude Code and OpenCode telemetry.
- SQLite store (WAL mode) with cost computation from bundled/fetchable pricing.
- 7-view Bubble Tea TUI: Overview, Cost, Tools, Models, Projects, Live, Export.
- `hcli` embedded mode and `collect` / `tui` / `status` / `stop` / `logs` daemon commands.
- `hcli setup claude` and `hcli setup opencode` agent configuration.
- `hcli query` read-only SQL access and `hcli export` (CSV/JSON/Markdown).
- `hcli pricing` list/import/fetch.
- Distribution via GoReleaser: binaries, `.deb`, `.rpm`, shell installer, `go install`.
- `SECURITY.md`, `CODE_OF_CONDUCT.md`, `ARCHITECTURE.md`, issue/PR templates, and Dependabot config.
- CI: blocking `govulncheck` job and race-enabled tests.

### Telemetry correctness
- **Claude Code** is trace-driven: the same LLM call is reported via both trace
  spans and metrics, so only spans create rows (metrics would double-count).
- **OpenCode** is log-driven: the `@devtheops` plugin delivers per-call data as
  log events (`api_request` → llm_call, `tool_result` → tool_call) with explicit
  cost; its trace spans are not reliably exported, so logs are canonical.
- Bundled pricing covers current Claude models (Opus 4.5–4.8, Sonnet 4.5/4.6,
  Haiku 4.5, Fable 5).

### Security
- `~/.hedge/` is created `0700` and the SQLite database (plus WAL/SHM) and daemon
  logs `0600`, keeping captured telemetry owner-only.
- OTLP receiver bounds each request body (`http.MaxBytesReader`, 16 MiB) and sets
  `ReadHeaderTimeout`.
- `hcli query` runs on a read-only (`query_only`) database connection.
- `golang.org/x/net` at `v0.51.0` (resolves `GO-2026-4559`).

[Unreleased]: https://github.com/justinmaks/hedge-local/compare/v0.1.1...HEAD
[0.1.1]: https://github.com/justinmaks/hedge-local/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/justinmaks/hedge-local/releases/tag/v0.1.0
