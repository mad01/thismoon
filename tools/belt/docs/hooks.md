# belt hooks

How each guard, hint and ledger entry works: what invokes it, the decision
sequence it runs, why it exists, and how it fails. The keys behind them are in
[config.md](../config.md); this page covers behavior and wiring. For the
reasoning behind the guard/hint split itself, see `docs/adr/0008` in the repo
root. `docs/adr/0024` adds the ledger as the third concept and records why
the watcher is the one command that exits 2.

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
- **Ledger entries only record.** `belt record` posts an entry and prints
  nothing at all, so it cannot keep a subagent running or block a task the
  way output or a non-zero exit would on those events.
- **belt always exits 0, with one exception.** A deny travels as JSON data,
  never as an exit code, so a bug in belt cannot break tool calls outright.
  The exception is `belt watch`, an asyncRewake hook that exits 2 when it
  wakes an idle session. On asyncRewake exit 2 blocks nothing, because the
  turn it would have blocked has already ended (see Watcher below).
- **Silence is the common case.** Most invocations write nothing at all. A
  guard that allows and a hint with nothing to say are indistinguishable from
  belt not being wired up, which is why `belt doctor` and the events
  timeline exist.
- **Every deny and every hint is attributable and audited.** Output is
  prefixed `belt[<id>]:`, and each fire POSTs an event to the local events
  service (`events.this`): denies and soft-mode hits as `warn`, hints as
  `info`. Each event is tagged with the session id on every real call, and
  with the agent id and type when the call came from inside a subagent.
  When a guard seems silent, the timeline is where to look.

## Wiring belt into Claude Code

