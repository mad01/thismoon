# operating belt

belt is the Claude Code hook binary: guards inspect a tool call before it
runs and deny risky ones with a reason. Hints inspect a tool call after it
ran and add advisory context beside the result. Ledger entries record
session lifecycle events (subagents, tasks, permission denials,
notifications, compaction, session end, failed turns) to the events
service. The three are separate by design. A hint has no denial path in its
interface, and the record path has no output at all, so neither can block
or steer anything, even by mistake. A fourth concept, the allow, rides the
guard hook: once every guard passed, an enabled allow may answer allow so
the call skips the permission prompt. It has no denial path either, and
every allow is off until the config turns it on.

## how it runs

There is no daemon. Claude Code spawns `belt hook <event>`,
`belt hint <event>` or `belt record <event>` as a fresh process for every
event it is wired to, feeds the payload on stdin, and reads the decision or
advice on stdout. The output is JSON for every hint event; `record` writes
nothing. Each exits 0 even when it
denies: the deny travels in the JSON, never the exit code, so a belt bug
cannot break tool calls.

`belt watch` is the exception, and it is off until `watch.enabled` is set.
It runs as an asyncRewake hook on Stop and SessionStart and polls the
events service for items whose `to` tag is the session id. When some
arrive it exits 2 with them on stderr, which wakes the idle session. Exit 2 blocks nothing there,
because the turn has already ended. Every other outcome is exit 0, silent.

Because every invocation is a fresh process, changes apply on the next tool
call. belt config edits, Claude settings deny-list edits, hook registration
changes in the Claude settings, and a newly installed binary all take effect
immediately, with no restart and no new session.

## where config lives

The belt-owned config is {{.StorePath}}: guard and hint toggles, exclude
paths, extra patterns, claude_settings, guard rules, and the internal_names
section, with the shared names files it lists under internal_names.include.
The XDG path `$XDG_CONFIG_HOME/belt/config.yaml` wins when that variable is
set, and `--config` or `$BELT_CONFIG` relocates the file outright (a relocated
file gets no legacy TOML fallback). The config is standalone and has no
machine-profile concept: belt reads no other tool's file, and the
provisioning layer renders one config per machine class. A guard meant for
only some machines is absent or disabled in the others' files. The Bash deny
patterns are read live from the Claude settings files, so the script guard
and the permission system share one deny list. claude_settings.enabled:
false turns that read off, leaving the guard with extra_patterns only.

The config file is optional, but a broken one is not tolerated. No file at
all means the built-in defaults. A file that is present and fails to parse
or validate makes every `belt hook` invocation deny with
`belt[config]: belt cannot read its config …`. belt cannot tell "no rules
configured" from "the rules did not load". Guessing the permissive one
would silently disarm every guard.

Validation covers the values that parse and then do nothing:

- a `custom_guards.<name>.event` outside bash/write;
- a `custom_hints.<name>` entry with an event other than session-start or
  subagent-start, an empty command, or a built-in hint's name;
- an `agent_types` list on a custom hint that is empty, not on
  subagent-start, or holds an entry with a `*` before its end;
- `mode:` values other than hard/soft, or set on a guard that ignores them.

A names file listed under internal_names.include that is missing or fails
to parse is the same error, and the deny names the include file. The
broken config does not reach the ledger or the watcher: `record` posts with
the defaults, and `watch` stays off. No allow runs either, since the deny
comes first. The `ledger`, `watch` and `allows` values never fail
validation at all; an `allows.<id>.roots` entry that is not absolute or
`~/`, does not resolve, or covers home is dropped with a doctor warning. A bad number is clamped and a source is rewritten
the way the events service stores it, and doctor warns about each. A
malformed `ledger.agent_types` entry or value is dropped with a doctor
warning, even one of the wrong YAML type.

Rollout order matters for config that names something new. A binary that
predates subagent-start rejects `custom_hints.<name>.event: subagent-start`
as an unknown event, and that config error makes it deny every guarded tool
call. Build and install the new belt (a `ralph up` on the fleet) before any
machine config names the new event. A new key fails the other way: an older
belt ignores `custom_hints.<name>.agent_types` and runs the hint for every
subagent, and ignores `ledger.agent_types` and records no model or effort.
A belt that knows `agent_types` but not `*` matching reads a `*` entry
literally, so a gated brief reaches no subagent until the new belt is
installed. The same order applies.

