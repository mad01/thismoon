# belt hooks

How each guard and hint works: what invokes it, the decision sequence it runs,
why it exists, and how it fails. The keys behind them are in
[config.md](../config.md); this page covers behavior and wiring. For the
reasoning behind the guard/hint split itself, see `docs/adr/0008` in the repo
root.

## The execution model

belt is one binary with no daemon. Claude Code execs it once per hook event,
pipes the event payload to stdin, reads stdout, and the process exits. That
shape has consequences worth knowing before reading about any single hook:

- **Changes apply on the next tool call.** A config edit, a new deny pattern,
  or a freshly installed binary is picked up immediately — no restart, no new
  session.
- **Guards can deny, hints cannot.** A guard runs on PreToolUse and can return
  `permissionDecision: deny` with a reason the model reads. A hint has no
  denial path in its interface; the worst a broken hint can do is add a few
  lines of ignored text.
- **belt always exits 0.** A deny travels as JSON data, never as an exit code,
  so a bug in belt cannot break tool calls outright.
- **Silence is the common case.** Most invocations write nothing at all. A
  guard that allows and a hint with nothing to say are indistinguishable from
  belt not being wired up — which is why `belt doctor` and the events
  timeline exist.
- **Every deny and every hint is attributable and audited.** Output is
  prefixed `belt[<id>]:`, and each fire POSTs an event to the local events
  service (`events.this`): denies and soft-mode hits as `warn`, hints as
  `info`. When a guard seems silent, the timeline is where to look.

## Wiring belt into Claude Code

