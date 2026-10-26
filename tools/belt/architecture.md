# belt architecture

## Overview

belt is a tool that runs as Claude Code hooks. Claude Code invokes
`belt hook <event>` (PreToolUse), `belt hint <event>` (PostToolUse,
SessionStart, SubagentStart, UserPromptSubmit) or `belt record <event>` (the
subagent, task, teammate, permission-denied, notification, compaction,
session-end and stop-failure lifecycle hooks) with the event payload on
stdin. belt runs the guards or hints registered for that event and writes its
decision or advice as JSON on stdout, or posts a ledger entry and writes
nothing. For the guards, it writes nothing when everything is allowed, which
is the common case. Each invocation inspects exactly one event and exits.
`belt watch` is the one longer-lived command: an asyncRewake hook that polls
for up to its max wait and exits 2 to wake an idle session. belt keeps no
daemon, registers no hooks itself, and ships no config of its own; hook
registration and the config overlay live in the consuming repo's companion
recipe (docs/adr/0006).

The three concepts are asymmetric by design (docs/adr/0008,
docs/adr/0024). A guard can return `permissionDecision: deny` and stop a
tool call. The `Hint` interface has no denial path at all, and the record
entrypoint takes no output writer. A hint returns advice text or nil; a
record posts an entry and prints nothing.

## Structure

```
cmd/belt/           entry point, hands off to internal/cli
internal/cli/       cobra commands: hook <event>, hint <event>,
                    record <event>, watch, check, doctor, config, override,
                    docs, version; Execute maps a watcher's wake to exit 2
internal/hook/      payload parsing + output emission per event: deny JSON,
                    additionalContext JSON, plain stdout for prompt, nothing
                    for record; the watch entrypoint
internal/ledger/    the record events and the pure entry builder: title,
                    level, 500-byte message, session/agent/repo/worktree tags,
                    model and effort from ledger.agent_types
internal/watch/     the inbox poller: one GET client, the to-tag filter,
                    the per-session cursor and owner files
internal/cache/     per-session file names under ~/.cache/belt with the
                    session id reduced to path-safe characters
internal/guard/     the Guard interface, the ForEvent registry, the built-in
                    guards (git-push-main, git-discard, git-identity, commit-guard,
                    script-deny-list, write-internal-names,
                    publish-internal-names), and the Custom guard that execs
                    config-registered external commands
internal/hint/      the Hint interface and the hints (prefer-csl,
                    read-size, commit-policy, lint-policy, kof-assertions, kof-consult,
                    kof-deposit, agent-memory, humanizer-check, em-dash), the Custom
                    hint that execs config-registered external commands,
                    plus csl shard lookup, search-response parsing, and the
                    per-session seen store
internal/config/    config.Load(): the belt config plus the gated
                    Claude-settings deny list, into one Config
internal/mcptool/   MCP tool-name reading (server/operation split, the
                    read-only verb rule) shared by the humanizer hint and
                    the publish-internal-names guard
internal/notify/    best-effort event emission to the local events service
```

Each guard and hint lives in its own file and implements its interface;
`ForEvent` returns the enabled ones for an event in fixed order (built-ins
first, then the custom guards or custom hints alphabetically by name).

## Data flow

The guard path: Claude Code pipes the PreToolUse payload into `belt hook
bash|write`. `hook.Run` decodes the JSON with `decodePayload`, which
fails on stdin that is not a JSON object and on a field the guards read
arriving with another JSON type. That failure is a `belt[payload]` deny
plus a warn event tagged `decode_error`, before the config loads (the rule
is in operating.md, failure modes).

A payload that decodes goes to `toInput`, which maps the
tool-specific fields onto a shared `guard.Input` (bash carries `command`;
Write carries `file_path` plus `content`, falling back to Edit's
`new_string`). `guard.Run` loads config via `config.Load()` and walks the
guards from `ForEvent`; the first denial wins. On a deny, `notify.EmitEvent`
POSTs a warn event (synchronous, one-second timeout, dropped if the events
service is down), then the decision is encoded as
`hookSpecificOutput.permissionDecision: "deny"` on stdout. The guard path
always exits 0 (the deny is data, not an exit code), and an allow produces
no output at all. Across belt the only non-zero exit on
purpose is `belt watch`'s exit 2 (see the watch path below).

The hint path mirrors it: `belt hint search|bash|read|external-text|
session-start|prompt` maps the payload onto a `hint.Input` (tool name, request, response,
session id, transcript path, cwd) and runs every enabled hint for the event.
Hints do not short-circuit, each speaks or stays silent independently. Advice
is emitted as `hookSpecificOutput.additionalContext` with the invoking
event's name, except the prompt event, which writes plain stdout text because
UserPromptSubmit adds stdout to context directly. Every fired hint POSTs an
info event, so hint volume is measurable from the events timeline. A
payload that fails the same decode runs no hint, writes nothing, and POSTs
a warn event tagged `decode_error`.

Both paths carry who made the call: the session id on every real call, and
the agent id and type inside a subagent. They reach custom guards and custom
hints on stdin and tag every deny and hint event. The session and agent
fields decode leniently, because the guard path denies a payload it cannot
decode, and a field no guard reads must not decide a call.