## failure modes

A deny reason is prefixed `belt[<guard-id>]:`, so a block always names the
guard that fired. Treat it as a true positive first. Do not retry the same
call unchanged, and do not work around the block by writing the command to a
script or switching tools. If the deny looks wrong, surface it and let the
user decide.

Known false-positive path for `script-deny-list`: an inline script matches
as text, the shell after it included. So a heredoc or `-c` string that only
writes or prints a denied command's name is denied. The one exception is a
Python heredoc that is the whole command and runs as `python3 -I`. It may
import only json, re, pathlib, textwrap, datetime or collections, and may
not name os, sys, exec, eval, getattr, vars or a `_`-prefixed name anywhere.
The deny reason for a Python heredoc names that shape. A `cat` or `tee`
heredoc written to a document file (`.md`, `.txt` and a few more) is text,
and its lines are not read as commands. The file may not sit in a dot
directory or a system directory, and JSON, YAML and TOML files are never
text. The temp folders `/tmp`, `/var/tmp` and `~/.tmp` are allowed, with the
same rules below them. Every other step must be one that cannot run the file (`mkdir`,
`git add`, `gh pr create` and a few more), with no redirect, and every
`cd` must be followed by `&&`.
After a `bash -c` or an interpreter heredoc in the same command it still
matches, because that inline script scans to the end of the command.

Known false-positive path for `write-internal-names`: exclude_paths entries
usually name the canonical checkout, and a git worktree at another path
misses the match. So a write that is fine in the canonical checkout gets
denied in the worktree. Make the edit in the canonical checkout, or have the
worktree path added to the guard's exclude_paths.

A hook payload belt cannot read fails closed, for the same reason as a
broken config: no guard can check a call it cannot see. `belt hook` denies
with `belt[payload]:` before any guard runs when stdin holds no JSON object
(empty, not JSON, or another JSON value). It also denies when a field the
guards read arrives with another JSON type. Those fields are `tool_name`,
`tool_input` and `cwd`, plus `command` inside `tool_input` on bash and
`file_path`, `content` and `new_string` on write. A JSON null counts as
absent. The session and agent fields (`session_id`, `transcript_path`,
`agent_id`, `agent_type`) and every key no guard reads decode leniently, so
a mistyped identity never denies.

Each payload deny leaves a warn event
under the `belt` source, tagged with the event and a `decode_error` naming
the field and the type that arrived. `belt hint` runs no hint on such a
payload and writes nothing, leaving the same warn event. `belt record`
still records the event, at warn, naming the decode error (nothing at all
with ledger.enabled false). A mistyped `session_id` or `transcript_path` is
dropped silently from the ledger entry, which stays at info. The harness
builds the payload, not the model, so a retry hits the same deny. A
`belt[payload]` deny on every call of one tool means the harness sends a
shape this belt does not read. Install a belt that reads it, and until then
unwire that `belt hook` entry from the harness settings.

Missing config fails in two directions, deliberately. Guards and hints
default to enabled. `git-push-main` fails closed: with no config it denies
every push to main or master, and only a rendered config that disables it or
allowlists a repo opens the door. With no internal_names section in the belt
config, `write-internal-names` and `publish-internal-names` have an empty
name list and allow every write and publish: those two fail open. They
block only in public-bound repos: the top-level public_repos list when the
config has one (a repo missing from it is not guarded), else every
github.com repo. `belt doctor` reports the mode and the empty-name condition
in its config-surfaces section, and flags a broken config file there too.
When it does, the guard and hint state below that line is the defaults, not
what belt is enforcing.

The rule-driven commit guards fail open across the board: `git-identity`
and `commit-guard` are no-ops without their config sections, skip repos
whose remote does not resolve, and a malformed block_hours window blocks
nothing. Soft-mode rules and broken custom guards allow with a warn event
to the events service — check there when a guard seems silent. A hard
`commit-guard` deny names its override. `belt override set <name> --reason
"..."` (10m default, `--for` sizes it, `extend` pushes it) is the escape
hatch, not rewording the commit command. set/extend archive the reason as
a warn event, a suppressed block leaves one too, and it expires on its own.