belt registers nothing itself. There is no `belt install`; the consuming
config repo owns the hook entries in `~/.claude/settings.json`, because which
matchers to use (and whether to wire belt up at all) is a machine decision,
not a property of the binary (`docs/adr/0006` in the repo root). A complete
hooks block looks like this:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [{ "type": "command", "command": "~/code/bin/belt hook bash" }]
      },
      {
        "matcher": "Write|Edit",
        "hooks": [{ "type": "command", "command": "~/code/bin/belt hook write" }]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "^mcp__csl__csl_(search|semantic_search|hybrid_search)$",
        "hooks": [{ "type": "command", "command": "~/code/bin/belt hint search" }]
      },
      {
        "matcher": "Bash",
        "hooks": [{ "type": "command", "command": "~/code/bin/belt hint bash" }]
      },
      {
        "matcher": "^mcp__(github|linear)__.*|^mcp__slack__slack_send_.*",
        "hooks": [{ "type": "command", "command": "~/code/bin/belt hint external-text" }]
      }
    ],
    "SessionStart": [
      {
        "hooks": [{ "type": "command", "command": "~/code/bin/belt hint session-start" }]
      }
    ],
    "UserPromptSubmit": [
      {
        "hooks": [{ "type": "command", "command": "~/code/bin/belt hint prompt" }]
      }
    ]
  }
}
```

A worked version of this block, with the belt config beside it, lives in
`examples/dotfiles/recipes/claude-hooks/` at the repo root.

The event-to-subcommand mapping is fixed:

| belt subcommand | Claude Code event | matcher |
|---|---|---|
| `belt hook bash` | PreToolUse | `Bash` |
| `belt hook write` | PreToolUse | `Write\|Edit` |
| `belt hint search` | PostToolUse | the csl search MCP tools |
| `belt hint bash` | PostToolUse | `Bash` |
| `belt hint external-text` | PostToolUse | your publishing MCP tools (see below) |
| `belt hint session-start` | SessionStart | none (fires on every session) |
| `belt hint prompt` | UserPromptSubmit | none (fires on every prompt) |

Three wiring rules that are easy to get wrong:

- **The event must match the subcommand.** belt stamps its JSON output with
  the hook event it was built for (`PreToolUse`, `PostToolUse`,
  `SessionStart`); register `belt hint search` under the wrong event and
  Claude Code silently drops the output. The exception is `belt hint prompt`,
  which emits plain stdout text rather than JSON, because UserPromptSubmit
  adds stdout to context directly and would drop an envelope.
- **An unknown subcommand fails loud on purpose.** `belt hook searchh` errors
  at argument parsing instead of running zero guards, so a typo in a settings
  entry does not impersonate a healthy no-op.
- **The `external-text` matcher is yours to choose.** belt cannot know which
  MCP servers on your machine publish text to places other people read (issue
  trackers, chat, code review). Pick the send/create/comment tools of those
  servers. belt drops calls whose operation names a read verb at either end
  (`get_file_contents`, `issue_read`), so a broad server-wide matcher wastes
  no nudge on fetches, but the matcher is still the primary filter.

`belt doctor` verifies the binary and the config, but it does not check
whether `~/.claude/settings.json` actually points at belt. A correctly
configured belt that is simply not wired up looks identical to a healthy one:
run one `git push origin main` dry-run through `belt check bash` and one real
session if you want proof the wiring is live.

## Other harnesses

belt is not Claude-Code-exclusive: any harness that execs a command per tool
event and reads its stdout can run it. What to expect per harness:

- **Claude Code** — the full set: both guard events and all six hints, wired
  as shown above.
- **Codex** — reads the same hooks schema from `~/.codex/hooks.json`, so the
  entries above work verbatim with the paths adjusted. The reference setup
  wires the five tool-event entries (`hook bash`, `hook write`,
  `hint search`, `hint bash`, `hint external-text`); the session-boundary
  events are not wired there, so the agent-memory and kof-consult injections
  and the kof-deposit nudge stay Claude-only. Two Codex-specific rules:
  define hooks in `hooks.json` only — a duplicate `[[hooks.PreToolUse]]`
  block in `config.toml` runs belt twice per call — and Codex trusts hooks
  by content hash, so a new or changed entry must be re-trusted inside Codex
  before it fires.
- **opencode** — not wired today: opencode has no Claude-style hooks schema,
  so belt does not run there. Its native `permission` config covers the
  allow/deny surface instead.

Whichever harness drives the session, the commit-time layer is unaffected:
suspenders guards `git commit` in the repo itself, for agents and humans
alike.

## Guards

Guards run in fixed order (built-ins first, then custom guards alphabetically)
and the first denial wins.

### git-push-main

Blocks `git push` to `main` or `master` before it happens.

- **Fires on** every `git push` found in the Bash command, including compound
  commands (`cd x && git push`). Bare `git push` and `HEAD` refspecs resolve
  the current branch via `git rev-parse --abbrev-ref HEAD` in the push
  directory.
- **Allows when** the machine carries the `personal` ralph profile (the whole
  guard steps aside — personal machines push to their own main freely), or the
  push directory's origin remote resolves to a repo on
  `guards.git-push-main.allow_repos`.
- **Why it exists**: a session once pushed straight to master and tried to
  self-merge. The permission system sees `git push` as one more shell command;
  this guard reads the target.
- **Fails closed.** An unresolved repo, an empty allowlist, or a machine with
  no profiles at all each mean the push is denied. No profiles anywhere means
  every push to main/master on the machine is denied — `belt doctor` warns
  about that state.
- **Note**: this guard reads `allow_repos` only. It does not consult
  `allow_repos_by_profile`; that field belongs to `write-internal-names`.

### git-identity

Blocks `git commit` when the repo's effective `git config user.email` is not
the one configured for that repo.

- **Fires on** every `git commit` in the command.
- **Decides by** the first `git_identity` rule whose optional `profile` the
  machine carries and whose `repos` patterns match (exact `host/owner/repo`
  or a trailing `/*` org wildcard; an empty `repos` list covers every repo).
  The rule names the expected email; a mismatch denies with the exact
  `git config user.email <expected>` fix in the reason.
- **Why it exists**: committing with the wrong identity is painful to fix
  after push. Rules are repo-scoped rather than machine-scoped on purpose: a
  work machine committing to a personal repo in the evening still gets the
  personal email enforced, because the repo decides.
- **Fails open**: no rules configured, no rule covering the repo, an
  unresolvable repo, or an unresolvable email all allow. `mode: soft` turns a
  deny into a warn event.

### commit-guard

The work-hours nudge: discourages personal-repo commits during configured
hours.

- **Fires on** every `git commit`; every `commit_guards` rule is evaluated.
- **A rule matches when**, in order: its `profile` is carried (or unset), the
  repo is not in `always_allow`, the repo matches `repos` (an empty list here
  matches nothing — the opposite of `git_identity`), the rule's override
  switch is not set, and local time is inside `block_hours` on a `block_days`
  day (default mon–fri; a window whose end is before its start crosses
  midnight).
- **Then**: `mode: soft` (the default) emits a warn event and lets the commit
  through; `mode: hard` denies, naming the override switch
  (`belt override set <name> --reason "..."`) in the reason so a legitimate
  exception is one command away.
- **Why it exists**: a soft boundary between work hours and personal projects
  that a human can consciously step over but not absent-mindedly drift over.
- **Fails open**: unresolved repos and malformed windows block nothing.

### script-deny-list

Applies the Bash deny list inside scripts, closing the "write it to a file,
then run the file" bypass.

- **Fires on** Bash commands that execute or source script content: `bash
  x.sh`, `python x.py` (versioned names like `python3.12` too), `perl`,
  `ruby`, `node`, `osascript`, `uv run`, `./x.sh`, extensionless `./deploy`
  when the file starts with a shebang, `source x.sh`, interpreter `-c`/`-e`
  strings, heredocs, `bash < x.sh` stdin redirects, and `cat x.sh | bash`
  pipes. Wrappers (`sudo`, `env`, `nohup`, `timeout <n>`, …) are stripped
  first.
- **Indirection heads are scanned too**: the command after `eval`, `xargs`,
  and `find -exec`/`-execdir`/`-ok` runs through the same patterns — those
  are the routes a prefix matcher never sees.
- **A file written and run in one command** (`echo '…' > s.sh && bash
  s.sh`, `tee` included) gets the whole command text scanned: at check time
  the file does not exist yet, so the command line is where its future
  content lives.
- **`curl | bash` is denied outright** (`wget` too): nothing can read what
  would run. Download to a file first, then run the file.
- **Patterns come from** the `permissions.deny` `Bash(...)` entries of
  `~/.claude/settings.json` and `settings.local.json`, read live on every
  invocation so the guard and the permission system cannot drift, plus
  `guards.script-deny-list.extra_patterns` for things the settings file only
  lists under "ask" (like `rm -rf`). An extra pattern starting `re:` is
  compiled as a case-insensitive regex — the escape hatch for flag
  reordering and argument wildcards the literal form cannot express.
- **Matching is** per non-comment line, case-insensitive, whole-word, and
  whitespace-normalized: `kubectl delete` matches `  kubectl   delete pod x`
  but not `kubectl deleted`, in shell lines and Python
  `subprocess`/`os.system` strings alike. Backslash-continued lines are
  joined before matching, and a second pass collapses quotes, commas, and
  brackets so `kubectl "delete"` and list-form `subprocess.run(["rm",
  "-rf", …])` match the same patterns as their plain forms.
- **`mode: soft`** downgrades every denial to a warn event on the events
  service and lets the command proceed — the rollout setting for tuning new
  patterns before they block.
- **Why it exists**: the permission system judges the literal command line.
  `bash cleanup.sh` looks harmless even when the script runs `kubectl
  delete`.
- **Fails open**: unreadable files, files over 1 MiB, `python -m`, and paths
  under `exclude_paths` are all allowed. Known holes are documented in the
  component CLAUDE.md; the fix is adding `extra_patterns` (or a `re:`
  pattern), not a bigger parser.

### write-internal-names

Blocks Write/Edit content that would put internal org, repo, or host names
into a public repo.

- **Fires on** Write and Edit calls whose target file sits in a git repo with
  a `github.com` origin remote — that is the whole public/private test, so
  other git hosts and non-repo paths are exempt by construction, never by
  enumeration.
- **The blocked-name set is derived, not listed**: for every git repo under
  `internal_names.workspace_dirs`, the org segment, repo segment, and
  checkout directory basename each become a separate blocked name (never the
  combined `org/repo`); `blocked_words` adds names outright; `allowlist`
  removes safe ones; anything under three characters is dropped. This is the
  same derivation the suspenders pre-commit guard uses, so the session layer
  and the git layer agree whenever their configs do — and a config file that
  enumerated internal names would itself be the leak.
- **Exemptions**: `exclude_paths` for paths that deliberately carry internal
  references, `allow_repos` (and `allow_repos_by_profile`, per ralph profile)
  by the target repo's canonical origin identity — remote identity, not path,
  so a second checkout of the same repo is covered too. For a single
  sanctioned compound that contains a blocked name (a private companion
  repo's own name), `internal_names.allow_phrases` neutralizes exactly that
  phrase in the checked content before matching — the bare name elsewhere in
  the same write still denies, so nothing leaves the blocked set.
- **Why it exists**: internal names reached `git commit` twice before the
  pre-commit hook caught them; this guard moves the check to the moment of
  writing, where the fix is cheapest.
- **Fails open** in exactly one way that matters: with no name source
  configured anywhere, the blocked set is empty and every write is allowed.
  `belt doctor` calls that state out and prints the full derived set.

### Custom guards

A `custom_guards.<name>` config entry registers a guard implemented as any
external command, no Go required.

- belt execs the command with the tool-call fields as JSON on stdin
  (`{"command","cwd"}` for bash, `{"file_path","content","cwd"}` for write).
- Exit 0 allows. Exit 1 denies, with stdout line one as the reason. Exit 2 or
  higher, a five-second timeout, or a failure to start all **allow with a
  warn event** — a broken external guard must never block work, but it also
  must not fail silently.
- An optional `match` substring gates when the external runs at all;
  `mode: soft` downgrades denials to warn events. `belt doctor` lists each
  custom guard with a PATH reachability check, because an unreachable command
  means the guard warns-and-allows on every call — it checks nothing.

## Overriding, allowing, and disabling

A guard that blocks you when it is wrong is worse than no guard — you stop
trusting the right blocks too. There is an escape ladder, ordered from
narrowest to widest; take the lowest rung that solves your problem.

1. **Read the deny reason.** Every deny is prefixed `belt[<guard-id>]:` and
   names the fix when there is one — the branch-and-push commands for
   git-push-main, the `git config user.email <expected>` line for
   git-identity, the override switch name for a hard commit-guard rule.
2. **Flip the rule's override switch** (commit-guard rules only, today):
   `belt override set <name> --reason "..."` suppresses every rule naming
   that override for a timed window — 10 minutes by default, `--for 2h` to
   size it, `belt override extend <name>` to push it forward, and it expires
   on its own, so a forgotten override cannot disarm a rule for days. Set
   and extend require a non-blank `--reason`, archived to the events service
   when it is running.
   `belt override` and `belt doctor` list every override with its state and
   remaining time. The switch is one file under
   `~/.config/belt/overrides/<name>` carrying its RFC 3339 expiry — nothing
   hidden (an empty file from an older belt counts as untimed and active
   until cleared). A block the override suppresses still leaves a warn event
   on the events timeline, so the exception stays auditable. The name itself
   is a label you invent in the rule's `override:` field — name the
   exception, not the rule, and let several rules share one name when one
   switch should stand them all down. The rule and the CLI connect by exact
   string match with no cross-check, so a typo'd `set` is a silent no-op:
   copy the name from the deny reason, which always prints the exact string
   the blocking rule reads.
3. **Allow the repo.** For repo-scoped denials, put the repo on the guard's
   allowlist instead of weakening the guard everywhere — see "Scoping a
   guard to repos" below.
4. **Disable the guard or hint in the belt config**:
   `guards.<id>.enabled: false` or `hints.<id>.enabled: false` in
   `~/.config/belt/config.yaml`. No restart — belt reads config fresh on the
   next tool call. This is the right rung for "this guard does not fit how I
   work", and it is reversible the same way.
5. **Unwire the hook in settings.** Removing the entry from
   `~/.claude/settings.json`'s `hooks` block stops belt running for that
   event at all. This is the permanent form — prefer rung 4, which keeps the
   wiring in place and the decision in one config file.

To see the state behind any decision: `belt doctor` shows enabled guards and
hints, active overrides, and the resolved allowlists; `belt check bash
"<command>"` dry-runs a verdict without a live session; and soft-mode rules
and fail-open custom guards leave `warn` events on the events timeline
rather than denying, so "it allowed something odd" is answered there.

### Scoping a guard to repos

Two different matching dialects exist, and mixing them up is the most common
config mistake:

- **Rule lists** — `git_identity[].repos`, `commit_guards[].repos`, and
  `commit_guards[].always_allow` — match canonical `host/owner/repo` exactly
  **or** with a trailing `/*` org wildcard: `github.com/you/*` covers every
  repo in the org.
- **Guard allowlists** — `guards.<id>.allow_repos` and
  `allow_repos_by_profile` — match the exact `host/owner/repo` string
  **only**. A trailing `/*` is not a wildcard there; it just never matches.

Both resolve the repo from its **origin remote**, never its filesystem path,
so a second checkout or a cached clone of the same repo behaves identically.
`allow_repos_by_profile` is read only by `write-internal-names`;
`git-push-main` takes plain `allow_repos`. Path-based exceptions use
`exclude_paths` (write-internal-names, script-deny-list): a `~/`- or
`/`-prefixed entry matches as a directory prefix, anything else as a
substring. `belt config` prints every list in effect after fallbacks, and
`belt doctor` shows the blocked-name set they produce.

## Hints

Hints add advisory context next to a tool result or at session boundaries.
Each exists because an instruction in a CLAUDE.md file proved unreliable at
the moment it mattered; a hook fires deterministically where prose gets
skimmed.

Configuration-wise every hint is one switch: `hints.<id>.enabled` in the
belt config, default on. Everything else about hint behavior is fixed (see
Fixed constants below). The guards carry the richer per-guard keys — each
guard section above names its own, and [config.md](../config.md) specifies
them all.

### agent-memory (session-start)

Injects the shared agent-memory index when a session opens.

- **Reads** `~/.config/agent-memory/MEMORY.md` (every machine) and
  `~/.config/agent-memory-work/MEMORY.md` (machines that have one), keeps
  only the fact bullets, and injects them as session context. A missing store
  renders nothing, so a personal machine never sees a work index.
- **Caps** at 30 facts per store; past that it truncates and flags the store
  for pruning.
- **No per-session dedupe** on purpose: the index is the thing to see every
  session.
- **Why**: "read the memory index at session start" as instruction prose
  triggers sometimes. A SessionStart hook triggers always.
- **Setup**: the store is a git clone you create yourself; belt only reads
  it. A worked recipe — clone item, index format, and a fact-file template —
  is in `examples/dotfiles/recipes/agent-memory/` at the repo root.

### kof-consult (session-start)

Surfaces the current repo's stored assertions before any searching happens.

- **Resolves** the session's working directory to a repo via its origin
  remote, queries the local keeper-of-facts (kof) service for assertions
  about that repo, and injects up to five — matched on the repo-name
  boundary, so `thismoon` does not drag in `thismoon-arcade`.
- **Why**: prior sessions' conclusions about a repo should arrive before the
  session re-derives them.
- **Silent when** outside a repo, when kof is down (400 ms budget), and for
  assertions already shown this session. `belt doctor` tells "kof down" and
  "store empty" apart; the hint renders both as silence.

### kof-assertions (search)

After a csl search, surfaces stored assertions about the code the search hit.

- **Extracts** hit paths and repo from the search result, derives a subject
  from the deepest common directory, then queries the repo wide and ranks by
  shared path segments — assertions are labelled at component level while
  search hits go deeper, and querying the hit's own subject would find
  nothing.
- **Caps** at three, drops retracted assertions, includes stale ones but
  marks them (stale means the pinned code changed; that is a signal, not
  noise), and repeats nothing within a session.
- **Why**: assertions only pay off if a later session reads them, and nothing
  else prompts that read at the moment the related code surfaces.

### prefer-csl (bash)

After a Bash command sweeps multiple files inside a repo csl already indexes,
hands back the equivalent `csl_search` call.

- **Fires when** a `grep`/`rg`/`ag`/`find`/`fd` command recurses or touches
  multiple files inside an indexed repo (checked against csl's shard listing
  on disk — no csl process is launched, because this runs on every Bash
  call).
- **Deliberately silent** on pipe filters (`cmd | grep x`), single-file
  greps, and paths outside an indexed repo — those are not what csl replaces.
- **The advice is runnable**: the pattern arrives translated to zoekt syntax
  (`--include=*.go` becomes `f:\.go$`), because a bare "use csl" costs a turn
  while the model guesses the query language.
- **Why**: a transcript audit found 97% of recursive greps ran inside indexed
  repos. Habit, not ignorance — so the fix is a well-timed reminder with the
  exact replacement, not a deny.

### kof-deposit (prompt)

Once per session, nudges a session that did substantial work to record what
it derived.

- **Fires when** a prompt arrives after the transcript shows at least 30 tool
  calls and no `kof_assert` call yet. The check matches the JSON tool-use
  form, not the bare name, so prose mentions of `kof_assert` (including this
  hint's own earlier advice) do not count as a deposit.
- **The advice arrives prefilled** with the repo, repo path, and session id,
  and asks for an explicit deposit-or-decline decision — passive wording
  measurably got ignored.
- **Why**: the consult hints only pay off if sessions actually write
  assertions, and by the time a session ends its conclusions are about to
  evaporate with the context.
- **Once per session**, tracked in `~/.cache/belt/seen-<session>`; no session
  id means no nudge.

### humanizer-check (external-text)

Once per session, after an MCP call publishes text somewhere other people
read, points at `humanizer_detect` while the wording is still editable.

- **Fires when** a matched MCP tool call publishes (the matcher is the
  consuming repo's choice; belt drops read-verb operations itself), the
  session has not already run any humanizer tool, and the nudge has not fired
  before.
- **The advice quotes** the opening of the just-published text, so the
  instruction cannot read as generic boilerplate.
- **Why**: instruction prose covers PR descriptions and docs, so those get
  linted; a Slack message or short issue comment gets skipped for feeling too
  small. Length is not what makes AI tells visible.

## How belt resolves repos

Several guards and hints above need to know "which repo is this, and is it
public?". belt answers that three different ways, and none of them involve
walking the filesystem at hook time — a hook runs on every tool call and
cannot afford discovery:

- **Point-in-place, by origin remote.** The bash guards (git-push-main,
  git-identity, commit-guard) and the kof hints run
  `git remote get-url origin` in the relevant directory and canonicalize to
  `host/owner/repo`. Identity is the remote, never the filesystem path, so a
  second checkout or a ralph source-cache clone of the same repo is treated
  identically.
- **Walk up, for writes.** `write-internal-names` resolves the target file
  through `nearestExistingDir` — walking upward, because a Write may create
  its parent directories first — and then reads that directory's origin
  remote. A `github.com` remote is the entire public/private test.
- **Read another tool's artifacts, for csl.** `prefer-csl` never asks csl
  anything; it lists csl's shard directory (`~/.config/csl/search-index/`)
  to learn which repos are indexed, and names the repo containing a swept
  path as `<parent-dir>/<repo-dir>` to match csl's shard naming.

The one place belt does walk is derivation time, not hook time: the
blocked-name set behind `write-internal-names` comes from scanning
`internal_names.workspace_dirs` with the shared
[`kit/repofind`](../../../kit/repofind/README.md) package — the same walker
and the same org/repo/dirname derivation the suspenders pre-commit guard
uses, which is what keeps the session layer and the git layer agreeing on
what is blocked.

## Fixed constants

These are behavior, not configuration — listed so nobody hunts for the
missing key:

| Constant | Value | Where it applies |
|---|---|---|
| kof query budget | 400 ms | kof-consult, kof-assertions |
| events POST budget | 1 s | every deny and hint event |
| custom guard timeout | 5 s | custom guards |
| script size cap | 1 MiB | script-deny-list |
| memory facts per store | 30 | agent-memory |
| assertions per fire | 3 / 5 | kof-assertions / kof-consult |
| substantial-work threshold | 30 tool calls | kof-deposit |
| published-text excerpt | 200 bytes | humanizer-check |
| session dedupe store | `~/.cache/belt/seen-<session>` | all once-per-session hints |
| agent-memory store paths | `~/.config/agent-memory[-work]/MEMORY.md` | agent-memory |
| csl shard listing | `~/.config/csl/search-index/` | prefer-csl |

## When a hook seems silent

In symptom order:

1. **Nothing ever fires, anywhere.** The wiring is missing or points at the
   wrong path. Check the `hooks` block in `~/.claude/settings.json` against
   the table above; `belt doctor` cannot see this for you.
2. **One guard never denies.** Run the case through `belt check bash "<the
   command>"` or `belt check write --file <path> --content <text>` — one
   verdict line per guard shows whether the guard matched and why it decided
   as it did. Then `belt config` shows which file and key produced that
   state.
3. **A guard allows something you expected it to block.** Check the events
   timeline: soft-mode rules and broken custom guards allow with a `warn`
   event rather than denying silently.
4. **A kof hint is silent.** `belt doctor` reports whether kof is reachable
   and how many assertions it stores — the hint renders "service down" and
   "store empty" identically.
5. **A hint fired once and never again.** That is the once-per-session
   contract (kof-deposit, humanizer-check) or the per-session dedupe
   (kof-assertions, kof-consult). A new session resets both.
