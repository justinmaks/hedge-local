# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `SECURITY.md`, `CODE_OF_CONDUCT.md`, issue/PR templates, and Dependabot config.
- CI: blocking `govulncheck` job and race-enabled tests.

### Security
- `~/.hedge/` is now created `0700` and the SQLite database (plus WAL/SHM) and
  daemon logs `0600`, keeping captured telemetry owner-only.
- OTLP receiver bounds each request body (`http.MaxBytesReader`, 16 MiB) and sets
  `ReadHeaderTimeout`.
- `hcli query` runs on a read-only (`query_only`) database connection.
- Upgraded `golang.org/x/net` to `v0.51.0` (resolves `GO-2026-4559`).

## [0.1.0] - 2026-06-21

### Added
- Local-only OTLP/HTTP receiver (port 4318) for Claude Code and OpenCode telemetry.
- SQLite store (WAL mode) with cost computation from bundled/fetchable pricing.
- 7-view Bubble Tea TUI: Overview, Cost, Tools, Models, Projects, Live, Export.
- `hcli` embedded mode and `collect` / `tui` / `status` / `stop` / `logs` daemon commands.
- `hcli setup claude` and `hcli setup opencode` agent configuration.
- `hcli query` read-only SQL access and `hcli export` (CSV/JSON/Markdown).
- `hcli pricing` list/import/fetch.
- Distribution via GoReleaser: binaries, `.deb`, `.rpm`, shell installer, `go install`.

[Unreleased]: https://github.com/justinmaks/hedge-local/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/justinmaks/hedge-local/releases/tag/v0.1.0