`rm-temp` fails open as well. An rm with one target it cannot place
inside a temp folder, a call where one rm of several does not qualify,
and every rm on a machine without `toss-bin` on PATH get no answer, so
the settings ask rule decides them as before. The doctor guard line says
when toss-bin is unreachable.

The `git-c-read` allow fails toward the prompt. When it gives no answer,
the call goes to Claude Code's own permission check, exactly as without
belt. It answers `belt[git-c-read]:` only for a plain Bash call made of
nothing but read-only `git -C <dir> <subcommand>` commands, after a guard
pass with no denial and no warning, in a normal repo whose work tree and
git directory sit inside the session's work tree or an
`allows.git-c-read.roots` entry. A call it should have answered but did
not: check `belt doctor` for the allow's state and roots. Then look for
shell syntax the strict reader refuses (a pipe, a redirect, `$`, a glob,
an unquoted `^`), a dir that resolves through a symlink outside the
roots, a linked worktree whose main checkout is under no root, or a
soft-mode or failing custom guard (its warning is on the events
timeline). An approval posts no event; the hook output in the transcript
is its only record. Per the Claude Code hooks
documentation, a hook allow does not override a settings deny or ask rule,
and whether it skips the auto-mode classifier is not verified
(docs/adr/0032).

The moondb-inflight hint is silent when moondb is down or older than its
in-flight route, and the doctor moondb line says which. read-size never
sees a csl_read whose tool the `belt hint read` matcher leaves out; the
doctor hook wiring section prints a WARNING naming the matcher to set,
without changing the exit status. A custom
hint (custom_hints in the belt config) that fails, runs past its budget, or
prints nothing stays silent and leaves a warn event naming the cause. The
doctor custom hints section says whether its command resolves to something
executable at all.

A memory store missing from a Claude Code session's start context is
usually the `--agent claude-code` skip: Claude Code's autoMemoryDirectory
names that store's directory, so Claude Code loads the MEMORY.md itself.
The doctor agent memory section prints one line per store the flag leaves
out. An older belt fails on the flag, and then no session-start hint
reaches the session: install the new belt before a settings entry passes
`--agent`.

The ledger and the watcher both need the events service. When it is down,
ledger entries are dropped without a trace and the watcher wakes nobody;
the doctor events line says which. Ledger entries live under the
`belt-ledger` source, not `belt`. A turn that died on an API error is a
`stop-failure` entry at warn or error, with an `error` tag naming the type.
A subagent-start entry without `model` and `effort` tags means no
`ledger.agent_types` key matches its agent type, the entry was dropped, or
the belt predates the key. A named spawn reports its name as its agent
type, so a key for named spawns ends in `*` (`reviewer-*` covers
`reviewer-deep-<slug>`); a plain key never matches a named spawn.
The doctor ledger block lists the keys in effect and the warnings name the
drops. A watcher that never wakes the session usually means the item's `to`
tag does not match the session id exactly, or the Stop entry lacks
`asyncRewake`. Each item is delivered once: the cursor in
`~/.cache/belt/watch-<session>` names the newest one already delivered.

## version skew

`belt version -o json` reports the build of the binary on PATH. An
installed binary is live on the next tool call, so skew here means the
installed binary lags the repo. A fleet rebuild can report ok while an old
binary keeps answering. Rebuild and reinstall, then compare the version
again.

## first moves

1. `belt doctor`: build metadata, which config surface loaded (include
   files too), guard, allow and hint enablement with each allow's roots,
   the `belt hint read` matcher,
   moondb reachability, each agent-memory index against its budget share,
   the ledger and watch settings with events-service reachability, and the
   resolved blocked-name list. It ends with a warnings section and exits 1 when that
   section is not empty
2. `belt check bash "<command>"`, or `belt check write --file <path>
   --content "<text>"`: dry-run the guards and print each verdict
3. `belt config`: every setting in effect, with resolved defaults,
   guard allowlists, and the Claude deny patterns
4. `belt override`: every override with its state — active with remaining
   time, expired, legacy untimed, or malformed (set/extend/clear to manage)
5. `belt version -o json`, when a fix does not seem to apply: confirm the
   binary is the build you expect
6. On a `belt[payload]` deny: query the events service for source `belt`.
   The deny's `decode_error` tag names the field and the JSON type that
   arrived, and `belt version -o json` shows which build read it
