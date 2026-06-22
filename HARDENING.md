# Hardening Backlog

Security hardening items identified during pre-release review. These are
**defense-in-depth** improvements, not known exploited vulnerabilities — hcli is
a local-only tool whose receiver binds to `127.0.0.1`. Each item below is to be
implemented **with tests** in a dedicated hardening commit.

Status legend: ☐ todo · ☑ done

---

## 1. Restrict permissions on local data (`~/.hedge/`)

☑ **done** — `~/.hedge` and the SQLite DB (+ WAL/SHM) are now owner-only
(`0700`/`0600`) via `mkdirSecure`/`openSecureAppendLog` (`internal/cli/perms.go`)
and `store.New`. Existing 0755 dirs are downgraded on next run.

The config/data directory is created `0755` and files are written `0644`. With
`hcli collect --with-logs`, the SQLite database can contain prompt and log
content, which is sensitive. On a shared host any local user can read it.

- Create `~/.hedge/` as `0700` instead of `0755`.
  - `internal/store/store.go:18` (`os.MkdirAll(dir, 0755)`)
  - `internal/cli/setup_claude.go` (`MkdirAll(hedgeDir, 0755)`)
  - `internal/cli/setup_opencode.go` (`MkdirAll` for config/env dirs)
  - `internal/cli/daemon.go:33` (`writePIDFile` dir)
  - `internal/cli/collect.go` (log dir)
- Write the database, `daemon.log`, and the WAL/SHM sidecar files `0600`.
  - `internal/cli/collect.go:97` (`daemon.log`, currently `0644`)
  - `internal/cli/daemon.go:99` (`daemon.log`, currently `0644`)
  - SQLite file mode (set umask or chmod after open).
- The generated `env.sh` / `opencode-env.sh` contain no secrets, so `0644` is
  acceptable, but `0600` is harmless and consistent.

**Test:** after `setup` / `collect`, assert directory mode is `0700` and
sensitive files are `0600` (Unix only; skip on Windows).

gosec rules: G301 (dir perms), G306 (file perms).

---

## 2. Bound the OTLP receiver request body and add server timeouts

☐ **todo**

The receive handlers read the entire request body with `io.ReadAll(req.Body)`
and no size cap (`internal/collect/receiver.go:79,110,141`). The `http.Server`
also has no timeouts (`receiver.go:46`). The listener is localhost-only and fed
by trusted local agents, so risk is low, but a malformed/oversized body could
exhaust memory and a slow client could hold a connection open.

- Wrap each body read with `http.MaxBytesReader(w, req.Body, maxOTLPBodyBytes)`
  (suggest 16 MiB, matching the pricing-fetch `LimitReader`).
- Set `ReadHeaderTimeout` (and ideally `ReadTimeout` / `WriteTimeout`) on the
  `http.Server`.

**Test:** post a body over the limit and assert a `4xx` / no OOM; assert the
server has a non-zero `ReadHeaderTimeout`.

gosec rules: G112 (missing `ReadHeaderTimeout`), G114-adjacent.

---

## 3. Open a read-only connection for `hcli query`

☐ **todo**

`hcli query` enforces read-only access by checking that the SQL starts with
`SELECT` or `WITH` (`internal/cli/query.go:27`), but the underlying database is
opened read-write (`internal/store/store.go:21`). `database/sql` does not
execute stacked statements, so this is low-risk, but a crafted query could still
reach writable PRAGMAs.

- Provide a read-only store/connection for the `query` path, e.g. open with
  `&_pragma=query_only(true)` (and/or `mode=ro`).
- Keep the existing prefix check as a fast user-facing guard.

**Test:** assert a write attempt (e.g. `PRAGMA user_version = 1` via a crafted
`WITH`/`SELECT`-prefixed payload, or a direct write) fails on the query
connection.

---

## 4. Fold gosec into golangci-lint

☐ **todo**

Rather than a standalone gosec CI job (noisy, duplicates tooling), enable the
`gosec` linter inside `.golangci.yml` once items 1–3 above are fixed, so the
findings above stay fixed. Scope to meaningful rules and exclude `_test.go` as
appropriate.

Note: `govulncheck` already runs as a blocking CI job and is the primary
vulnerability gate. The `golang.org/x/net` advisory `GO-2026-4559` it caught was
resolved by upgrading to `v0.51.0`.
