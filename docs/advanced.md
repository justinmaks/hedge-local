# Advanced usage

## Per-project attribution

The agents don't report which directory you're working in, so by default all
sessions land under `(ungrouped)` in the Projects view. To group by project, wrap
your agent so each run tags its working directory. Add to your shell rc
(`~/.bashrc` or `~/.zshrc`):

```sh
claude()   { OTEL_RESOURCE_ATTRIBUTES="hcli.project_path=$PWD" command claude "$@"; }
opencode() { OPENCODE_RESOURCE_ATTRIBUTES="hcli.project_path=$PWD" command opencode "$@"; }
```

Each wrapper runs the real binary (via `command`) but sets `hcli.project_path` to
your current directory, so hcli groups sessions by repo.

## SQL query (power users)

Run read-only SQL against the local database:

```sh
hcli query "SELECT agent, SUM(total_cost_usd) FROM sessions GROUP BY agent"
```