The record path: `belt record <event>` decodes the lifecycle payload. Its
lifecycle-only fields sit in a separate struct, out of the guard decode's
reach. It places the working directory in git with `gitloc.LocationAt` under
a 1 s budget, builds the entry with `ledger.Build`, and posts it with a
300 ms budget. A deferred recover keeps a panic from exiting 2. The config decides
only `ledger.enabled`, `ledger.source`, and the model and effort tags
`ledger.agent_types` adds to a subagent-start entry; a broken config records
with the defaults.

The watch path: `belt watch` reads the session id and, when `watch.enabled`
is set, `watch.Run` claims the session's owner file. It polls the events
service until items addressed to the session arrive, the max wait passes,
or a newer watcher claims the session. On a hit it moves the cursor, then
the CLI prints the items on stderr and exits 2.

The check path: `belt check <event> [command]` builds the same `guard.Input`
from argv instead of stdin. It prints one verdict line per guard, so a guard
decision can be inspected without a live Claude Code session.

## Storage

belt writes per-session files under `~/.cache/belt`, named through
`internal/cache` so a malformed session id cannot escape the directory. The
seen markers, `seen-<session-id>` with one hint id per line, are how the
once-per-session hints (kof-deposit, humanizer-check) and the per-session
dedupe (kof-assertions, kof-consult) remember what a session already saw.
Every failure path around that store degrades to "not seen". The watcher
keeps `watch-<session-id>`, the id of the newest item it delivered, and
`watch-<session-id>.pid`, the owner token of the newest watcher. Both are
replaced through a rename. A cursor that cannot be written means no
delivery, never a repeated one.

Everything else is read-only. Config comes from these surfaces. A missing
file yields zero values. A present file that fails to parse, or a listed
include that is missing or broken, is an error `config.Load()` returns and
every hook denies on:

- `~/.config/belt/config.yaml`: per-guard toggles, exclude paths, extra patterns, `claude_settings`, guard rules, the `internal_names` section, and the two purpose-named repo lists `direct_main_repos` and `public_repos` (legacy `config.toml` read when the YAML file is absent)
- the shared names files `internal_names.include` lists (`~`-expanded paths belt's own config names, docs/adr/0016): `blocked_words`, `allowlist`, and `allow_phrases` only, decoded strictly so a stray key is an error. The lists are appended to the section's own, and a listed file that is missing denies exactly like a broken config.yaml
- `~/.claude/settings.json` + `settings.local.json`: the `permissions.deny` Bash entries for script-deny-list, read live on every invocation unless `claude_settings.enabled: false` turns the read off

No other tool's config is read. Neither the suspenders config nor the
ralph machine config is a fallback for anything, and belt has no
machine-profile concept (docs/adr/0010). The provisioning layer renders one
belt config per machine class.

The hints also read the world they advise about: the csl shard listing in
`~/.config/csl/search-index/` (prefer-csl and read-size, no csl process launched) and the
agent-memory index files in `~/.config/agent-memory[-work]/` (agent-memory).
They read the session transcript file named in the payload (kof-deposit,
humanizer-check) and the git repo a commit ran in. From that repo they read
the branch, origin remote, and an optional repo-root `.belt.yaml` overlay,
via git execs (commit-policy; overlay is hints-only and never denies,
docs/adr/0012). They also read the local kof serve API on `KOF_PORT` with a
400 ms budget (kof-consult, kof-assertions). A custom hint execs the command
its `custom_hints` entry names, with the same 400 ms default budget
(`timeout_ms` overrides it), and relays the command's stdout.

Denials, hints and ledger entries are recorded remotely: a POST to the
local events service (`http://127.0.0.1:7430/api/events`, overridable via
`EVENTS_BASE_URL`), denials and hints under `belt`, ledger entries under
`belt-ledger`. The watcher reads the same service with a GET on
`/api/events`. Persisting them is that service's job, not belt's.

## Interfaces

The hook entrypoints are `belt hook bash|write|external-text`, `belt hint
search|bash|external-text|session-start|subagent-start|prompt`,
`belt record <lifecycle event>` and `belt watch`. The other CLI commands are
`belt check bash "<command>"` and `belt check write --file <path> --content
<text>` (dry runs), and `belt doctor` (installed build, resolved config,
guard/hint state, custom guard and custom hint reachability, active
overrides, kof reachability, and the blocked-name set). The CLI also has `belt override`
(list/set/clear the named switches commit-guard rules honor) and `belt config`
(config locations + the settings in effect; the annotated setting reference
is in its `--help`). The remaining two are `belt docs` (the embedded operating
doc) and `belt version [-o json]` (bare version token, or the four-key build
metadata object shared across the repo's components).

Hook contract: event payload on stdin, decision or advice on stdout
(nothing at all for record), exit 0 in every case but one. `belt watch`
exits 2 with the items on stderr when it wakes a session. Config surfaces are read-only,
listed above. There is no web
or MCP surface. The full per-hook behavior reference, including the
settings.json wiring, is [docs/hooks.md](docs/hooks.md).
