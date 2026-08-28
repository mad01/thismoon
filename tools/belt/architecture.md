# belt architecture

## Overview

belt is a tool that runs as Claude Code hooks. Claude Code invokes
`belt hook <event>` (PreToolUse) or `belt hint <event>` (PostToolUse,
SessionStart, UserPromptSubmit) with the event payload on stdin; belt runs
the guards or hints registered for that event and writes its decision or
advice as JSON on stdout — or, for the guards, nothing when everything is
allowed, which is the common case. Each invocation inspects exactly one event
and exits. belt keeps no daemon, registers no hooks itself, and ships no
config of its own; hook registration and the config overlay live in the
consuming repo's companion recipe (docs/adr/0006).

The two halves are asymmetric by design (docs/adr/0008): a guard can return
`permissionDecision: deny` and stop a tool call, while the `Hint` interface
has no denial path at all — a hint returns advice text or nil.

## Structure

```
cmd/belt/           entry point, hands off to internal/cli
internal/cli/       cobra commands: hook <event>, hint <event>, check,
                    doctor, config, override, docs, version
internal/hook/      payload parsing + output emission per event: deny JSON,
                    additionalContext JSON, or plain stdout for prompt
internal/guard/     the Guard interface, the ForEvent registry, the built-in
                    guards (git-push-main, git-identity, commit-guard,
                    script-deny-list, write-internal-names), and the Custom
                    guard that execs config-registered external commands
internal/hint/      the Hint interface and the hints (prefer-csl,
                    kof-assertions, kof-consult, kof-deposit, agent-memory,
                    humanizer-check), plus csl shard lookup, search-response
                    parsing, and the per-session seen store
internal/config/    config.Load(): the belt config plus its ralph profiles
                    fallback and the gated Claude-settings deny list, into one Config
internal/notify/    best-effort event emission to the local events service
```

Each guard and hint lives in its own file and implements its interface;
`ForEvent` returns the enabled ones for an event in fixed order (built-in
guards first, custom guards alphabetically).

## Data flow

The guard path: Claude Code pipes the PreToolUse payload into `belt hook
bash|write`. `hook.Run` decodes the JSON, and `toInput` maps the
tool-specific fields onto a shared `guard.Input` (bash carries `command`;
Write carries `file_path` plus `content`, falling back to Edit's
`new_string`). `guard.Run` loads config via `config.Load()` and walks the
guards from `ForEvent`; the first denial wins. On a deny, `notify.EmitEvent`
POSTs a warn event (synchronous, one-second timeout, dropped if the events
service is down), then the decision is encoded as
`hookSpecificOutput.permissionDecision: "deny"` on stdout. belt always exits
0 (the deny is data, not an exit code), and a malformed payload or an allow
produces no output at all.

The hint path mirrors it: `belt hint search|bash|external-text|session-start|
prompt` maps the payload onto a `hint.Input` (tool name, request, response,
session id, transcript path, cwd) and runs every enabled hint for the event —
hints do not short-circuit, each speaks or stays silent independently. Advice
is emitted as `hookSpecificOutput.additionalContext` with the invoking
event's name, except the prompt event, which writes plain stdout text because
UserPromptSubmit adds stdout to context directly. Every fired hint POSTs an
info event, so hint volume is measurable from the events timeline.

The check path: `belt check <event> [command]` builds the same `guard.Input`
from argv instead of stdin and prints one verdict line per guard, so a guard
decision can be inspected without a live Claude Code session.

## Storage

belt writes one thing: per-session seen markers under
`~/.cache/belt/seen-<session-id>`, one hint id per line, which is how the
once-per-session hints (kof-deposit, humanizer-check) and the per-session
dedupe (kof-assertions, kof-consult) remember what a session already saw.
Every failure path around that store degrades to "not seen".

Everything else is read-only. Config comes from four surfaces, every one
optional (`config.Load()` never errors; a missing file yields zero values):

- `~/.config/belt/config.yaml`: per-guard toggles, exclude paths, extra patterns, `profiles`, `claude_settings`, and the `internal_names` section (legacy `config.toml` read when the YAML file is absent)
- `~/.config/ralph/config.local.toml`: profiles fallback when the belt config sets none
- `~/.claude/settings.json` + `settings.local.json`: the `permissions.deny` Bash entries for script-deny-list, read live on every invocation unless `claude_settings.enabled: false` turns the read off

No other tool's config is read — the suspenders config in particular is not
a fallback for anything (docs/adr/0010).

The hints also read the world they advise about: the csl shard
listing in `~/.config/csl/search-index/` (prefer-csl, no csl process
launched), the agent-memory index files in `~/.config/agent-memory[-work]/`
(agent-memory), the session transcript file named in the payload (kof-deposit,
humanizer-check), and the local kof serve API on `KOF_PORT` with a 400 ms
budget (kof-consult, kof-assertions).

Denials and hints are recorded remotely: a POST to the local events service
(`http://127.0.0.1:7430/api/events`, overridable via `EVENTS_BASE_URL`).
Persisting them is that service's job, not belt's.

## Interfaces

CLI commands: `belt hook bash|write` and `belt hint
search|bash|external-text|session-start|prompt` (the hook entrypoints),
`belt check bash "<command>"` and `belt check write --file <path> --content
<text>` (dry runs), `belt doctor` (installed build, resolved config,
guard/hint state, custom-guard reachability, active overrides, kof
reachability, and the blocked-name set), `belt override` (list/set/clear the
named switches commit-guard rules honor), `belt config` (config locations +
the settings in effect; the annotated setting reference is in its `--help`),
`belt docs` (the embedded operating doc), and `belt version [-o json]` (bare
version token, or the four-key build metadata object shared across the
repo's components).

Hook contract: event payload on stdin, decision or advice on stdout, exit 0
in every case. Config surfaces are read-only, listed above. There is no web
or MCP surface. The full per-hook behavior reference, including the
settings.json wiring, is [docs/hooks.md](docs/hooks.md).
