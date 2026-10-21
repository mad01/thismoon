# agent-sync architecture

## Overview

agent-sync is a tool: one Go program installed to `~/code/bin/agent-sync`.
Each run reads the config file, computes one section's change, prints a
report and exits. Nothing stays running, and the only state it keeps is
the mcp section's stamp file.

## Structure

```
cmd/agent-sync/      entrypoint: os.Exit(cli.Main(os.Args[1:], ...))
internal/cli/        cobra tree; one sync|plan|check family per section
internal/apply/      modes, exit codes, Streams, the error printer
internal/config/     config file location, top-level parse, Section helpers
internal/pyjson/     ordered JSON with Python's parse, equality and dumps,
                     bool(), str() and repr() of the values, str methods
internal/pypath/     Python's path handling: pathlib, expanduser, realpath, mkdir
internal/gate/       this machine's host and profiles, the hosts/profiles gates
internal/claudecode/ the claude CLI lookup and Claude Code's config directory
internal/settings/   settings section: config, merge, report, sync, doctor
internal/mods/       mods section: config, desired, gate, state, plan, doctor
internal/mcp/        mcp section: config, servers, entry, claude, doctor
```

The section packages depend on `apply`, `config`, `pyjson`, `pypath`,
`gate` and `claudecode`, and never on each other. `cli` is the only package that knows
every section.

## Data flow

`agent-sync <section> <mode>` resolves the config path, parses the file
into three sections, and passes the named one to that section's
`ParseConfig`, which checks its keys and resolves its paths. Then:

- settings reads the template, the live file and the overlay with
  `pyjson.ReadFile`. It prunes the retired keys from a copy of live, merges
  the template and then the overlay into it, and diffs the original live
  file against the result. Sync writes the merge as `json.dumps(indent=2)`
  plus a newline.
- mods reads the mods files, each marketplace's manifest and Claude Code's
  plugin state, plans the `claude plugin` calls that close the gap, and in
  sync runs them in order.
- mcp merges the servers files into the desired set and asks `claude mcp
  list` what is registered. It also asks `claude mcp get` per server,
  unless the stamp matches and neither `--force` nor `prune` is set. In
  sync it runs `claude mcp add` and `remove`, then writes the stamp.

Check prints a summary line and exits 1 on any change. Plan prints the
change and writes nothing.

## Storage

It reads the config file and the data files the config names. settings
writes only the live settings file, mods writes nothing itself, and mcp
writes its stamp (`~/.cache/claude-mcp/sync.hash` by default). Everything
else changes through the `claude` CLI.

## Interfaces

CLI: `settings|mods|mcp sync|plan|check` (`--force` on mcp sync and
check), `doctor`, `docs`, and `version` (bare token, or the shared
four-key build metadata object with `-o json`). Exit codes: 0 ok, 1 drift
or error, 2 a failed `claude` command. The `check` modes' first stdout
line is the summary `ralph doctor` shows.
