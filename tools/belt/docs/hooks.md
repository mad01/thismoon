# belt hooks

How each guard, hint and ledger entry works: what invokes it, the decision
sequence it runs, why it exists, and how it fails. The keys behind them are in
[config.md](../config.md); this page covers behavior and wiring. For the
reasoning behind the guard/hint split itself, see `docs/adr/0008` in the repo
root. `docs/adr/0024` adds the ledger as the third concept and records why
the watcher is the one command that exits 2. `docs/adr/0032` adds the allow
as the fourth, and records why it is off by default.

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
- **Allows answer allow, and only after the guards.** An enabled allow runs
  in the same PreToolUse hook once every guard passed cleanly, with no
  denial and no warning, and may return `permissionDecision: allow`. It has
  no denial path, a deny always wins, and every allow is off until the
  config turns it on (see Allows below).
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
  When a guard seems silent, the timeline is where to look. An approval is
  prefixed `belt[<id>]:` too, but posts no event: it would be the most
  frequent one and push denials out of the source's capped history.

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
      },
      {
        "matcher": "Read",
        "hooks": [{ "type": "command", "command": "~/code/bin/belt hook read" }]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "Bash",
        "hooks": [{ "type": "command", "command": "~/code/bin/belt hint bash" }]
      },
      {
        "matcher": "Read|mcp__csl__csl_read",
        "hooks": [{ "type": "command", "command": "~/code/bin/belt hint read" }]
      },
      {
        "matcher": "^mcp__(github|linear)__.*|^mcp__slack__slack_send_.*",
        "hooks": [{ "type": "command", "command": "~/code/bin/belt hint external-text" }]
      }
    ],
    "SessionStart": [
      {
        "hooks": [
          { "type": "command", "command": "~/code/bin/belt hint session-start --agent claude-code" },
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
| `belt hook read` | PreToolUse | `Read` |
| `belt hint bash` | PostToolUse | `Bash` |
| `belt hint read` | PostToolUse | `Read\|mcp__csl__csl_read` |
| `belt hint external-text` | PostToolUse | your publishing MCP tools (see below) |
| `belt hint session-start` (`--agent claude-code` in Claude Code) | SessionStart | none (fires on every session) |
| `belt hint subagent-start` | SubagentStart | none, or an agent type to brief only some subagents |
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

There is no `belt hint search` or `belt hint prompt`. No hint ran on either,
so belt dropped them. A settings entry that still calls one fails as an
unknown event on every csl search or prompt until you delete it.

The wiring rules that are easy to get wrong:

- **The event must match the subcommand.** belt stamps its JSON output with
  the hook event it was built for (`PreToolUse`, `PostToolUse`,
  `SessionStart`, `SubagentStart`); register `belt hint read` under the
  wrong event and Claude Code silently drops the output.
- **The read matcher names both tools.** With `Read` alone, read-size never
  sees a `csl_read`. The matcher is `Read|mcp__csl__csl_read`, with the
  server segment your csl MCP registration uses. `belt doctor` checks it:
  its hook wiring section prints a `WARNING` line naming the matcher to set
  when no `belt hint read` entry lets a `csl_read` through. That line leaves
  the exit status alone, since read-size is advisory.
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

`belt doctor` verifies the binary and the config. Of the wiring, it checks
the matcher of the `belt hint read` entry, and whether the SessionStart
entry passes `--agent claude-code` when a memory store needs it. It doesn't
check whether the rest of `~/.claude/settings.json` points at belt. A correctly
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
  wires the five tool-event entries (`hook bash`, `hook write`,
  `hook external-text`, `hint bash`, `hint external-text`);
  the session-boundary events are not wired there, so the
  agent-memory and moondb-inflight injections stay Claude-only. Custom hints
  ride the same `belt hint session-start` entrypoint, so a Codex setup that
  adds the SessionStart entry to its hooks file gets them with no further
  wiring. Leave `--agent claude-code` off that entry: Codex has no auto
  memory, so it must get every agent-memory store.
  The reference setup wires no `belt record` entry either, so a Codex
  session leaves nothing in the ledger. `belt record` has no Claude-specific
  code: it decodes the fields it knows, and a payload it can't decode still
  posts, as a warn entry. Reading the ledger needs no hook: Codex queries
  the events service like any other client.
  Codex has no Read tool (it reads files through the shell), so
  `belt hook read` has nothing to guard there.
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
  travels as the exit code and the `DENY` line instead of hook JSON. The
  gate does not run the read guard (large-read): its edit exemption reads
  Claude Code's transcript format, and pi's session file is not one. The
  pi package in `pi/` also runs the session-start, bash, read and
  external-text hints. It records pre-compact, post-compact, session-end,
  permission-denied and stop-failure in the ledger, adapting pi's events
  to the Claude payloads; the README's pi section maps them.
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
  goes home. A variable the command sets to a plain literal path is
  substituted first, so `S=~/repo; git -C $S push` is judged in `~/repo`
  (`${S}`, `"$S"`, and `cd $S` too). belt does that only when nothing could
  leave the variable empty or change it before the use; otherwise every
  variable stays as written. So the name gets one assignment, at the start
  or after a `;` or newline, with no quote before it. And the command holds
  no heredoc, subshell, brace group, comment, backslash, glob, keyword,
  other variable, zsh modifier like `$S:h`, or builtin such as `eval`,
  `read`, or `source`. A target only a shell can resolve (any other
  variable, `cd -`, a command substitution, a glob) leaves the directory
  unknown. So do `popd`
  and a bare `pushd`, since belt keeps no directory stack, `cd -P`, whose
  physical mode resolves symlinks before `..`, and
  `--git-dir`/`--work-tree`, which point git at a tree only git can name.
  A cd the shell may not run as written is unknown too: `cd +1`, `chdir`,
  any word before cd but `builtin`, a cd after `||` or in a pipeline, or a
  relative target while CDPATH is set. So is git with `GIT_DIR`,
  `GIT_WORK_TREE`, `GIT_COMMON_DIR` or `env -C`, and a push where git reads
  no branch, as after a failed cd and `;`. A
  `-C` path reaches git as written: git resolves `..` physically, so
  `link/..` is the link target's parent there, where a shell's cd is logical
  and lands in the link's own parent. A push to `main` or `master` from an
  unknown directory is denied, with a hint to use an absolute path, `~`,
  `$HOME`, or a variable set to one of those at the start of a plain
  command. belt never runs git in its own process directory to fill the gap:
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
  strings, heredocs, `bash < x.sh` stdin redirects, `cat x.sh | bash`
  pipes, and a heredoc or here-string passed through `cat`, `head`, `tail`
  or `tee` into an interpreter (`cat <<EOF | sh`). Wrappers (`sudo`, `env`,
  `nohup`, `timeout <n>`, …) are stripped first.
- **Indirection heads are scanned too**: the command after `eval`, `xargs`,
  and `find -exec`/`-execdir`/`-ok` runs through the same patterns — those
  are the routes a prefix matcher never sees.
- **A file written and run in one command** (`echo '…' > s.sh && bash
  s.sh`, `tee` included) gets the whole command text scanned. At check time
  the file does not exist yet, so the command line is where its future
  content lives. A written file the same command gives an execute bit
  (`chmod +x s.sh`, `chmod 755 s.sh`) is scanned the same way. A mode that
  adds none, such as `600` or `-x`, is left alone.
- **A heredoc written to a text file is text, not commands.** The command
  is split into lines, so a heredoc body is otherwise read line by line as
  commands, and a markdown code block showing `bash -c '…'` or `xargs rm
  -rf` is denied. The guard skips those lines (`scripttext.go`) only when
  it can show the shell never runs them:
  - The heredoc is a bare `cat` or `tee`, optionally after `sudo`, whose
    other words are output redirects, tee's `-a`/`-i`, and file targets.
    Its line is reached through plain lines only: no quote, comment,
    substitution or continuation before it.
  - Each target is a literal path, with no variable, glob or `~user`, and
    a document extension: `.md`, `.markdown`, `.txt`, `.rst`, `.adoc`,
    `.log`, `.csv` or `.tsv`. JSON, YAML and TOML are left out, because
    hook runners and mise run commands from `package.json`, `lefthook.yml`
    and `mise.toml`.
  - No part of the target's path below the cwd or home starts with a dot,
    so `.git/`, `.husky/`, `.claude/` and dotfiles are out. The target is
    not under `/etc`, `/private`, `/Library`, `~/Library`, `/usr`, `/opt`,
    `/System`, `/bin`, `/sbin` or `/dev`, and no directory on the way is a
    symlink. A temp folder (`/tmp`, `/private/tmp`, `/var/tmp`,
    `/private/var/tmp` or `~/.tmp`, but not `$TMPDIR` or `/var/folders`) is
    a base like the cwd and home, so only the part below it is checked.
  - An existing target is a regular file with one link and no execute bit.
  - The delimiter is quoted, or the body holds no `$`, backtick or
    backslash, since the shell expands an unquoted body. The body does not
    start with a shebang.
  - Every `cd` is `cd PATH` followed by `&&`. A relative target counts only
    after the cds in its own `&&` chain, since a failed cd skips the rest of
    that chain. `CDPATH` in the command ends the relaxation, and one in the
    environment makes a relative `cd` unknown.
  - Every other step is `mkdir`, `touch`, `ls`, `echo`, `printf`, `cat`,
    `wc`, `pwd`, `true`, `git add|commit|status|diff|log|show` without
    `--output` or `--chmod`, `gh pr|issue create|edit|comment` or `gh
    release|gist create|edit`. No step carries a redirect or `sudo`, and
    nothing outside the bodies opens a pipe, a substitution, a subshell or
    a background job.

  So `cat > doc.md <<'EOF'` followed by `git add doc.md` passes, while the
  same file run, sourced, made executable, copied or piped on keeps every
  line checked.
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
- **One Python heredoc shape is read as code, not text.** Every other inline
  script matches as text, so a heredoc that only writes a file naming
  `rm -rf` is denied like one that runs it. The exception (`scriptpy.go`)
  is a Python heredoc the guard can prove runs nothing, and it needs all of
  these:
  - It is the whole command: `python3 -I [-] <<'EOF'`, optionally after
    `cd DIR &&` or `cd DIR;`, with only option flags and output redirects
    (`> out.txt`, `2>&1`) on the line, and nothing after the terminator.
    A second heredoc, an input redirect, a pipe, a substitution, a
    background job or any other command keeps the text match.
  - The interpreter is the bare name (`python3`, `python3.12`). A path such
    as `./python3` or `~/bin/python3` may be a link to a shell, so it keeps
    the text match.
  - It runs with `-I`. Without it Python imports from the working directory,
    so a script could write `heapq.py` and have a later import in the same
    run execute it. `-I` keeps the working directory and the user site
    directory off the import path. It does not cover the stdlib or
    site-packages directory: where the user owns it (mise and Homebrew
    installs), a script can still write a module there and import it. The
    text match never stopped that write either.
  - The delimiter is quoted, or the body holds no `$`, backtick or
    backslash, so the shell hands the text over unchanged.
  - Every import is json, re, pathlib, textwrap, datetime or collections.
  - No word in the body is a blocked name: os, sys, subprocess, posix,
    builtins, exec, eval, getattr, vars, globals and the rest. Nor is any
    word `_`-prefixed, a frame attribute such as `gi_frame` or `f_globals`,
    or the bare builtin `compile` (`re.compile` is fine). Strings and
    comments count too, and a source encoding declaration keeps the text
    match.

  A blocked word inside a string costs only a text match. A denied Python
  heredoc's reason names the shape that passes.
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
  true, each also behind a prefix such as pi-mcp-adapter's `gh_com_`) are
  public by nature. No `public_repos` entry could name them, so
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

### rm-temp

Sends deletes of temp paths to `toss-bin` instead of `rm`.

- **Fires on** a Bash call in which every `rm` deletes only temp paths.
  `rm` counts only as a command name: in a chain (`make build && rm -rf
  /tmp/x`), a pipeline, a subshell, a `{ }` group, after `if`, `then` or
  `do`, behind `NAME=value` prefixes, and as `\rm` or `/bin/rm`. An `rm`
  inside a quoted string, a `$(...)` substitution, a heredoc body or a
  comment is not one. Neither is an `rm` that is another command's
  argument: `echo rm`, `xargs rm`, `find -exec rm`, `sudo rm`.
- **Decides by** each target, cleaned lexically. belt never stats a path
  or follows a symlink. A target is temp when it sits strictly inside
  `/tmp`, `/var/tmp`, `/private/tmp`, `/private/var/tmp` or `~/.tmp`, the
  folders `toss-bin --temp-only` accepts. A leading `~`, `$HOME` or
  `${HOME}` expands to the home directory. A variable set from `mktemp`
  earlier in the same command counts too (`D=$(mktemp -d); ...; rm -rf
  "$D"`, or `"$D/sub"`), as long as the target stays inside what mktemp
  made. Not temp: a folder itself (`/tmp`), a path that cleans out of one
  (`/tmp/../etc`, `$HOME/.tmp/../x`), and a relative path. An unknown
  variable or substitution and brace expansion are not temp either.
  Neither is a glob that could leave its folder: one with a `..`
  component, or a globbing component that starts with a dot, such as `.*`.
- **Answers with** the toss-bin command for each rm, its targets as
  written: `rm -rf /tmp/x "$D"` becomes `toss-bin --temp-only /tmp/x "$D"`.
  A plain `mktemp` creates in `$TMPDIR`, which is under `/var/folders` on
  macOS and which toss-bin refuses. When a target comes from one, the
  reason also says to give mktemp a template under `/tmp`.
- **Why it exists**: a settings ask rule `Bash(rm:*)` shows a permission
  dialog for every rm, temp paths included. A PreToolUse deny runs before
  the ask rule, so the agent switches to toss-bin and no dialog shows.
  toss-bin moves the paths to the trash, so the delete also stays
  recoverable.
- **Fails open.** Anything the guard cannot vouch for gets no answer, and
  the ask rule decides as before. That covers one target outside the temp
  folders or one it cannot resolve, and one rm of several that does not
  qualify. It also covers an option after an operand (BSD and GNU rm read
  it differently), an unknown option, and a command it cannot read. On a
  machine where `toss-bin` is not on PATH every rm gets no answer, and
  `belt doctor` marks the guard's line. There is no mode and no repo list:
  `enabled: false` is the only switch.

### large-read

Refuses a whole-file Read of a large file before the tokens are spent.

- **Fires on** a Read with no `offset` and no `limit` of a text file of
  `large_file_lines` or more (300 by default) that csl indexes: a file in
  a repo csl indexes, or in a linked worktree of one. The index check is
  the shard listing read-size and prefer-csl read. The line count stops
  reading at the threshold, so a large file costs one bounded read. Every
  file type counts, not only the ones `csl_outline` knows, because
  `csl_ask` reads any indexed text file.
- **A linked worktree** has no shard of its own, but csl builds it an
  index on the first call that names it. The guard finds the worktree from
  its files alone, as prefer-csl does (`cslindex.LinkedWorktree`): the
  `.git` pointer file, then the `commondir` file in the git dir it names,
  which leads to the main checkout. The worktree counts when its main
  checkout has a shard. A worktree of a repo csl does not index passes.
- **The reason** names the file and hands over the `csl_ask` call for it,
  built by the same code as the read-size hint's: a placeholder question,
  the path as an anchored `f:` query and the repo as an anchored regex. In
  a linked worktree the call sets `worktree` to the worktree's root instead
  of a repo, with the path relative to that root, so csl answers from the
  worktree's files and not the main checkout's. Its last line says that a
  ranged read passes. That is the way in for a file the agent will edit,
  since an edit needs the real lines.
- **Passes a file the `csl_ask` call cannot answer from.** csl's walk
  keeps some files out of the index, name and all, so the call's `f:`
  query finds nothing. Those rules are csl's own. The directory names, the
  `.cslignore` matcher and the 2 MiB size limit come from `kit/cslignore`,
  the package csl's walk uses, so the two cannot drift apart. A file passes
  when a directory on its path is `node_modules`, `vendor`, `__pycache__`,
  `build`, `dist`, `target`, a hidden directory, or a symlink. It passes
  when the checkout's `.cslignore` matches it, when it is a symlink, and
  when it is over 2 MiB. zoekt keeps every other file at least by name, and
  `csl_ask` reads it from disk. So a text file zoekt skips the content of,
  such as one with too many distinct trigrams, is refused. `csl_ask`
  refuses a file with a NUL byte anywhere as binary, so such a file passes.
  Every hidden directory counts, `.github` and `.claude` too: csl indexes a
  file there only when git tracks it, and the guard runs no git. A
  gitignored file anywhere else is refused, because csl's walk reads no
  `.gitignore`. The NUL check reads the whole file. It runs after the line
  count and the index lookup, and before the transcript check.
- **Allows** any Read with an `offset` or a `limit`, a relative path, an
  image, PDF or notebook, a missing file, a directory, a file it cannot
  read, a file with a NUL byte, a file under the threshold, and a file
  outside the index.
- **Allows a file the agent already edited.** Only on a read it would
  refuse, the guard reads the calling agent's transcript and passes when it
  holds an `Edit`, `Write` or `MultiEdit` call on the same path. The main
  thread's calls are in the session transcript the payload names. A
  subagent's are in `<session>/subagents/agent-<id>.jsonl` beside it, and
  only those count for it. So a subagent reviewing what the main thread
  changed is refused like any reader.
- **Why it exists**: the read-size hint gave the same advice after the
  read, when every line was already in context. Prose asking agents to try
  `csl_ask` first did not change the habit.
- **Fails open**: a transcript that cannot be found or read allows, since a
  wrong refusal blocks work and a missed one only costs tokens. There is no
  soft mode: `enabled: false` switches the guard off, and without the
  PreToolUse `Read` entry it never runs. `belt check read --file <path>`
  allows unless `--transcript` names a transcript to read (`/dev/null` for
  one with no edits).

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
  never fires. So is an empty `command`: a guard that runs nothing checks
  nothing.

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

## Allows

An allow answers allow for one narrow call shape, so the call runs without
a permission prompt. It rides the `belt hook <event>` entry the guards
already use, so it needs no new wiring. The hook runs it only after a
clean guard pass: no guard denied, no custom guard failed open (timed out,
failed to start, or exited with neither 0 nor 1), and no guard in soft
mode let the call through with a warning. An unreadable payload or config
denies before either runs. So a deny, or a warning that stands in for
one, always wins. When an allow gives no answer, the call goes to Claude
Code's own permission check as before.

Per the Claude Code hooks documentation, a hook allow skips the permission
prompt and does not override a settings deny or ask rule. Whether it also
skips the auto-mode classifier is not documented and not verified;
docs/adr/0032 says how to check.

Every allow is off until `allows.<id>.enabled: true`, the opposite of
guards and hints: a missing config must never approve a call. `belt doctor`
lists each allow with its state and roots.

### git-c-read (bash)

- **Invoked**: `belt hook bash`, after a clean pass of every bash guard,
  when `allows.git-c-read.enabled` is true.
- **Decision**: answers allow with `belt[git-c-read]:` only when every
  command in the call is `git -C <dir> <subcommand> [args]`. Any doubt is no
  answer.
  1. The tool call is a plain Bash call: `tool_name` is exactly `Bash`,
     `dangerouslyDisableSandbox` and `run_in_background` are absent or
     false, and `tool_input` holds no key besides those, `command`,
     `description` and `timeout`. A key a newer Claude Code adds means no
     answer until belt knows it.
  2. The call is read by a strict, quote-aware reader. Commands may be
     joined by `;`, `&&`, `||` or newlines, and words may be quoted (single
     quotes, or double quotes without `$`, backticks, `\` or `!`). Anything
     else means no answer: a pipe, a redirect, `$`, backticks, a subshell or
     group, a lone `&`, a heredoc, a comment, a line continuation, an
     unquoted glob (`*`, `?`, `[`, `^`) or brace, a word that starts with
     `=`, a non-ASCII byte outside quotes, or an empty command. A word that
     starts with `~` must have no quote before its first unquoted `/`:
     zsh expands `~'/x'` and bash does not.
  3. Each command starts with an unquoted `git`, so an env assignment and a
     wrapper (`sudo`, `command`, `env`, `time`) mean no answer. Exactly one
     `-C <dir>` follows, and the subcommand comes right after it, so `-c`,
     `--git-dir`, `--no-pager` or a second `-C` mean no answer.
  4. The subcommand is `status`, `log`, `diff`, `show`, `rev-parse` or
     `ls-files`, or a listing form: `branch` with only `--list`, `-a`,
     `--all`, `-r`, `--remotes`, `-v`, `-vv`, `--verbose` or
     `--show-current` (a positional only beside `--list`, where it is a
     pattern), bare `remote` or `remote -v`, and `worktree list` with
     `--porcelain`, `-v`, `--verbose` or `-z`.
  5. No argument may be risky: a long option that is any prefix of
     `--output`, `--ext-diff`, `--textconv`, `--no-index`, `--exec`,
     `--upload-pack`, `--receive-pack`, `--open-files-in-pager`,
     `--show-signature`, `--help`, `--exclude-from` or `--resolve-git-dir`
     (git accepts abbreviations, so `--exclude=<pattern>` counts too); a
     short-flag word holding `O` or `X` (`-O<file>`, `-X<file>`, bundled
     `-pO<file>`); a `%G` signature placeholder; or a positional argument
     or `--option=value` value that is absolute, starts with `~`, or has a
     `..` segment (`git diff` with a path outside the tree runs as
     `--no-index`). Arguments are judged by their text: a relative one
     that names a symlink in the work tree can still make git read the
     file the link points to, as with `ls-files
     --exclude-per-directory=<name>`.
  6. The dir is joined to the session cwd when relative, a leading `~` or
     `~/` is expanded, and symlinks are resolved the way git resolves them,
     `..` after a symlink included. The real dir must exist.
  7. The repo git would find from the dir must be one belt can vouch for,
     found the way git's own discovery walks, without running git:
     - No directory from the dir up to the top of the work tree looks like
       a git directory: a `HEAD` beside `objects`, `refs`, `commondir` or
       `config`. git would use such a directory, its config included, and a
       clone can carry one as tracked files.
     - The top is the first directory with a `.git` entry. That entry is a
       real directory git accepts (a valid `HEAD`, `objects` and `refs` as
       real directories, no `commondir`), or the `.git` file `git worktree
       add` writes: `gitdir:` naming `<main>/.git/worktrees/<name>`, whose
       `commondir` leads back to `<main>/.git` and whose `gitdir` leads back
       to this file. A symlinked `.git`, an empty or broken `.git`
       directory (git walks past it to a repo further up), a submodule's
       `.git` file, or a `.git` file naming anything else is no answer.
     - The work tree top and every git directory git reads sit inside a
       root: the `.git` directory, or for a linked worktree its own git
       directory and `<main>/.git`. A worktree under `~/.worktrees` whose
       main checkout lives under `~/code/src` needs both folders in the
       roots.
     - The repo's `config` and `config.worktree` set no `core.worktree`,
       no `core.bare` other than false, no `core.hooksPath`, and no
       `include` or `includeIf` section. A file the scan cannot read with
       certainty (a line continuation, a malformed section header or key,
       over 1 MiB) counts as setting one.
     - belt's own environment sets none of `GIT_DIR`, `GIT_WORK_TREE`,
       `GIT_COMMON_DIR`, `GIT_INDEX_FILE` or `GIT_OBJECT_DIRECTORY`.
  8. The roots are the work tree the session's cwd is in, unless that is
     home or a directory above it, plus `allows.git-c-read.roots`. Each
     root is resolved the same physical way as the dir; one that is `/`,
     home or above home, or that does not resolve, is dropped with a doctor
     warning.
- **Why it exists**: Claude Code allow rules match the command text as
  written. `Bash(git status:*)` never matches `git -C <path> status`, and
  `Bash(git -C * status *)` also matches `git -C x -c core.fsmonitor=<cmd>
  status`, which runs code.
- **Residual risk**: any repo under a root can carry config or an embedded
  git directory from wherever it came. The check refuses embedded git
  directories and the config keys above, and trusts the rest of a repo's
  own `.git/config`, which a clone never writes: the user or a tool on the
  machine did. The read commands still run code that config names: `git
  status` and `git diff` run `core.fsmonitor` and clean filters, `git
  status` runs the `post-index-change` hook when it refreshes the index,
  `git diff` runs `diff.external` and diff drivers, `git diff`, `git log
  -p` and `git show` run textconv filters, and a partial clone fetches
  missing objects. A tracked `.gitattributes` picks which configured
  driver or filter applies. `Bash(git status:*)` already accepts all that
  for the cwd repo; the roots widen it to every repo under them, so keep
  out of them any tree that carries its own `.git` directory without being
  a clone, such as an unpacked archive. belt sees only its own environment,
  so a `GIT_DIR` that the session's shell sets in an rc file is out of its
  sight.
- **Fails toward the prompt**: an allow that is off, a call it cannot read,
  or a dir or repo it cannot vouch for is no answer, never a deny. `belt
  check` does not run the allows.

## Hints

Hints add advisory context next to a tool result or at session boundaries.
Each exists because an instruction in a CLAUDE.md file proved unreliable at
the moment it mattered; a hook fires deterministically where prose gets
skimmed.

Configuration-wise almost every hint is one switch: `hints.<id>.enabled` in
the belt config, default on. The repo-aware hints (commit-policy,
lint-policy, prefer-csl, moondb-inflight) also read an
`exclude_repos` list, and humanizer-check reads an `exclude_tools` list. Hints opt repos out with `exclude_repos`;
`allow_repos` belongs to guards, and each key on the wrong kind is a
validation error. commit-policy additionally honors the shared
`direct_main_repos` list, and read-size takes its threshold from the
top-level `large_file_lines`, shared with the large-read guard. Everything else
about hint behavior is fixed
(see Fixed constants below). The guards carry the richer per-guard keys —
each guard section above names its own, and [config.md](../config.md)
specifies them all. A `custom_hints` entry registers an external command as
a session-start or subagent-start hint beside the built-ins (see Custom
hints below).

A payload belt cannot read runs no hint at all: nothing reaches the model,
and a `warn` event tagged `decode_error` records why (see Wiring above).

### Tags on hint events

Every hint fire posts an `info` event tagged `hint` (the hint id), `event`
(the hint event), and the session identity (`session_id`, plus `agent_id`
and `agent_type` inside a subagent). The hook sets all five; a hint adds
none of its own. Two kinds of fire post nothing: `agent-memory`, and a
custom hint set to `skip_event`. Both inject the same context on every
start, so they carry no signal and would push guard history out of the
events cap.

### agent-memory (session-start)

Injects the shared agent-memory index when a session opens.

- **Reads** `~/.config/agent-memory/MEMORY.md` (every machine) and
  `~/.config/agent-memory-work/MEMORY.md` (machines that have one), keeps
  only the fact bullets, and injects them as session context. A missing store
  renders nothing, so a personal machine never sees a work index.
- **Caps** the whole injected block at 9,000 bytes: the `belt[agent-memory]`
  prefix, each store's header, its fact lines and any truncation note.
  Claude Code hands the model at most 10,000 characters of one hook's output
  and swaps anything longer for a short preview. The budget keeps the memory
  block alone under that line. Every session-start hint renders into the same
  output, so when other hints fire with it (moondb-inflight, custom hints)
  and are large, the combined output can still pass the cap.
- **Holds a quarter for the work store** when both stores have facts. The
  personal store renders first, and without the reserve a large personal
  index would leave the work index a few hundred bytes. A store that needs
  less than its share hands the rest to the other.
- **Cuts a store past its share** at the last fact that fits, and its note
  names that store. `belt doctor` shows each store against its share, warns
  at 80% of the budget, and lists a cut store by name in its warnings
  section.
- **Leaves out a store Claude Code loads itself**, but only when the entry
  runs `belt hint session-start --agent claude-code`. Claude Code's
  `autoMemoryDirectory` setting makes it load that directory's `MEMORY.md`
  on its own. When the setting names a store's directory, the hint skips
  that store, so its index does not load twice. belt lets
  `~/.claude/settings.local.json` win over `~/.claude/settings.json`. Claude
  Code's settings docs list only `settings.json` as the user file in
  `~/.claude`, and name `settings.local.json` only inside a project. A leading `~` is expanded and symlinks are resolved
  on both sides, so a store linked into a synced folder still matches. The
  store left out counts as absent, and the other store gets the whole
  budget. A missing setting, broken JSON, or a path that does not resolve
  keeps the store. Without the flag, or with any other value, both stores
  are injected, which is what Codex and pi need. `belt doctor` prints one
  line for a store the flag would leave out. The line also says when a
  SessionStart entry in the Claude settings runs the hint without the flag.
  It reads the entry the way belt parses its arguments, so the flag may sit
  before or after the event, a `--config` may sit before `hint`, and the
  binary may be named by a full path. Install a belt that knows the flag
  before a settings entry passes it: an older belt fails on the unknown flag
  and the session starts with no session-start hints.
- **Posts no event** per fire: it injects the same text every session.
- **No per-session dedupe** on purpose: the index is the thing to see every
  session.
- **Why**: "read the memory index at session start" as instruction prose
  triggers sometimes. A SessionStart hook triggers always.
- **Setup**: the store is a git clone you create yourself; belt only reads
  it. A worked recipe — clone item, index format, and a fact-file template —
  is in `examples/dotfiles/recipes/agent-memory/` at the repo root.

### moondb-inflight (session-start)

Lists the files of the session's checkout that other sessions edited and no
commit has taken yet.

- **Resolves** the checkout root from the session's working directory with
  `git rev-parse --show-toplevel`. It asks the local moondb service for up
  to 200 files there that hold edits no commit has taken, leaving out the
  session's own. moondb only links a commit a session saw land, so the hint
  then runs one `git status` over the named paths and keeps the ones git
  reports dirty. It lists up to ten of those, each with the sessions behind
  it and the age of its last edit. Each git call gets 1 s.
- **Why**: a session that commits with `git add -A`, stashes or resets can
  sweep up or throw away another session's work in the same checkout.
  Nothing told it that work existed.
- **Silent when** the payload has no session id or the session is outside
  a repo. It is silent when moondb is down (400 ms budget) or older than
  its `/api/inflight` route, and when the answer is empty or doesn't
  decode. It is also silent when a git call fails or runs out of time, or
  every named file is clean.
  `belt doctor` tells "moondb down" and "moondb too old" apart; the hint
  renders both as silence.
- **No per-session dedupe**: a resumed or compacted session gets the
  current state.
- **`/clear` starts a new session id.** The edits made before the clear
  belong to the old id, so the hint lists them as another session's.

### prefer-csl (bash)

After a Bash command sweeps multiple files inside a repo csl already indexes,
or inside a linked git worktree of one, hands back the equivalent `csl search`
command, then the `csl_search` call.

- **Fires when** a `grep`/`rg`/`ag`/`git grep`/`find`/`fd` command recurses,
  or names a glob or three or more files, inside an indexed repo. With
  several paths, the first one inside an indexed repo counts. The index
  check reads csl's shard listing on disk. No csl process is launched,
  because this runs on every Bash call.
- **Names the repo the command searched**, not the session's. Each segment's
  paths resolve against the directory it runs in. A leading `cd <path> &&`
  or `pushd <path> &&` moves that directory for the segments after it, and
  `popd` moves it back. A `cd` inside a subshell ends with the subshell; a
  `cd` inside a `{ ...; }` group stays in effect, as in the shell.
  `git -C <path>` moves it for that git command, and an absolute path
  argument names itself. The session's cwd is the fallback. When the
  directory can't be known without running the shell (`cd -`, `pushd +1`, a
  variable, a command substitution, `~user`), the hint stays silent rather
  than guess.
- **Deliberately silent** on pipe filters (`cmd | grep x`) and on paths
  outside an indexed repo. A grep of one or two named files stays silent
  too, with or without `-r`, since it reads known files. So does `rg` or
  `ag` with no path, which may be reading stdin. None of those is what csl
  replaces. Redirections (`2>/dev/null`, `2>&1`, `> out`, `< file`) are
  neither a pattern nor a path, so `rg foo < file` counts as no path.
- **Known gap: heredoc bodies.** The command is split on newlines, so a
  heredoc body is read line by line as commands, and a `grep -r` line inside
  one can fire.
- **The advice is runnable, CLI first**: `csl search '<query>' --repo
  '^<org>/<repo>$'` with the pattern translated to zoekt syntax, then a note
  on how zoekt differs from grep, then the same query as a `csl_search`
  call. The repo is anchored in both forms, so `org/repo` never also matches
  `org/repo-fork`. The translation follows the dialect the tool read.
  grep's basic regex swaps `\|` and `(` for `|` and `\(`, and `-F` escapes
  every metacharacter. A pattern with a space is quoted as one phrase
  rather than read as two AND terms. A plain-extension glob from
  `--include *.go`, `rg -g` or `--glob`, `find -name` or `fd -e` becomes
  `f:\.go$`. An fd pattern becomes an `f:` filter, and repeated `-e`
  patterns join as `a|b`.
  `find . -name '*.go' | xargs grep x` folds the grep into one query,
  `x f:\.go$`. A file listing with nothing to translate gets a
  `csl search 'f:<regex>'` template and the `csl_ls` call. The CLI leads
  because it runs in any session with csl on PATH and has no per-call
  result cap. A bare "use csl" would cost a turn while the model guesses
  the query language.
- **A linked worktree gets its own index.** csl's main index holds each
  repo's main checkout, so `--repo` would answer from the default branch. A
  path inside a linked worktree (its `.git` is a pointer file whose git dir
  holds `commondir`) gets `csl search '<query>' --worktree '<root>'` and
  `csl_search worktree="<root>"` instead, which search the worktree's own
  index. The repo comes from the worktree's origin remote, since the
  `~/.worktrees/<repo>/<slug>` directory names match no shard, and the hint
  fires only when csl indexes that repo. A listing with nothing to translate
  gets `csl_search worktree=... query="f:<regex>"`, since `csl_ls` takes a
  repo only.
- **Once per checkout per agent per session.** A marker in the session's
  seen file, keyed like read-size's on the agent id plus the repo name (a
  worktree by its root), stops the hint repeating on every grep in the same
  checkout. A subagent gets its own word. Without a session id nothing is
  recorded and the hint speaks on every match.
- **Wording follows the search**: a recursive search "re-walks the
  filesystem", and a grep over three or more named files is "a grep over
  several files".
- **Why**: a transcript audit found 97% of recursive greps ran inside indexed
  repos. Habit, not ignorance, so the fix is a well-timed reminder with the
  exact replacement, not a deny.

### read-size (read)

After a Read or a `csl_read` pulls in a large file without a range, points
at the cheaper ways in.

- **Fires when** a Read with no `offset` and no `limit` returns a file of
  `large_file_lines` or more (300 by default) inside a repo csl indexes. The index check is the same
  shard listing prefer-csl reads. The file must be a type `csl_outline`
  has symbols for: Go, TypeScript, Python, Java, protobuf, shell, or
  markdown.
- **Also fires on `csl_read`** with no `start_line` and no `end_line`,
  under the same rules. A whole-file `csl_read` returns at most 500 lines,
  but the hint goes by the file's length, so `lines 1-500 of 843` counts.
  The call's `repo` is a regex, so the file and its length come from the
  response. csl writes it in the text form, which opens with the header
  line `org/repo/path lines A-B of N`, or the json form, an object with
  `repo`, `path` and `total_lines`. belt reads either form from content
  blocks (the first text block) or from a JSON string. A text form result
  arrives as content blocks. A json form result also carries structured
  content, and Claude Code records such a result as one JSON string. belt
  also accepts the json object itself. Other response formats stay silent.
  The json form states the repo and the path apart, and belt keeps them
  apart. The text header joins them, and csl names a repo after its origin
  remote, which can hold more than one slash. So belt splits a header's
  name at the longest repo name it has a shard for. A bound of 0 counts as
  no range, as csl reads it. The matcher has to name the tool:
  `Read|mcp__csl__csl_read`.
- **The advice** names the file and its line count. It hands over a
  `csl_ask` call first, for when only an answer is needed. The question is
  a placeholder, and the file goes in as an `f:` query on its path. csl
  reads the path and the repo as regexes, so both are escaped and
  anchored. Then comes the `csl_outline` call with repo and path filled
  in, and `csl_read` or a ranged Read for a region the agent will edit,
  which needs the real lines. The outline call always sets
  `include_tests: true`, because csl leaves test files out of an outline by
  default. For markdown it adds `kinds: ["section"]`, since headings are
  left out otherwise.
- **Once per file per agent per session**, through the seen file. The
  marker names the file by repo and repo-relative path, so a Read and a
  `csl_read` of one file share a nudge. A subagent shares its parent's
  session id, so it gets its own nudge, and its read doesn't use up the
  main thread's. A ranged read of the file doesn't use one up either.
- **Silent** without a session id, on a relative path, and outside an
  indexed repo. It's silent on every file type `csl_outline` returns no
  symbols for: JSON, YAML, TOML, plain text, CSV, lock files, JavaScript,
  Dockerfiles and the rest. It's also silent on a response it can't read:
  an image, notebook or PDF read, or a line count that is missing or of
  another JSON type. A loom worktree
  has no shard of its own, so reads there stay silent too. That is on
  purpose, since `csl_outline` would describe the indexed checkout, not the
  worktree's files.
- **Why**: a whole-file read of a large file puts every line in context
  when the task usually needs one region or one answer. A question to
  `csl_ask`, an outline or a ranged read gets there for a fraction of the
  tokens.
- **Beside the large-read guard**: a whole-file Read the guard refuses
  never runs, so the hint never sees it. The hint still speaks for the
  reads the guard lets through, such as a file the agent edited, and for
  every `csl_read`, which the guard does not watch.

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

### humanizer-check (external-text)

Once per session, after an MCP call publishes text somewhere other people
read, points at `humanizer_detect` while the wording is still editable.

- **Fires when** a matched MCP tool call publishes (the matcher is the
  consuming repo's choice; belt drops read-verb operations itself). It fires
  only if the session has not already run any humanizer tool and the nudge
  has not fired before. A tool named in `hints.humanizer-check.exclude_tools`
  is skipped without spending the nudge: the consuming repo lists there the
  tools another check already scans, such as the PR tools the
  humanizer-gate mod covers.
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
- The budget is 400 ms by default; `timeout_ms` sizes it per
  hint. Both start hooks run before a first prompt, the session's or the
  subagent's. So the budget is the whole cost a slow external can impose,
  plus a short grace for closing the pipe and landing the warn event. On timeout belt kills the external's
  whole process group, so a hung grandchild (a `curl` inside the script)
  does not outlive the hook either.
- `agent_types` limits a `subagent-start` hint to the subagent types it
  names, matched against the payload's `agent_type`: exactly, or by prefix
  for an entry ending in `*`. A spawn of any other type skips the hint:
  belt starts no process and logs no warn event, because the hint declined
  on purpose. Without the key the hint runs for every subagent. A brief
  meant for one kind of subagent is the case it exists for. A named spawn
  reports its name as its `agent_type`, so an entry for named spawns ends
  in `*`: `reviewer-*` covers a spawn named `reviewer-deep-<slug>`. A brief
  whose entry lists `reviewer-*` and `docs-writer` skips every other type,
  so an Explore or general-purpose spawn gets nothing and leaves no warning
  on the events timeline. The SubagentStart hook
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
hint for every subagent, so a brief meant for reviewers reaches every
spawn. Run `ralph up` first and change the config second. Keep any agent
type check the script already does until every machine runs the new belt.
On an old binary that check makes the hint decline, which costs only the
old warn events. A `*` entry has the opposite skew: a belt older than `*`
matching reads it literally, so the brief reaches no subagent until the
fleet runs the new belt.

## Ledger

`belt record <event>` turns Claude Code lifecycle events into entries on the
events service, so a session and the subagents it spawns leave one
timeline. It is the third concept beside guards and hints: an entry
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
  A named spawn reports its name as its `agent_type`, so a key for named
  spawns ends in `*`: `reviewer-deep*` covers `reviewer-deep` and
  `reviewer-deep-<slug>`. The map restates the definition, belt never
  infers a value. The loom-pane member line reads these two tags. A
  malformed entry or value drops its own tag, with a `belt doctor` warning,
  and never fails the config file (see `config.md`). Run `ralph up` before
  a machine config names the key: an older belt ignores unknown ledger keys
  and records no model or effort.
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
  git-identity, commit-guard) run
  `git remote get-url origin` in the relevant directory and canonicalize to
  `host/owner/repo`. Identity is the remote, never the filesystem path, so a
  second checkout or a ralph source-cache clone of the same repo is treated
  identically.
- **Walk up, for writes.** `write-internal-names` resolves the target file
  through `nearestExistingDir` — walking upward, because a Write may create
  its parent directories first — and then reads that directory's origin
  remote. A `github.com` remote is the entire public/private test.
- **Read another tool's artifacts, for csl.** `prefer-csl`, `read-size`
  and `large-read` never ask csl anything; they list csl's shard directory
  (`~/.config/csl/search-index/`) to learn which repos are indexed, and
  name the repo containing a path as `<parent-dir>/<repo-dir>` to match
  csl's shard naming. `large-read` names a linked worktree's main checkout
  the same way, found through the worktree's `commondir` file.
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
| moondb query budget | 400 ms | moondb-inflight |
| git budget per call | 1 s | moondb-inflight |
| events POST budget | 1 s | every deny and hint event |
| custom guard timeout | 5 s | custom guards |
| custom hint default budget | 400 ms | custom hints (`timeout_ms` overrides it per hint) |
| script size cap | 1 MiB | script-deny-list |
| temp folders | `/tmp`, `/var/tmp`, `/private/tmp`, `/private/var/tmp`, `~/.tmp` | rm-temp (the folders `toss-bin --temp-only` accepts) |
| memory block budget, prefix and headers included | 9,000 bytes | agent-memory |
| share held for the work store when both have facts | 2,250 bytes | agent-memory |
| in-flight files asked of moondb | 200 | moondb-inflight |
| in-flight paths per fire | 10 | moondb-inflight |
| published-text excerpt | 200 bytes | humanizer-check |
| session dedupe store | `~/.cache/belt/seen-<session>` | all once-per-session hints |
| agent-memory store paths | `~/.config/agent-memory[-work]/MEMORY.md` | agent-memory |
| csl shard listing | `~/.config/csl/search-index/` | prefer-csl, read-size, large-read |
| csl index file size limit | 2 MiB (`kit/cslignore.SizeMax`, the limit csl's index takes) | large-read |
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
   the table above; `belt doctor` checks only the read matcher. If belt is wired
   but every hint on one event stays silent, look for `warn` events tagged
   `decode_error`: that event's payloads do not decode.
2. **One guard never denies.** Run the case through `belt check bash "<the
   command>"`, `belt check write --file <path> --content <text>` or
   `belt check read --file <path> --transcript /dev/null` — one
   verdict line per guard shows whether the guard matched and why it decided
   as it did. Then `belt config` shows which file and key produced that
   state.
3. **A guard allows something you expected it to block.** Check the events
   timeline: soft-mode rules and broken custom guards allow with a `warn`
   event rather than denying silently.
4. **moondb-inflight is silent.** `belt doctor` reports whether moondb
   answers and whether it is new enough to have the in-flight route. The
   hint renders both failures as silence.
5. **A hint fired once and never again.** That is the once-per-session
   contract (humanizer-check, lint-policy). A new session resets it.
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
9. **read-size fires after a Read but never after a `csl_read`.** The
   `belt hint read` matcher leaves the tool out. `belt doctor` prints a
   `WARNING` line in its hook wiring section with the matcher to set.
10. **A `git -C` read still prompts.** `belt doctor` shows whether
    git-c-read is enabled, which roots it uses and which it dropped. Then
    check the call against the git-c-read rule above: one pipe, redirect,
    `$`, unquoted glob or `^`, or a dir that resolves outside the roots
    means no answer. So does a linked worktree whose main checkout is under
    no root, and a custom guard in soft mode or one that times out or
    fails: the events timeline shows its warning. A settings ask rule that
    matches the call wins over the allow.
