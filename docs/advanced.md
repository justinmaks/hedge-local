# Advanced usage

## Per-project attribution

The agents' telemetry doesn't report which directory you're working in, so
without extra help all sessions land under `(ungrouped)` in the Projects view.

### Claude Code (automatic)

`hcli setup claude` installs a SessionStart hook into `~/.claude/settings.json`:

```json
{
  "hooks": {
    "SessionStart": [
      {"hooks": [{"type": "command", "command": "hcli session-start"}]}
    ]
  }
}
```

Claude Code runs the hook at the start of every session and passes the
session ID and working directory on stdin; `hcli session-start` records the
attribution directly in the database. No wrapper needed, and it works however
you launch `claude`. The install is a careful merge: existing settings and
hooks are preserved, the file is backed up first, and a rerun is a no-op.

### OpenCode (shell wrapper)

OpenCode has no equivalent hook, so wrap it in your shell rc
(`~/.bashrc` or `~/.zshrc`):

```sh
opencode() { OPENCODE_RESOURCE_ATTRIBUTES="hcli.project_path=$PWD" command opencode "$@"; }
```

The wrapper runs the real binary (via `command`) but sets `hcli.project_path`
to your current directory. The same mechanism also still works for Claude Code
(`claude() { OTEL_RESOURCE_ATTRIBUTES="hcli.project_path=$PWD" command claude "$@"; }`)
if you prefer it over the hook.

## SQL query (power users)

Run read-only SQL against the local database:

```sh
hcli query "SELECT agent, SUM(total_cost_usd) FROM sessions GROUP BY agent"
```
