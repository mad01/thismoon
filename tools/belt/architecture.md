# belt architecture

## Overview

belt is a tool that runs as a Claude Code PreToolUse hook. Claude Code invokes
`belt hook <event>` with the tool-call payload on stdin; belt runs the guards
registered for that event and, when one fires, writes a deny decision as JSON
on stdout. Each invocation inspects exactly one tool call and exits. belt keeps
no state between invocations, registers no hooks itself, and enables no guards
by default; hook registration and the config overlay live in the consuming
repo's companion recipe (docs/adr/0006).

## Structure

```
cmd/belt/           entry point, hands off to internal/cli
internal/cli/       cobra commands: hook <event>, check, version
internal/hook/      PreToolUse payload parsing and deny JSON emission
internal/guard/     the Guard interface, the ForEvent registry, and the three
                    guards (git-push-main, script-deny-list, write-internal-names)
internal/config/    config.Load(): reads the belt, ralph, suspenders, and
                    Claude-settings surfaces into one Config
internal/notify/    best-effort event emission to the local events service
```

Each guard lives in its own file under `internal/guard/` and implements the
`Guard` interface; `ForEvent` returns the enabled guards for an event in fixed
order.

## Data flow

The hook path: Claude Code pipes the PreToolUse payload into `belt hook
bash|write`. `hook.Run` decodes the JSON, and `toInput` maps the tool-specific
fields onto a shared `guard.Input` (bash carries `command`; Write carries
`file_path` plus `content`, falling back to Edit's `new_string`). `guard.Run`
loads config via `config.Load()` and walks the guards from `ForEvent`; the
first denial wins. On a deny, `notify.EmitEvent` POSTs a warn event
(synchronous, one-second timeout, dropped if the events service is down), then
the decision is encoded as `hookSpecificOutput.permissionDecision: "deny"` on
stdout. belt always exits 0 (the deny is data, not an exit code), and a
malformed payload or an allow produces no output at all.

The check path: `belt check <event> [command]` builds the same `guard.Input`
from argv instead of stdin and prints one verdict line per guard, so a guard
decision can be inspected without a live Claude Code session.

## Storage

Nothing is stored on disk. belt reads four config surfaces, every one
optional (`config.Load()` never errors; a missing file yields zero values):

- `~/.config/belt/config.yaml`: per-guard toggles, exclude paths, extra patterns (legacy `config.toml` read when the YAML file is absent)
- `~/.config/ralph/config.local.toml`: the machine profile for git-push-main
- `~/.config/suspenders/config.yaml`: the guard section write-internal-names shares with the pre-commit guard
- `~/.claude/settings.json` + `settings.local.json`: the `permissions.deny` Bash entries for script-deny-list

Denials are recorded remotely: a POST to the local events service
(`http://127.0.0.1:7430/api/events`, overridable via `EVENTS_BASE_URL`).
Persisting them is that service's job, not belt's.

## Interfaces

CLI commands: `belt hook bash|write` (the hook entrypoint), `belt check bash
"<command>"` and `belt check write --file <path> --content <text>` (dry runs),
`belt doctor` (resolved config, guard/hint state, and the blocked-name set),
`belt config` (config locations + annotated setting reference), `belt version`.

Hook contract: PreToolUse payload on stdin, deny JSON on stdout, exit 0 in
every case. Config surfaces are read-only, listed above. There is no web or
MCP surface.