belt registers nothing itself. There is no `belt install`. The consuming
config repo owns the hook entries in `~/.claude/settings.json`, because which
matchers to use (and whether to wire belt up at all) is a machine decision.
It is not a property of the binary (`docs/adr/0006` in the repo root). A
complete hooks block looks like this:

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
      },
      {
        "matcher": "^mcp__github__.*",
        "hooks": [{ "type": "command", "command": "~/code/bin/belt hook external-text" }]
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
        "hooks": [
          { "type": "command", "command": "~/code/bin/belt hint session-start" },
          { "type": "command", "command": "~/code/bin/belt watch", "asyncRewake": true, "timeout": 590 }
        ]
      }
    ],
    "SubagentStart": [
      {
        "hooks": [
          { "type": "command", "command": "~/code/bin/belt hint subagent-start" },
          { "type": "command", "command": "~/code/bin/belt record subagent-start" }
        ]
      }
    ],
    "UserPromptSubmit": [
      {
        "hooks": [{ "type": "command", "command": "~/code/bin/belt hint prompt" }]
      }
    ],
    "Stop": [
      {
        "hooks": [
          { "type": "command", "command": "~/code/bin/belt watch", "asyncRewake": true, "timeout": 590 }
        ]
      }
    ],
    "SubagentStop": [
      { "hooks": [{ "type": "command", "command": "~/code/bin/belt record subagent-stop" }] }
    ],
    "TaskCompleted": [
      { "hooks": [{ "type": "command", "command": "~/code/bin/belt record task-completed" }] }
    ],
    "TeammateIdle": [
      { "hooks": [{ "type": "command", "command": "~/code/bin/belt record teammate-idle" }] }
    ],
    "PermissionDenied": [
      { "hooks": [{ "type": "command", "command": "~/code/bin/belt record permission-denied" }] }
    ],
    "Notification": [
      { "hooks": [{ "type": "command", "command": "~/code/bin/belt record notification" }] }
    ],
    "PreCompact": [
      { "hooks": [{ "type": "command", "command": "~/code/bin/belt record pre-compact" }] }
    ],
    "PostCompact": [
      { "hooks": [{ "type": "command", "command": "~/code/bin/belt record post-compact" }] }
    ],
    "SessionEnd": [
      { "hooks": [{ "type": "command", "command": "~/code/bin/belt record session-end" }] }
    ],
    "StopFailure": [
      { "hooks": [{ "type": "command", "command": "~/code/bin/belt record stop-failure" }] }
    ]
  }
}
```

The `belt watch` entries do nothing until `watch.enabled` is set in the belt
config; the rest of the lifecycle block is safe to wire everywhere.

A worked version of this block lives in
`examples/dotfiles/recipes/agent-config/settings.json` at the repo root, the
settings template agent-sync applies. The belt config it pairs with is in
`examples/dotfiles/recipes/claude-hooks/`.

The event-to-subcommand mapping is fixed:

| belt subcommand | Claude Code event | matcher |
|---|---|---|
| `belt hook bash` | PreToolUse | `Bash` |
| `belt hook write` | PreToolUse | `Write\|Edit` |
| `belt hook external-text` | PreToolUse | the github.com MCP server's tools, and only that server (see below) |
| `belt hint search` | PostToolUse | the csl search MCP tools |
| `belt hint bash` | PostToolUse | `Bash` |
| `belt hint external-text` | PostToolUse | your publishing MCP tools (see below) |
| `belt hint session-start` | SessionStart | none (fires on every session) |
| `belt hint subagent-start` | SubagentStart | none, or an agent type to brief only some subagents |
| `belt hint prompt` | UserPromptSubmit | none (fires on every prompt) |
| `belt record subagent-start` | SubagentStart | none |
| `belt record subagent-stop` | SubagentStop | none |
| `belt record task-completed` | TaskCompleted | none |
| `belt record teammate-idle` | TeammateIdle | none |
| `belt record permission-denied` | PermissionDenied | none, or a tool name |
| `belt record notification` | Notification | none, or a notification type |
| `belt record pre-compact` | PreCompact | none |
| `belt record post-compact` | PostCompact | none |
| `belt record session-end` | SessionEnd | none |
| `belt record stop-failure` | StopFailure | none, or an error type |
| `belt watch` (asyncRewake, timeout 590) | Stop and SessionStart | none |

The wiring rules that are easy to get wrong:

- **The event must match the subcommand.** belt stamps its JSON output with
  the hook event it was built for (`PreToolUse`, `PostToolUse`,
  `SessionStart`, `SubagentStart`); register `belt hint search` under the
  wrong event and Claude Code silently drops the output. The exception is
  `belt hint prompt`, which emits plain stdout text rather than JSON, because
  UserPromptSubmit adds stdout to context directly and would drop an
  envelope.
- **Never register a belt command on WorktreeCreate or WorktreeRemove.**
  Those hooks replace Claude Code's own `git worktree` handling: a
  WorktreeCreate hook must print the path it created, and a WorktreeRemove
  hook that exits 0 counts as the removal. They also never fire for a
  `git worktree add` run through Bash, which is how loom worktrees are made.
  The ledger tags every entry with the worktree it happened in instead.
- **`belt watch` needs `asyncRewake` and a timeout above its max wait.**
  Without asyncRewake the hook blocks the session's turn for the whole wait.
  With a timeout at or below `watch.max_wait_s`, Claude Code kills the
  watcher before it can exit on its own. The block above uses 590 s, and
  `belt doctor` warns when an enabled watcher's max wait reaches it.
- **An unknown subcommand fails loud on purpose.** `belt hook searchh` errors
  at argument parsing instead of running zero guards, so a typo in a settings
  entry does not impersonate a healthy no-op.
- **An unreadable config denies, for the same reason.** A `config.yaml` that
  is present and fails to parse or validate makes every `belt hook` call
  return a `belt[config]:` deny naming the file. Running with "no rules"
  would look exactly like a healthy belt while every guard is disarmed. A
  config file that is simply absent is fine — that is the defaults, with
  every guard enabled. `belt hint` cannot deny, so it delivers the same
  message as advice instead.
- **An unreadable payload denies too.** When stdin holds no JSON object, or
  a field the guards read arrives with another JSON type, `belt hook`
  returns a `belt[payload]:` deny naming the field. It also leaves a `warn`
  event tagged `decode_error`. A guard that cannot see the call would
  otherwise allow it. A mistyped session or agent field never denies.
  `belt hint` stays silent on such a payload and leaves the same event.
  The full rule, with every field it covers, is in the operating doc's
  failure modes (`belt docs`).
- **The `external-text` matcher is yours to choose.** belt cannot know which
  MCP servers on your machine publish text to places other people read (issue
  trackers, chat, code review). Pick the send/create/comment tools of those
  servers. belt drops calls whose operation names a read verb at either end
  (`get_file_contents`, `issue_read`), so a broad server-wide matcher wastes
  no nudge on fetches, but the matcher is still the primary filter.
- **Only the MCP server that talks to github.com belongs on the
  `hook external-text` matcher.** The guard behind it
  (publish-internal-names) resolves the call's `owner`/`repo` pair as a
  github.com repo. The host is fixed in the code. So a server for another
  host would have its repos judged as if they lived on github.com. With a
  `public_repos` list the resolved repo is checked against it. An unlisted
  one passes. Without the list every resolved repo is public-bound, and an
  internal GitHub Enterprise server routed there would deny every internal
  PR body that names an internal repo. Either way the matcher names
  github.com tools only.

`belt doctor` verifies the binary and the config, but it does not check
whether `~/.claude/settings.json` actually points at belt. A correctly
configured belt that is simply not wired up looks identical to a healthy one.
Run one `git push origin main` dry-run through `belt check bash` and one real
session if you want proof the wiring is live.

## Other harnesses

belt is not Claude-Code-exclusive: any harness that execs a command per tool
event and reads its stdout can run it. What to expect per harness:

- **Claude Code** gets the full set: every guard event, every hint, the
  ledger and the watcher, wired as shown above. The watcher rides Claude
  Code's asyncRewake hooks, so it is Claude-only.
- **Codex** reads the same hooks schema from `~/.codex/hooks.json`, so the
  entries above work verbatim with the paths adjusted. The reference setup
  wires the six tool-event entries (`hook bash`, `hook write`,
  `hook external-text`, `hint search`, `hint bash`, `hint external-text`);
  the session-boundary
  events are not wired there, so the agent-memory and kof-consult injections
  and the kof-deposit nudge stay Claude-only. Custom hints ride the same
  `belt hint session-start` entrypoint, so a Codex setup that adds the
  SessionStart entry to its hooks file gets them with no further wiring.
  The reference setup wires no `belt record` entry either, so a Codex
  session leaves nothing in the ledger. `belt record` has no Claude-specific
  code: it decodes the fields it knows, and a payload it can't decode still
  posts, as a warn entry. Reading the ledger needs no hook: Codex queries
  the events service like any other client.
  Two Codex-specific rules:
  define hooks in `hooks.json` only. A duplicate `[[hooks.PreToolUse]]`
  block in `config.toml` runs belt twice per call. Codex trusts hooks by
  content hash, so a new or changed entry must be re-trusted inside Codex
  before it fires. Codex sends a shell call as `tool_name: "Bash"` with
  `tool_input.command` as a string. Its `Write|Edit` matchers are aliases
  for `apply_patch`, which sends `tool_name: "apply_patch"` with the patch
  text as a string in `tool_input.command` and no `file_path`, `content` or
  `new_string`. Both shapes pass the payload check, because the command is
  a string and absent fields are allowed. `belt hook write` has no path to
  guard on Codex, since the patch carries no `file_path`. That was already
  so before the payload check. Any harness that sends another JSON type in
  a field the guards read gets a `belt[payload]` deny on every such call,
  not a silent allow. Check one real payload before wiring a new harness.
- **pi** has no hooks schema either, but its permission extension can
  exec `belt check bash <command>`, `belt check write --file … --content …`,
  and `belt check external-text --tool <name> --input <json>` per tool
  call and block on exit code 1. The guards are the same, the verdict
  travels as the exit code and the `DENY` line instead of hook JSON.
- **opencode** is not wired today: opencode has no Claude-style hooks schema,
  so belt does not run there. Its native `permission` config covers the
  allow/deny surface instead.

Whichever harness drives the session, the commit-time layer is unaffected:
suspenders guards `git commit` in the repo itself, for agents and humans
alike.

## Guards

Guards run in fixed order (built-ins first, then custom guards alphabetically)
and the first denial wins. Two checks come before every guard below, so no
guard's "fails open" applies to them. A payload belt cannot read denies
with `belt[payload]`, and then a config belt cannot read denies with
`belt[config]` (see Wiring above).

### git-push-main

Blocks `git push` to `main` or `master` before it happens.

- **Fires on** every `git push` found in the Bash command, including compound
  commands (`cd x && git push`) and pushes behind a global flag
  (`git --no-pager push`). Bare `git push` and `HEAD` refspecs resolve
  the current branch via `git rev-parse --abbrev-ref HEAD` in the push
  directory.
- **Follows `cd` and `git -C`.** The guard walks the command's segments and
  tracks `cd`/`pushd`, so a push is judged in the directory it runs in, not
  the session cwd. A `git -C` target is resolved against that directory the
  same way. Absolute and relative targets resolve as written. A leading
  `~`, `$HOME`, or `${HOME}` expands to the home directory in either, so
  `git -C ~/repo` names the same repo as the absolute path, and a bare `cd`
  goes home. A target only a shell can resolve (another variable, `cd -`, a
  command substitution, a glob) leaves the directory unknown. So do `popd`
  and a bare `pushd`, since belt keeps no directory stack, `cd -P`, whose
  physical mode resolves symlinks before `..`, and
  `--git-dir`/`--work-tree`, which point git at a tree only git can name. A
  `-C` path reaches git as written: git resolves `..` physically, so
  `link/..` is the link target's parent there, where a shell's cd is logical
  and lands in the link's own parent. A push to `main` or `master` from an
  unknown directory is denied, with a hint to use an absolute path, `~`, or
  `$HOME`. belt never runs git in its own process directory to fill the gap:
  that once let a push into a non-exempt repo ride an exempt session cwd.
- **Allows when** the push directory's origin remote resolves to a repo on
  the top-level `direct_main_repos` list or on
  `guards.git-push-main.allow_repos`. Both take an exact `host/owner/repo`
  or a trailing `/*` org wildcard, the same patterns `git_identity[].repos`
  takes. A machine class where direct pushes are fine (a personal machine,
  say) disables the guard in its rendered config instead. belt reads no
  machine profile, from ralph or anywhere else (docs/adr/0010).
- **Why it exists**: a session once pushed straight to master and tried to
  self-merge. The permission system sees `git push` as one more shell command;
  this guard reads the target.
- **Fails closed.** An unresolved repo or an empty allowlist means the push
  is denied — a machine with no config at all denies every push to
  main/master. So is a push after a cd the guard could not resolve.

### git-identity

Blocks `git commit` when the repo's effective `git config user.email` is not
the one configured for that repo.

- **Fires on** every `git commit` in the command, judged in the directory it
  runs in: a tracked `cd`/`pushd`, a `git -C` target (`~` and `$HOME`
  expanded), else the session cwd.
- **Decides by** the first `git_identity` rule whose `repos` patterns match
  (exact `host/owner/repo` or a trailing `/*` org wildcard; an empty `repos`
  list covers every repo).
  The rule names the expected email; a mismatch denies with the exact
  `git config user.email <expected>` fix in the reason.
- **Why it exists**: committing with the wrong identity is painful to fix
  after push. Rules are repo-scoped rather than machine-scoped on purpose: a
  work machine committing to a personal repo in the evening still gets the
  personal email enforced, because the repo decides.
- **Fails open**: no rules configured, no rule covering the repo, an
  unresolvable repo, or an unresolvable email all allow. So does a commit
  whose directory only a shell can name, such as
  `cd "$(git rev-parse --show-toplevel)" && git commit`: it is not judged at
  all, not even in the session cwd. `mode: soft` turns a deny into a warn
  event.

### commit-guard

The work-hours nudge: discourages personal-repo commits during configured
hours.

- **Fires on** every `git commit`, judged in the directory it runs in as for
  git-identity; every `commit_guards` rule is evaluated.
- **A rule matches when**, in order: the repo is not in `always_allow`, and
  the repo matches `repos` (an empty list here matches nothing, the opposite
  of `git_identity`). It also requires that the rule's override switch is
  not set and that local time is inside `block_hours` on a `block_days` day
  (default mon-fri). A window whose end is before its start crosses
  midnight.
- **Then**: `mode: soft` (the default) emits a warn event and lets the commit
  through; `mode: hard` denies, naming the override switch
  (`belt override set <name> --reason "..."`) in the reason so a legitimate
  exception is one command away.
- **Why it exists**: a soft boundary between work hours and personal projects
  that a human can consciously step over but not absent-mindedly drift over.
- **Fails open**: unresolved repos and malformed windows block nothing, and
  neither does a commit whose directory only a shell can name
  (`cd "$(git rev-parse --show-toplevel)" && git commit` is not judged, not
  even in the session cwd).

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
  s.sh`, `tee` included) gets the whole command text scanned. At check time
  the file does not exist yet, so the command line is where its future
  content lives.
- **`curl | bash` is denied outright** (`wget` too): nothing can read what
  would run. Download to a file first, then run the file.
- **Patterns come from** the `permissions.deny` `Bash(...)` entries of
  `~/.claude/settings.json` and `settings.local.json`, read live on every
  invocation so the guard and the permission system cannot drift.
  `claude_settings.enabled: false` in the belt config turns that read off,
  leaving only the extra patterns. Patterns also come from
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

- **Fires on** Write and Edit calls whose target file sits in a public-bound
  repo. A public-bound repo is one matching the config's `public_repos` list
  (exact `host/owner/repo` or a `/*` org wildcard), or, for a rendering
  without that list, any repo with a `github.com` origin remote. Non-repo
  paths and unlisted repos are exempt; `belt doctor` says which of the two
  modes is in effect (`docs/adr/0015` in the repo root).
- **The blocked-name set is derived, not listed**. For every git repo under
  `internal_names.workspace_dirs`, the org segment, repo segment, and
  checkout directory basename each become a separate blocked name (never the
  combined `org/repo`). `blocked_words` adds names outright; `allowlist`
  removes safe ones; anything under three characters is dropped. This is the
  same derivation the suspenders pre-commit guard uses, so the session layer
  and the git layer agree whenever their configs do. A config file that
  enumerated internal names would itself be the leak.
- **Exemptions**: `exclude_paths` for paths that deliberately carry internal
  references, `allow_repos`
  by the target repo's canonical origin identity — remote identity, not path,
  so a second checkout of the same repo is covered too. For a single
  sanctioned compound that contains a blocked name (a private companion
  repo's own name), `internal_names.allow_phrases` neutralizes exactly that
  phrase in the checked content before matching. The bare name elsewhere in
  the same write still denies, so nothing leaves the blocked set.
- **Why it exists**: internal names reached `git commit` twice before the
  pre-commit hook caught them; this guard moves the check to the moment of
  writing, where the fix is cheapest.
- **Fails open** in exactly one way that matters: with no name source
  configured anywhere, the blocked set is empty and every write is allowed.
  `belt doctor` calls that state out and prints the full derived set.

### publish-internal-names

Blocks internal org, repo, and host names on their way to a public
github.com remote: the outbound half of the firewall write-internal-names
starts. Where that guard checks the file being written, this one checks what
an agent publishes around the code. That covers PR and issue text, comments,
branch and tag names, commit messages, and content pushed through the gh MCP
without a local commit. Same name set, same `allow_phrases`, one guard id on
two events.

- **Fires on the `bash` event** for `git push` to a github.com remote (the
  names of the pushed refs plus the messages of the commits the remote does
  not have yet). It also fires for `git commit -m`/`-F`, `git tag`, branch
  creation (`checkout -b`, `switch -c`, `branch <name>`, `worktree add -b`),
  and the gh subcommands that write: `pr create/edit/comment/review/merge/close/
  reopen`, `issue create/edit/comment/close/reopen/develop`, `release
  create/edit`, `repo create/edit/rename`, `gist create/edit/rename`,
  `label create/edit`, and every `gh api`. `pr view`, `pr list`, `pr
  checkout`, `repo clone` read and pass.
- **Fires on the `external-text` event** for every MCP tool the matcher
  routes there whose operation is not a read verb (`get`, `list`, `search`,
  `read`, `fetch` at either end of the name). Every string in the call's
  input except the top-level owner/repo pair is scanned, nested lists and
  objects included. So a PR body, a `head` branch name, a label list, a
  reviewer team slug, and a `push_files` content entry all count.
- **The target repo decides, the same way as for write-internal-names**.
  The push remote, the gh `-R` target, the repo a gh call names itself
  (`gh api repos/{owner}/{repo}/...`, `gh repo create OWNER/REPO`), the cwd
  origin, or the MCP call's owner/repo resolves to a canonical identity.
  That identity is public-bound when it matches `public_repos` (or, for a
  rendering without the list, when its host is github.com). An
  internal org that lives on github.com is therefore simply not listed,
  and its pushes, PR bodies, and API calls pass with no exemption. Another
  host, and `gh api --hostname` pointing elsewhere, never counts.
- **Targets that cannot be named** follow the mode, with a floor. A gist,
  a `gh api` call on the `gists` endpoints (wherever the endpoint sits
  among the arguments, so `gh api -X POST gists` counts), `gh repo create`
  without `--private` or `--internal`, and the MCP equivalents
  (`create_gist`, `update_gist`, `create_repository` unless `private` is
  true) are public by nature. No `public_repos` entry could name them, so
  they are guarded under both modes, whatever the cwd's origin is. Every
  other unnamed target (a raw `gh api` outside `repos/` and `gists`, an MCP
  call without owner/repo) is guarded under the legacy host rule. It passes
  with a list, being by definition not on it. `gh api orgs/<org>/...` stays
  on the passing side on purpose: the whole command text is scanned, so
  guarding it would deny every call into an internal org.
- **Outgoing commits**: for each pushed branch the base is its
  remote-tracking ref when one exists, else the remote's HEAD
  (`refs/remotes/<remote>/HEAD`). With neither, only ref names are checked
  rather than all of history. Messages come from `git log -z` bounded to
  the last 200 commits, and the deny names the commit (`commit 1a2b3c4
  message`). A bare `git push` resolves its target through `@{push}`, else
  origin and the current branch. `--all`, `--mirror`, and `--tags` sweeps
  check names only. Deletions (`--delete`, `:branch`) are allowed: they
  remove a name.
- **Command text, not one flag**: commits, tags, and gh calls are scanned
  against the whole bash command, so a heredoc body (which spans several
  line segments) is covered. The files a call reads (`--body-file`,
  `-F`, `--notes-file`, `--input`, `key=@path`, gist file arguments) are
  read from disk and scanned too. A file that does not exist yet
  contributes nothing; the same command is usually writing it, and then its
  text is in the command. `cd` is tracked across `&&` segments and `git -C`
  is honored, as in git-push-main. A cd target only a shell can resolve
  leaves the repo unknown, and so do `popd`, a bare `pushd`, `cd -P`, and
  a commit or push run with `--git-dir`/`--work-tree`; an unknown repo
  scans nothing.
- **Exemptions**: `allow_repos` by canonical identity, matched against the
  push remote, the gh target, or the MCP owner/repo. It is the same list
  shape as write-internal-names and a deliberate duplicate of it
  (`docs/adr/0013` in the repo root: exemptions are enumerated per check,
  never shared or derived). `internal_names.allow_phrases` neutralizes a
  sanctioned compound before matching. `mode: soft` turns denials into warn
  events on the events service, the rollout setting while the derived name
  set is tuned against real PR text and branch names.
- **Why it exists**: suspenders scans the staged diff and belt the written
  file. But a commit message, a branch name, a PR body, and a gh MCP call
  reached the remote unchecked, and an agent produces all four.
- **Fails open** the way write-internal-names does: with no name source
  configured the set is empty and nothing is denied. A public repo
  missing from `public_repos` is not guarded until it is added; `belt
  doctor` prints the set both guards use and the mode in effect. Git it
  cannot run (no repo in the cwd) resolves to "not public" for git
  commands.

### git-discard

Blocks git commands that throw away uncommitted work.

- **Fires on** `git stash` (bare, flag-led like `-u`, `push`, `save`),
  `reset --hard`, a path checkout (`checkout -- <path>`, `checkout <ref> --
  <path>`, `checkout .`) or a forced one, `switch --discard-changes`/`-f`,
  `restore` unless the only target is the index (`--staged` without
  `--worktree`), and `clean` with `-f` (a `-n` dry run passes). `stash
  list|show|apply|pop|branch` are allowed. Each invocation is judged in the
  directory it runs in: a tracked `cd`/`pushd`, a `git -C` target, else the
  session cwd.
- **Decides by** the working tree there. A clean tree (`git status
  --porcelain` empty, with `--ignored` added for `clean -x`/`-X`) allows,
  since nothing would be lost. `allow_repos` exempts repos by canonical
  identity. Otherwise the reason tells the model to run the command only
  when the user's latest message asked for it, and to ask first or hand the
  user `! <command>` when it did not.
- **Why it exists**: a session audit found agents stashing, hard-resetting,
  and checking out over the user's uncommitted work without being asked.
  Worktree changes are in no reflog, so this is damage that is hard to undo.
- **Fails closed**: a directory only a shell can name, or a `git status`
  that fails there, denies. `mode: soft` turns a deny into a warn event.

### Custom guards

A `custom_guards.<name>` config entry registers a guard implemented as any
external command, no Go required.

- belt execs the command with the tool-call fields as JSON on stdin
  (`{"command","cwd"}` for bash, `{"file_path","content","cwd"}` for write).
  Who made the call is added when the payload carries it: `session_id` on
  every real main-session call, plus `agent_id` and `agent_type` inside a
  subagent. A `belt check` dry-run has no session, so it sends the bare
  shape. The keys are additive: a script written against the bare shape
  keeps working.
- Exit 0 allows. Exit 1 denies, with stdout line one as the reason. Exit 2 or
  higher, a five-second timeout, or a failure to start all **allow with a
  warn event**. A broken external guard must never block work, but it also
  must not fail silently.
- An optional `match` substring gates when the external runs at all;
  `mode: soft` downgrades denials to warn events. `belt doctor` lists each
  custom guard with its event, mode, and a PATH reachability check, because
  an unreachable command means the guard warns-and-allows on every call — it
  checks nothing.
- `custom_guards.<name>.event` is required and accepts `bash` and `write`
  only; there is no custom guard on `external-text`. A missing or
  misspelled one is a config error rather than a guard that registers and
  never fires.

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
   that override for a timed window. The window is 10 minutes by default,
   with `--for 2h` to size it and `belt override extend <name>` to push it
   forward. It expires on its own, so a forgotten override cannot disarm
   a rule for days. Set and extend require a non-blank `--reason`, archived
   to the events service when it is running.
   `belt override` and `belt doctor` list every override with its state and
   remaining time. The switch is one file under
   `~/.config/belt/overrides/<name>` carrying its RFC 3339 expiry — nothing
   hidden (an empty file from an older belt counts as untimed and active
   until cleared). A block the override suppresses still leaves a warn event
   on the events timeline, so the exception stays auditable. The name itself
   is a label you invent in the rule's `override:` field. Name the
   exception, not the rule, and let several rules share one name when one
   switch should stand them all down. The rule and the CLI connect by exact
   string match with no cross-check, so a typo'd `set` is a silent no-op.
   Copy the name from the deny reason, which always prints the exact string
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
hints, active overrides, and the resolved allowlists. `belt check bash
"<command>"` dry-runs a verdict without a live session. Soft-mode rules
and fail-open custom guards leave `warn` events on the events timeline
rather than denying, so "it allowed something odd" is answered there.

### Scoping a guard to repos

Every repo list in the config — `git_identity[].repos`,
`commit_guards[].repos`, `commit_guards[].always_allow`,
`guards.<id>.allow_repos`, `hints.<id>.exclude_repos`, and the top-level
`direct_main_repos` — matches a canonical `host/owner/repo` exactly
**or** with a trailing `/*` org wildcard: `github.com/you/*` covers every
repo in the org. (Until belt v2, `allow_repos` was the exception and matched
exact strings only, so a `/*` entry there silently never matched.)

`direct_main_repos` is the one shared list, named for the workflow fact it
states. These repos' workflow is direct-to-main. It is read by exactly the
two checks whose subject is that workflow: `git-push-main` (push allowed)
and the `commit-policy` hint (advice silenced). It deliberately reaches
nothing else. A shared list gets edited for the cheap reason (quiet a
nudge) and must not silently grant the expensive one (disarm the
internal-name firewall or identity enforcement). See docs/adr/0013 for the
reversibility criterion behind that line.

Both resolve the repo from its **origin remote**, never its filesystem path,
so a second checkout or a cached clone of the same repo behaves identically.
Path-based exceptions use
`exclude_paths` (write-internal-names, script-deny-list): a `~/`- or
`/`-prefixed entry matches as a directory prefix, anything else as a
substring. `belt config` prints every list in effect after fallbacks, and
`belt doctor` shows the blocked-name set they produce.

## Hints

Hints add advisory context next to a tool result or at session boundaries.
Each exists because an instruction in a CLAUDE.md file proved unreliable at
the moment it mattered; a hook fires deterministically where prose gets
skimmed.

Configuration-wise almost every hint is one switch: `hints.<id>.enabled` in
the belt config, default on. The repo-aware hints (commit-policy,
lint-policy, prefer-csl, kof-assertions, kof-consult) also read an
`exclude_repos` list. Hints opt repos out with `exclude_repos`;
`allow_repos` belongs to guards, and each key on the wrong kind is a
validation error. commit-policy additionally honors the shared
`direct_main_repos` list. Everything else
about hint behavior is fixed
(see Fixed constants below). The guards carry the richer per-guard keys —
each guard section above names its own, and [config.md](../config.md)
specifies them all. A `custom_hints` entry registers an external command as
a session-start or subagent-start hint beside the built-ins (see Custom
hints below).

A payload belt cannot read runs no hint at all: nothing reaches the model,
and a `warn` event tagged `decode_error` records why (see Wiring above).

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
  about that repo, and injects up to five. They are matched on the
  repo-name boundary, so `thismoon` does not drag in `thismoon-arcade`.
- **Why**: prior sessions' conclusions about a repo should arrive before the
  session re-derives them.
- **Silent when** outside a repo, when kof is down (400 ms budget), and for
  assertions already shown this session. `belt doctor` tells "kof down" and
  "store empty" apart; the hint renders both as silence.

### kof-assertions (search)

After a csl search, surfaces stored assertions about the code the search hit.

- **Extracts** hit paths and repo from the search result, derives a subject
  from the deepest common directory, then queries the repo wide and ranks by
  shared path segments. Assertions are labelled at component level while
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
  on disk). No csl process is launched, because this runs on every Bash
  call.
- **Deliberately silent** on pipe filters (`cmd | grep x`), single-file
  greps, and paths outside an indexed repo — those are not what csl replaces.
- **The advice is runnable**: the pattern arrives translated to zoekt syntax
  (`--include=*.go` becomes `f:\.go$`), because a bare "use csl" costs a turn
  while the model guesses the query language.
- **Why**: a transcript audit found 97% of recursive greps ran inside indexed
  repos. Habit, not ignorance — so the fix is a well-timed reminder with the
  exact replacement, not a deny.

### commit-policy (bash)

After a `git commit` lands on main or master of a repo that is not opted
out, states the branch + PR commit policy and hands back the recovery. `git
switch -c <branch>` carries the commit along, `git branch -f main
origin/main` drops the local default branch back onto the remote.

- **Fires when** a commit in the command ran with its repo's current branch
  on main or master. The branch is read in the directory the commit ran in:
  a tracked `cd`/`pushd`, a `git -C` target (`~` and `$HOME` expanded), else
  the session cwd. That is what keeps a loom session quiet: the session cwd
  sits on main while every commit runs inside `cd ~/.worktrees/<repo>/<slug>
  && git commit`, on the worktree's own branch. The repo's canonical origin
  identity must also be on neither the shared `direct_main_repos` list (this
  hint is one of its two readers, docs/adr/0013) nor
  `hints.commit-policy.exclude_repos`.
- **Deliberately silent** on feature branches, opted-out repos, detached
  HEAD, and repos with no resolvable origin remote. A scratch `git init`
  repo lives its whole life on its default branch and has no upstream to
  protect. Silent too on a commit whose directory only a shell can name (a
  variable, `cd -`, a substitution): a hint that cannot tell says nothing,
  where the push guard fails closed.
- **Repo-local overlay**: a `.belt.yaml` at the repo root lets the repo
  version its own policy (docs/adr/0012). `hints.commit-policy.exclude`
  opts the repo out or back in over the machine's lists, and
  `protected_branches` replaces the default `main`/`master` set (exact
  names or trailing-`*` prefixes like `release/*`). A replacement set that
  matches nothing also silences the hint, so `exclude: false` opts back in
  only for the branches the set names. `message` appends a repo-authored
  line to the advice.
  Hints-only by construction (the loader lives in `internal/hint`, which
  guards cannot import), and never denying. A broken file draws one line
  of advisory text per commit until fixed, a foreign or empty file is a
  no-op.
- **Why**: the git-push-main guard denies at the push, which is late — the
  commit is already tangled into local main. This hint fires right after the
  commit, while moving it is still a two-command fix, and it names the exact
  commands so the model acts instead of guessing.

### lint-policy (bash)

After a `git commit` in a repo that declares a lint/format policy, relays
that policy, once per session per repo, while the commit is still
unpushed.

- **Fires when** a commit in the command ran in a repo whose root
  `.belt.yaml` carries a `hints.lint-policy.message` line. The directory the
  commit ran in is resolved as for commit-policy. The message is the whole advice: belt
  ships no language→linter table and executes nothing — the repo states its
  own toolchain (`run make fmt && make lint per component`, say) and belt
  only picks the moment.
- **Deliberately silent** when the trigger command itself names the
  toolchain outside quoted strings (`make lint && git commit` has nothing
  to be reminded of). A suppressed trigger does not spend the session's
  nudge, so a later bare commit still draws it. Also silent when the repo
  declares no message, on opted-out repos (overlay `exclude` or
  `hints.lint-policy.exclude_repos`), on repos with no resolvable origin,
  and in sessions without a session id. No id means the once-per-session
  cap cannot hold, and advice on every commit is worse than none.
- **Repo-local overlay**: the same `.belt.yaml` mechanics as commit-policy
  (docs/adr/0012). `message` declares the policy and is required for the
  hint to speak at all; `exclude` opts the repo out or back in over the
  machine's `exclude_repos`. A broken file draws one line of advisory text
  per commit until fixed.
- **Why**: "run the formatter and linter before committing" as CLAUDE.md
  prose gets skimmed mid-session. Right after a commit is the last cheap
  moment — the fix is still an amend away — and the repo's own words beat a
  generic reminder.

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
  consuming repo's choice; belt drops read-verb operations itself). It fires
  only if the session has not already run any humanizer tool and the nudge
  has not fired before.
- **The advice quotes** the opening of the just-published text, so the
  instruction cannot read as generic boilerplate.
- **Why**: instruction prose covers PR descriptions and docs, so those get
  linted; a Slack message or short issue comment gets skipped for feeling too
  small. Length is not what makes AI tells visible.

### em-dash (external-text, bash)

After a publishing call, flags em dashes in the text that just went out.

- **Fires when** an MCP call that is not a read verb carries an em dash
  anywhere in its input, or a `gh pr|issue create|edit|comment` or a `gh
  api` call sending fields (`-f`, `-F`, `--field`, `--raw-field`, `--input`)
  carries one in its command text or in a body file it names.
- **The advice** gives the count and asks for an edit now (`gh pr edit`, the
  MCP update call), replacing each with a colon, comma, parentheses, or a
  new sentence.
- **Why**: the user reads em dashes as an AI tell, an audit found them in
  about one PR body in four, and `humanizer_detect` does not flag them.
- **No dedupe**: every offending publish is flagged, since each one is a
  separate text still open to editing. `hints.em-dash.enabled` switches
  both events.

### Custom hints (session-start, subagent-start)

A `custom_hints.<name>` config entry registers a hint implemented as any
external command, no Go required. It is the hint-side twin of custom
guards, built for context that belongs to one machine. The first case is a
private daily journal that should open every session on one machine class.
That machine's rendered config registers it, with no second SessionStart
hook in the Claude settings and no compiled-in hint in a public repo. The
second is a brief for worker subagents. A hint on `subagent-start` runs once
per subagent, before its first prompt, and its advice lands in the
subagent's own context rather than the parent's. belt ships no built-in
text for that event; the brief comes from the consuming repo's config.

- belt execs the command with the hook payload fields as JSON on stdin
  (`{"event","cwd","session_id","transcript_path"}`). Inside a subagent,
  which is every `subagent-start` run, the payload adds `agent_id` and
  `agent_type`, so one script can brief an Explore agent differently from a
  general-purpose one. A main-session payload keeps the original four keys.
- stdout is the advice: trailing whitespace trimmed, multiple lines and
  leading indentation kept, emitted under `belt[<name>]:` like every
  built-in. A non-zero exit, a run
  past the budget, a failure to start, or empty stdout all **end in silence
  with a warn event**, so a broken external never breaks a session and
  never fails silently either.
- The budget is the kof hints' 400 ms by default; `timeout_ms` sizes it per
  hint. Both start hooks run before a first prompt, the session's or the
  subagent's. So the budget is the whole cost a slow external can impose,
  plus a short grace for closing the pipe and landing the warn event. On timeout belt kills the external's
  whole process group, so a hung grandchild (a `curl` inside the script)
  does not outlive the hook either.
- `agent_types` limits a `subagent-start` hint to the subagent types it
  names, matched against the payload's `agent_type`: exactly, or by prefix
  for an entry ending in `*`. A spawn of any other type skips the hint:
  belt starts no process and logs no warn event, because the hint declined
  on purpose. Without the key the hint runs for every subagent. The factory
  worker brief is the case it exists for. A named spawn reports its name as
  its `agent_type`, so entries for factory workers end in `*` and the
  factory names workers `factory-worker-<tier>-<slug>`. The brief's entry
  lists `factory-worker-low*`, `factory-worker-medium*` and
  `factory-worker-high*`, so an Explore or general-purpose spawn gets
  nothing and leaves no warning on the events timeline. The SubagentStart hook
  matcher can filter by agent type too, but it gates the whole `belt hint
  subagent-start` command and every custom hint behind it. `agent_types`
  gates one hint.
- `enabled` and `exclude_repos` sit on the entry and work like the
  built-ins' (a `hints.<name>` toggle of the same name is honored too).
  `event` accepts `session-start` and `subagent-start`. It is a string so
  prompt and post-tool events can be added later. Anything else is a config
  error naming the supported values, as is an empty `command` or a name
  that shadows a built-in hint. `agent_types` on a `session-start` hint is a
  config error too, and so is an empty list, a blank entry, or an entry
  with a `*` before its end.
- `belt doctor` lists each custom hint beside the built-ins with its event,
  budget, command, `agent_types` gate, opt-out count, and whether the
  command resolves to something executable. An unreachable command means
  the hint warns and stays silent on every session, which looks identical
  to a hint with nothing to say.
- Custom hints run after the built-ins, alphabetical by name, through the
  same `belt hint session-start` and `belt hint subagent-start`
  entrypoints, so any setup that wires those entries runs them.

Install the new binary before the config that uses it. A belt built before
subagent-start existed rejects `event: subagent-start` as an unknown event,
and that config error makes it deny every guarded tool call on the machine.
So the thismoon binary must be built and installed (a `ralph up` on the
fleet) before any machine config names the new event.

`agent_types` needs the same order for a different reason. A belt built
before the key existed does not reject it. It ignores the key and runs the
hint for every subagent, so a brief meant for factory workers reaches every
spawn. Run `ralph up` first and change the config second. Keep any agent
type check the script already does until every machine runs the new belt.
On an old binary that check makes the hint decline, which costs only the
old warn events. A `*` entry has the opposite skew: a belt older than `*`
matching reads it literally, so the brief reaches no worker until the
fleet runs the new belt.

## Ledger

`belt record <event>` turns Claude Code lifecycle events into entries on the
events service, so an orchestrator session and the workers it spawns leave
one timeline. It is the third concept beside guards and hints: an entry
never reaches the model and never steers the session (`docs/adr/0024`).

- **Events.** `subagent-start`, `subagent-stop`, `task-completed`,
  `teammate-idle`, `permission-denied`, `notification`, `pre-compact`,
  `post-compact`, `session-end` and `stop-failure`, one per Claude Code hook
  of the matching name. An unknown event fails at argument parsing, like
  every belt entrypoint.
- **Entry.** Source `belt-ledger` (`ledger.source` replaces it) and level
  `info`, except for a failed turn (see below). The title names the event
  and its qualifier: "subagent stopped (Explore)", "permission denied
  (Bash)", "notification (permission_prompt)", "turn failed (rate_limit)".
  The message is the subagent's final message, the completed task's
  subject, the denial reason, the notification's title and text, or the
  failed turn's API error. It is cut to 500 bytes on a character boundary.
- **Tags.** Every set field, nothing empty: `event`, `session_id`,
  `agent_id`, `agent_type`, `model` and `effort` (subagent-start only, see
  below), `repo`, `worktree` (only for a linked worktree), `canonical` (the
  main checkout), `branch`, `task_id`, `teammate`, `tool`, `tool_use_id`,
  `notification_type`, `trigger`, `reason`, `error` (stop-failure only),
  `transcript` (the subagent's own transcript when there is one) and
  `stop_hook_active`. Filter the timeline by any of them.
- **Failed turns.** Claude Code fires StopFailure instead of Stop when a
  turn ends on an API error after its retries ran out. Only that final
  error reaches the transcript, so this entry is the record of a turn that
  died silently. The `error` tag and the title carry the error type, or
  `unknown` when the payload names none. The type also sets the level:
  `rate_limit`, `overloaded`, `server_error` and `cloud_credential_error`
  are `warn` (the API gave up), every other type is `error` (auth,
  billing, a bad request: the user has to act). The message is the
  rendered error with its details in parentheses, "API Error: Rate limit
  reached (429 Too Many Requests)", or `No detail reported.` when the
  payload has neither. A matcher of error types limits which failures
  record.
- **Where `worktree` comes from.** It is git's toplevel for the payload's
  working directory, never the directory itself, so a hook that fires in
  `tools/belt` of a loom worktree still names the worktree root. The main
  checkout gets no `worktree` tag, only `canonical`. The payload's working
  directory belongs to the session that fired the hook. For a spawn that can
  be the parent's, so on subagent-start the tag says where the spawn
  happened, not where the subagent will work.
- **Model and effort.** The SubagentStart payload names the agent type and
  no model. `ledger.agent_types` in the belt config maps an agent type to the
  model and effort its agent definition declares. A subagent-start entry
  whose `agent_type` matches a key there gets `model` and `effort` tags; any
  other type gets neither, and no other event reads the map. A key ending
  in `*` matches by prefix: the exact key wins, then the longest `*` key.
  A named spawn reports its name as its `agent_type`, so keys for factory
  workers end in `*` and the factory names workers
  `factory-worker-<tier>-<slug>`. The map restates the definition, belt
  never infers a value. The loom-pane member
  line and the factory band read these two tags. A malformed entry or value
  drops its own tag, with a `belt doctor` warning, and never fails the
  config file (see `config.md`). Run `ralph up` before a machine config
  names the key: an older belt ignores unknown ledger keys and records no
  model or effort.
- **Never steers.** `belt record` takes no output writer at all and recovers
  from its own panics. On SubagentStop any output keeps the subagent
  running, TaskCompleted, TeammateIdle and PreCompact treat exit 2 as a
  block, and PermissionDenied reads its output for a retry flag. So for a
  valid event it writes nothing to stdout or stderr and exits 0, whatever
  happens.
- **Fails toward recording.** A config belt cannot read records with the
  defaults, so no `model` or `effort` tag, and tags the entry `config:
  unreadable, recorded with the defaults`. A payload that does not decode
  is still recorded, at `warn`, naming the decode error. A panic posts an
  `error` entry titled `belt record failed`.
- **Bounded.** Placing the event in git (two git processes) gets 1 s and
  the POST 300 ms. So a record stays inside the 1.5 s SessionEnd gives all
  of its hooks, and a stalled filesystem cannot hold a subagent's start.
- **Off switch.** `ledger.enabled: false` makes every record a no-op without
  touching the hooks.

## Watcher

`belt watch` wakes an idle session when an item addressed to it lands on the
events service. It is opt-in: nothing happens until `watch.enabled: true`.

- **When it runs.** As an asyncRewake hook on Stop and SessionStart, so
  Claude Code starts a watcher in the background each time the session goes
  idle and at session start. It reads only `session_id` from the payload.
- **What it polls.** `GET /api/events?source=belt-inbox&q=<session id>&since=<cursor>`
  every `watch.poll_interval_ms` (2000 by default), then keeps the entries
  whose `to` tag equals the session id. The `q` filter is a substring match,
  so an entry that only mentions the id is skipped.
- **Waking.** On a hit it writes the newest delivered id to its cursor
  file, prints each item as `belt[watch]: from <from tag>: <title>` with
  the message below it on stderr, and exits 2. Claude Code shows that text
  to the session as a system reminder and wakes it. The cursor moves before
  the wake, so each item is delivered once. The watcher also skips any entry
  at or before its cursor itself, so even a service that ignored `since`
  could not repeat an item. If the cursor cannot be written
  the watcher delivers nothing, because an item delivered without one would
  wake the session again on every Stop.
- **Quiet exits.** With nothing after `watch.max_wait_s` (540 by default) it
  exits 0 and the next idle starts a fresh watcher. A newer watcher for the
  same session claims the owner file and the older one exits 0 at its next
  poll. Watch disabled, a config belt cannot read, a payload without a
  session id, and an events service that does not answer all end in exit 0
  with no output.
- **Producers.** Anything that can POST to the events service can address a
  session: an entry under `belt-inbox` with `tags.to` set to the session id
  and `tags.from` naming the sender. The events MCP's emit tool works too.
- **State is one cache file.** The events service is the inbox and the
  cursor is a cache file beside the hints' seen files; there is no store
  and no port. Acknowledgement, retry or
  routing would be queue semantics, which the platform deliberately does not
  have (`docs/adr/0014`).

## How belt resolves repos

Several guards and hints above need to know "which repo is this, and is it
public?". belt answers that four different ways, and none of them involve
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
- **Locate, for the ledger.** `belt record` runs one
  `git rev-parse --show-toplevel --git-dir --git-common-dir --abbrev-ref HEAD`
  in the payload's working directory, plus the origin lookup. A linked
  worktree is the layout where the per-tree git dir differs from the shared
  common dir. The main checkout is the parent of the shared `.git`. Two rare
  layouts read wrong and are accepted: a bare repo cloned into `proj/.git`
  reads as a checkout at `proj`, and a worktree of a `--separate-git-dir`
  checkout reads as having no main checkout.

The one place belt does walk is derivation time, not hook time. The
blocked-name set behind `write-internal-names` comes from scanning
`internal_names.workspace_dirs` with the shared
[`kit/repofind`](../../../kit/repofind/README.md) package. It is the same
walker and the same org/repo/dirname derivation the suspenders pre-commit
guard uses. That is what keeps the session layer and the git layer
agreeing on what is blocked.

## Fixed constants

These are behavior, not configuration — listed so nobody hunts for the
missing key:

| Constant | Value | Where it applies |
|---|---|---|
| kof query budget | 400 ms | kof-consult, kof-assertions |
| events POST budget | 1 s | every deny and hint event |
| custom guard timeout | 5 s | custom guards |
| custom hint default budget | 400 ms | custom hints (`timeout_ms` overrides it per hint) |
| script size cap | 1 MiB | script-deny-list |
| memory facts per store | 30 | agent-memory |
| assertions per fire | 3 / 5 | kof-assertions / kof-consult |
| substantial-work threshold | 30 tool calls | kof-deposit |
| published-text excerpt | 200 bytes | humanizer-check |
| session dedupe store | `~/.cache/belt/seen-<session>` | all once-per-session hints |
| agent-memory store paths | `~/.config/agent-memory[-work]/MEMORY.md` | agent-memory |
| csl shard listing | `~/.config/csl/search-index/` | prefer-csl |
| ledger message cap | 500 bytes | belt record |
| record budgets | 1 s to locate in git, 300 ms to post | belt record |
| watcher request timeout | 2 s per poll | belt watch |
| watcher page size | 100 entries per poll | belt watch |
| watcher poll floor | 100 ms | belt watch (`poll_interval_ms` below it is clamped to it, with a doctor warning) |
| watcher max wait cap | 1 h | belt watch (`max_wait_s` past it is clamped to it, with a doctor warning) |
| watcher cursor and owner files | `~/.cache/belt/watch-<session>` and `.pid` beside it | belt watch |
| documented watch hook timeout | 590 s | belt watch (doctor warns when an enabled watcher's `max_wait_s` reaches it) |
| events source name cap | 64 characters | `ledger.source`, `watch.source` (rewritten like the events service does) |
| agent type effort levels | low, medium, high, xhigh, max | `ledger.agent_types` (the values Claude Code agent frontmatter accepts; any other effort drops the tag, with a doctor warning) |

## When a hook seems silent

In symptom order:

1. **Nothing ever fires, anywhere.** The wiring is missing or points at the
   wrong path. Check the `hooks` block in `~/.claude/settings.json` against
   the table above; `belt doctor` cannot see this for you. If belt is wired
   but every hint on one event stays silent, look for `warn` events tagged
   `decode_error`: that event's payloads do not decode.
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
   contract (kof-deposit, humanizer-check, lint-policy) or the per-session
   dedupe (kof-assertions, kof-consult). A new session resets both.
6. **A custom hint is silent.** `belt doctor` says whether its command
   resolves to something executable. Every silent outcome (a failed run,
   one past its budget, or empty stdout) leaves a `warn` event on the
   events timeline naming the cause. A silent custom hint with no event
   behind it was never run: check that the entry is enabled, on the
   `session-start` or `subagent-start` event its hook is wired for, and not
   opted out for the session's repo. A gated subagent-start hint also skips
   every agent type its `agent_types` list leaves out.
7. **The ledger has no entries.** `belt doctor` shows whether the ledger is
   enabled and whether the events service answers; an unreachable service
   drops every entry. Query the `belt-ledger` source (or your
   `ledger.source`) rather than `belt`, which holds denies and hints.
8. **The watcher never wakes the session.** `belt doctor` shows whether
   watch is enabled and warns when it cannot work. Check that the item was
   posted under the watch source with `tags.to` set to the exact session
   id, and that the Stop entry carries `asyncRewake: true`. The cursor file
   `~/.cache/belt/watch-<session>` must not already name a newer id.
