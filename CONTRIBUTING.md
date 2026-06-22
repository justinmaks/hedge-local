# Contributing to hcli

## Build from Source

```sh
git clone https://github.com/justinmaks/hedge-local.git
cd hedge-local
go build -o /tmp/hcli ./cmd/hcli
go test ./...
```

Requires Go 1.25+.

## Architecture

hcli is a single Go binary with two modes: `hcli collect` (OTLP receiver) and `hcli tui` (terminal UI). Both link the same SQLite reader/writer code.

### Internal packages

| Package | Responsibility |
|---------|---------------|
| `internal/cli` | Cobra commands (collect, tui, export, setup, pricing, query, status, stop, logs, version) |
| `internal/config` | TOML config loading with defaults |
| `internal/store` | SQLite schema, migrations, CRUD, pricing |
| `internal/collect` | OTLP/HTTP receiver + writer |
| `internal/normalize` | Per-agent normalizers (Claude Code, OpenCode, composite) |
| `internal/tui` | Bubble Tea root model, theme, table, tab bar, status line, date filter |
| `internal/tui/queries` | Read-only store queries backing all TUI views |
| `internal/tui/views` | 7 view implementations (Overview, Cost, Tools, Models, Projects, Live, Export) |
| `internal/tui/export_writer` | Shared CSV/JSON/Markdown format writers |

### Data flow

```
Agent → OTLP/HTTP → Receiver → Normalizer → Writer → SQLite → TUI views
```

## Development Workflow

- **TDD strictly**: write failing test → run (fail) → implement → run (pass) → commit
- **One commit per task**: clear, descriptive commit messages
- **No comments in code** unless explicitly requested
- Run `gofmt`, `go vet`, and `golangci-lint` before committing

## Adding a New Agent

1. Create `internal/normalize/<agent>.go` implementing the `Normalizer` interface:
   - `Agent() string`
   - `NormalizeTraces(*coltracepb.ExportTraceServiceRequest) ([]Event, error)`
   - `NormalizeMetrics(*colmetricspb.ExportMetricsServiceRequest) ([]Event, error)`
   - `NormalizeLogs(*collogspb.ExportLogsServiceRequest) ([]Event, error)`
2. Add the new normalizer to `normalize.NewCompositeNormalizer()` in `internal/cli/collect.go`.
3. Create `hcli setup <agent>` command in `internal/cli/`.
4. Add tests for the normalizer and an integration test for the full pipeline.
5. Only recognize spans/logs/metrics that belong to your agent — ignore everything else to avoid duplicate rows.

## Adding a New TUI View

1. Add query methods to `internal/tui/queries/queries.go` with typed return structs.
2. Create `internal/tui/views/<view>.go` implementing the `tui.View` interface:
   - `Title() string`
   - `Init() tea.Cmd`
   - `Update(msg tea.Msg, ctx tui.ViewContext) (tui.View, tea.Cmd)`
   - `Render(width, height int, theme *tui.Theme) string`
   - `Hints() string`
3. If the view is date-aware, also implement `ReloadableView`:
   - `Reload(ctx tui.ViewContext) tea.Cmd`
4. Register the view in `internal/cli/tui.go` via `tui.RegisterViewFactory()`.
5. Add tests for query methods and view behavior.

## Testing

```sh
go test ./...              # all tests
go test ./internal/tui/... # TUI only
go test -v -count=1 ./...  # verbose, no cache
```

Tests use temp SQLite databases per test. No external services required.

## Releasing

Releases are tag-triggered:

```sh
git tag v0.1.0
git push origin v0.1.0
```

This triggers the GitHub Actions release workflow, which runs GoReleaser to build binaries, archives, .deb, .rpm, and install.sh for all platforms. Artifacts are published to GitHub Releases.

To test GoReleaser locally without a tag:

```sh
goreleaser --snapshot --clean
```

## Code Style

- `gofmt` — formatting
- `go vet` — basic checks
- `golangci-lint` — linting (errcheck, govet, staticcheck, ineffassign, unused)
- No code comments unless explicitly requested
- Commit messages: `type: description` (feat, fix, test, docs, chore)
