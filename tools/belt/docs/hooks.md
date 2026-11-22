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
  lines of ignored text. A guard in hint mode (main-checkout) lets the call
  through and writes its reason as PreToolUse `additionalContext` instead,
  with no permission decision.
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
  service (`events.this`): denies and soft-mode hits as `warn`, hints and
  hint-mode guard advice as `info`. Each event is tagged with the session
  id on every real call, and with the agent id and type when the call came
  from inside a subagent.
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
  (`git --no-pager push`). Bare `git push` and `HEAD` or `@` refspecs resolve
  the current branch via `git rev-parse --abbrev-ref HEAD` in the push
  directory. A `heads/<name>` side reads as `refs/heads/<name>`, as git
  expands it, so `git push origin heads/main` targets main.
- **A checkout or switch earlier in the command counts.** belt reads the
  current branch before the command runs. So a `git checkout` or `git switch`
  before the push changes what the current branch means. A switch to `main`
  or `master` counts as that branch, so `git checkout main && git push` is
  denied from a feature branch, and the denial names the checkout. A switch
  belt cannot read counts as unknown, and a push that needs the current
  branch after it is denied. That is a variable or substitution, `-`,
  `--detach`, or an option belt does not know. A bundle of short options
  such as `-qc` reads letter by letter when belt knows every letter. The
  branch git reads before the command still counts too, since the switch may
  fail. The exception is a sure switch: `git switch`, or `git checkout` with
  `-b`, `-B`, `--orphan` or `--track`. A plain `git checkout <name>` is sure
  too when the repo has a local branch `<name>`, because git then always
  reads it as that branch. Without one, `<name>` may be a path and the
  checkout a file checkout. To check, belt runs one git call, `git
  for-each-ref refs/heads/<name>` in the push's own directory, with a 2
  second timeout. An error or a timeout counts as no branch. A sure switch
  replaces the branch only when all of these hold:
  - It runs as git itself. Only assignments or a wrapper such as `env` stand
    before the git word, not `echo` or `:`, and no `!` negates it, since
    `! git switch -c x && git push` pushes only when the switch failed. A
    wrapper that may run it beside the shell does not count (`coproc`,
    `setsid`), since the push may then run before the switch ends.
  - It is not itself after `||` or in a pipeline, and only `&&` joins it to
    the push, so the push runs only after the switch worked.
  - It is the last switch in the command, and every step between it and the
    push is on the list below.
  - Every step after the push is on the list too, since one after a `&` or
    in a pipeline runs beside the push.
  - No shell function is defined anywhere in the command, as
    `name() { ...; }` or `function name { ...; }`, since one may stand in
    for git or for a step on the list.

  One list says which steps leave HEAD alone, before the push and after it:
  git `add`, `commit`, `fetch`, `pull`, `merge`, `status`, `log` and `diff`
  run as git itself, `git rebase` with at most one upstream and no `--root`,
  `--exec` or `-i`, a whole `git branch --show-current`, `git rev-parse
  --abbrev-ref HEAD` or `git symbolic-ref --short HEAD`, and the read-only
  commands belt knows (`echo`, `printf`, `cat`, `head`, `tail`, `less`,
  `more`, `grep`, `rg`, `ls`, `stat`, `wc`, `diff`, `file`). A git step on
  the list does not count when it carries a `-c` override of its own, an
  `--output` option, or a word that names a path inside `.git`. Anything
  else keeps the branch from before the command as a candidate, a command
  belt does not know included: a git alias, `git branch -M`, a script, a
  test run. So does a step that writes into `.git`. The denial names the
  step.

  So `git switch -c feature && git push -u origin HEAD` passes on main, and
  the same with `;` is denied. So is `git switch -c feature && git push -u
  origin HEAD && gh pr create`: open the PR in a command of its own. A file
  checkout (`git checkout -- <path>`, `git checkout <ref> -- <path>`) moves
  no branch. A switch counts only for a push in the same directory, the same
  `git -C` target or the same `cd` state. One in a subshell counts, because
  it moves the repository and not the shell. One in another directory or in
  one belt cannot tell adds no branch for the push. When it is the last
  switch in the command, though, an earlier switch in the push's own
  directory is no longer sure, so the branch from before the command counts
  again. In an exempt repo a switch never turns an allow into a denial: belt
  judges such a push on the branch git reads, as if the command had no
  switch. A switch in a comment or a text heredoc body counts not at all.
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
  `read`, or `source`. Two words that name where the shell is are placed
  too, at the start of a cd or `-C` target. `$PWD` (or `${PWD}`) is the
  directory belt tracks at that point. `$(git rev-parse --show-toplevel)`,
  with or without its own `-C`, is the top of the repository there, found
  the way git finds it without running git. Each stays unknown where belt
  cannot tell: from an unknown directory, outside a repository it can read,
  as an unquoted `$PWD` whose value holds a space, or as a top whose name
  holds a newline or other control character, which the shell strips from
  the end of a substitution. It also stays unknown
  in a command that names `PWD` other than as `$PWD` or `${PWD}`, since an
  assignment may give it another value. Belt places either word only in the
  top-level command text. In nested script text it stays unknown: a
  subshell, a `-c` script, an `eval`, or a heredoc or here-string a shell
  runs. There the outer shell may expand it before the script's own cd
  runs. It also stays unknown after any command other than a `cd`,
  `pushd`, `popd`, a git call or a plain read such as `echo` or `ls`.
  Another command may move the shell where belt does not see it: a
  function, `trap`, `source`, `emulate` or `setopt autocd`. `printf -v`
  may set `PWD` under a name built at run time, so `printf` counts as
  another command. Any `${...}` other than a plain `${PWD}`, and any
  `$((...))` or `$[...]`, anywhere in the command keeps both words
  unknown. Such an expansion may cd or assign `PWD` in the shell itself:
  bash's `${ cmd; }`, a `:=` or `::=` default, zsh's `(P)` flag.
  A target only a shell can
  resolve (any other variable, `cd -`, any other command substitution, a
  glob) leaves the directory unknown. So do `popd`
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
- **Reads a wrapped push.** The guard walks a quote-aware reader, so a push
  inside a `( … )` subshell, a `$(…)` or backtick substitution, an
  `sh -c`/`bash -c` string, an `eval`, a `\`-continued line, or a `{ … }`
  group is judged like a plain `git push`. An `eval`, a shell that reads
  its script and an `xargs` count wherever they stand in a command: in a
  `{ … }` group, an `if`, a loop or a `case` arm, behind `!` or `time`, and
  in a function's body, which is read where the function is defined,
  called or not. A backtick body loses one level
  of `\$`, `` \` `` and `\\` escapes before it is read, so a push in nested
  backticks (`` `echo \`git push …\`` ``) counts at every level. Inside
  double quotes it loses the backslash of `\"` too, as bash and zsh drop
  it there, so in ``"`echo \"'\"; git push …; echo \"'\"`"`` the push is
  not inside quotes and counts. A backslash-newline between `$` and `(`
  joins them in a bare word, a double-quoted string and a here-string, as
  bash reads them, so `"$\` at a line end and `(git push …)"` on the next
  line is a substitution (zsh reads it as text). In single quotes it is
  text. When the
  reader cannot follow a command, the whole lines before the line it fails
  on get the full walk, because bash runs them before it meets the syntax
  error. The rest is read fail-closed: a git word and a push there are
  denied unless the refspecs name a non-default branch in plain text.
  Nesting deeper than 32 levels is read the same way.
- **Reads git aliases and text printed into a shell.** A git alias the
  command sets is read in place of the subcommand. belt learns an alias
  name only where the command sets config: an invocation's own `-c
  alias.<name>=` or `--config-env=alias.<name>=`, a `GIT_CONFIG_KEY_<n>` or
  `GIT_CONFIG_PARAMETERS` assignment, or the key of a `git config` write
  earlier in the command. A config read (`--get`, `--list`, `get`), an
  `--unset`, and a mention of `alias.<name>` anywhere else (a commit
  message, a `grep` pattern, a log filter) name no alias. git runs its own
  builtin commands before any alias, so `git status`, `git push` and the
  other builtins never read as one. A body that starts with `!` is read as
  shell text. So `git -c alias.p=push p origin main` and `git config
  alias.p push; git p origin main` are denied like the plain push. Text that
  `echo`, `printf`, or a `cat` of a heredoc or here-string prints is read as
  code when a shell runs it. That covers a pipe (`echo '...' | sh`), a
  here-string (`sh <<< '...'`), a `<( )` operand (`bash <(echo ...)`,
  `. <(...)`) and a substitution in a `-c` script or `eval`. An `xargs`
  that runs its items as a shell's script reads them the same way:
  `printf '%s' '...' | xargs -0 sh -c` runs the text, and `xargs -I{} sh
  -c '... {}'` runs the script with each item in place of `{}`. belt cuts
  the items as xargs does (at a NUL after `-0`, at the `-d` character,
  else at newlines and blanks with quotes taken off, and at newlines alone
  with `-I`) and reads each on its own. A backtick
  printer is read with its escapes taken off, as the push in it is read, so
  `` eval "`echo \"true; git push …\"`" `` runs the push. It also covers
  a variable the command set to such text (`X=$(cat <<'EOF' ...); eval
  "$X"`, or `$X` as the command word), and a file the command wrote and then
  runs (`cat <<'EOF' > x.sh && sh x.sh`).
- **Denies what it cannot read there.** Two causes are denied on any
  branch, in an exempt repo too: an alias the command sets without a body
  belt can read (a body from a variable or `--config-env`, or a config key
  that is itself a variable), and text that keeps building more text to run
  past 64 texts in one command. Everything else belt cannot read is denied
  only in a repo with no direct-push exemption, or from a directory belt
  cannot tell:
  - Shell text: printed text that holds a variable or a command's output,
    and any other command piped into a shell or into an `xargs` that runs
    its items as code (`cat x.sh | sh`, `curl x | sh`, `find . | xargs
    -I{} sh -c '...'`).
  - A Python, Perl, Ruby, Node or osascript script that starts a process
    (Python `subprocess`, `os.system`, `os.popen`, `os.exec*`, `pty.spawn`;
    Perl `system`, `exec`, backticks, `qx`, `open` with a pipe; Ruby
    `system`, `exec`, backticks, `%x`, `spawn`, `IO.popen`, `Open3`; Node
    `child_process`, `exec*`, `spawn*`; osascript `do shell script`) and
    names `git`, or a path to it, followed by `push`. A script that starts
    no process, such as a Perl `s/git push/.../` or a print of usage text,
    passes. One that runs `git status` beside a comment that mentions `git
    push` is refused in a guarded repo, since belt cannot read the
    language.
  - A script argument a `-c` script, a sourced file or a `!` alias body
    runs as code (`sh -c 'eval "$1"' _ '...'`, `"$@"` as the command), and
    an argument an `eval` or a `-c` runs as its text after a function
    definition, where it may be the function's body (`f() { eval "$1";
    }`). A function that runs an argument as a command (`"$@"`) is read at
    the call, whose words show it.
  - A variable or a written file past 64 KiB, or text past 512 KiB in all
    of one command: a variable doubled a few times over grows past any
    size, so belt stops reading it instead.

  Outside any git repo and in an exempt repo it passes, even when the text
  would push to another remote. belt asks git (`git rev-parse --git-dir`)
  from the nearest directory that exists, since a command may `cd` into one
  it creates first. Only git's own "not a git repository" answer counts as
  outside a repo; any other failure, or no answer within 2 seconds, counts
  as inside one. The denial says to write the text to a file and run it, or
  to run the command directly. A `$( )` or `<( )` of any other command
  keeps its old reading, so `eval "$(ssh-agent -s)"` and `source <(fzf
  --zsh)` pass. Still not seen: an alias set in a config file before the
  command, a script file already on disk, a variable from outside the
  command used as the command word, a script that builds the words at run
  time or starts a process under another name (`import os as o`), and
  `ssh`, `xargs sh -c` or `trap` bodies.
- **Heredoc bodies are text unless a shell may run them.** A body read by
  `cat`, `tee`, `head`, `tail`, `git commit` or a `gh pr`, `issue`,
  `release`, `gist` or `api` call is prose. So is a body `git tag`, `git
  merge` or `git notes add` reads as its message with `-F -`, `--file=-` or
  `-F /dev/stdin`, and one piped from `cat` into `gh pr create`, `comment`
  or `edit` or `gh issue create` or `comment` that reads its body with
  `--body-file -` or `-F -` and pipes nothing on. Its lines are read the old
  token way, so a line that starts with a push still counts, but a backtick
  or `$( )` in a quoted (`<<'EOF'`) body is text: a PR body that shows a
  push in a code span passes. In an unquoted body the shell runs every `$( )`
  and backtick before the command reads it, even after a `'` or `#`, so those
  are read as code and a push there is denied. belt finds them the way the
  shell expands the body. It drops each backslash-newline first, so `$\` at
  a line end and `(git push …)` on the next line is a substitution. A
  backtick substitution ends at the first backtick no backslash quotes, and
  its body loses one level of `\$`, `` \` `` and `\\` escapes, so
  `` `echo \`git push …\`` `` runs the inner push and is denied. A `\"`
  keeps its backslash there, since a body is not a double-quoted string:
  `` `echo \"; git push …; echo \"` `` runs the push. An escaped
  `\$(` or `` \` `` outside a substitution is text. Any other command, any
  other pipe, a `>( )` after the reader, and a substitution handed to
  `sh -c`, `eval`, `source` or an interpreter make the body code, read like
  the rest of the command. In an unquoted body a line ending in a
  backslash joins the next one before belt compares it with the delimiter,
  as bash and zsh do.
- **A git message is text, every line of it.** git stores the message and
  runs none of it, so no line of it counts, even one that starts with a push
  or a discard. That covers a `git commit -m "$(cat <<'EOF' ... EOF)"`
  value whose body the shell passes on as written (a quoted delimiter, or
  no `$`, backtick or `\` in it), and a heredoc `git tag` or `git notes add`
  reads with `-F -`, `--file=-` or `-F /dev/stdin`. So a commit message may
  name `git checkout -- f` or `reset --hard` in plain words. A heredoc that
  `git commit` or `git merge` reads with `-F -` gets no pass. Both refresh
  the index before they read the message, and a `core.fsmonitor` program
  runs then with git's standard input, so a shell may read the body. The
  pass needs a git belt can vouch for. The word `git` comes
  first (no path, wrapper or `VAR=` in front), with nothing but `-C <dir>`
  before the subcommand (`-c` or `-p` may bring in a pager or a config
  value), in top-level text. Every word after `git` is plain text the shell
  passes as written, since a variable, glob, brace or substitution
  (`$E`, `{-e,-q}`) may turn into an option. A quote or `$` in a word
  ends the pass too, since `'-em$'` is `-e` to git. Only a `-m` value may
  hold them, and only there is the quoted `"$(cat ...)"` let through. The heredoc is on standard input, not
  `3<<`. Only
  `cd`, a git call or a plain read such as `echo` runs before it; a
  function, alias, `source`, `exec` or `PATH` change ends the pass. The
  git call has no redirect but its heredocs and no pipe, so what git prints,
  which holds the subject line, cannot reach a shell. Every option is one
  belt knows starts no editor or prompt: for `git commit` that is `-q`,
  `-a`, `-s`, `-n`, `--allow-empty`, `--allow-empty-message`, `--amend` and
  `--no-edit`, for `git tag` `-a` and `-f`, and for `git notes add` `-f`
  and `--allow-empty`, besides `-m` and `-F`. The editor gets the message
  file and may be a shell, and `-p` reads its answers from standard input,
  so `-e`, `-p` and any option belt does not know end the pass. In the `-m` form
  bash 3.2, macOS's `/bin/bash`, must also run no more of the `$( )` than
  `cat` (`bash32Closes`, see the script-deny-list masking below). It knows
  no heredoc there, so a `)`, a lone `'` or a quote on a `#` line in the
  body can end the `$( )` early. A lone `'` that nothing after it in the
  command closes passes: bash 3.2 then stops at a syntax error and runs
  none of the command (`bash32Unclosed`). That holds for the command as
  typed only; in a nested text or a `-c` script it is read fail-closed. A
  `$(cat <<'EOF' ... EOF)` bash
  3.2 may run more of, in any command, is read fail-closed: a push or a
  discard anywhere in its body is denied. The reason (git-push-main,
  git-discard, file-discard) then points at the forms that stay text:
  `git commit -F -` (or `gh ... --body-file -`) with a quoted heredoc, or a
  message file. Any other message is read as the bullet above says.
  The `$( )` and backticks of an unquoted body still run, and so does
  anything else in the substitution.
- **A refspec must be plain text.** A refspec that holds a variable, a
  substitution, a `$'...'` string, a brace or a glob may name `main`, so the
  push is denied on any branch. Two shapes read as the current branch. The
  first is `"$(git branch --show-current)"` (or `git rev-parse --abbrev-ref
  HEAD`, `git symbolic-ref --short HEAD`) as the whole word, with no
  `git -C`. The second is a variable set to one of those forms:
  `B=$(git branch --show-current); git push -u origin "$B"`. The assignment
  must be a command of its own at the top level, before the push, joined by
  the start, `;`, a newline or `&&` and not piped. The name may appear nowhere
  else but as a whole refspec (`$B`, `${B}`, quoted or bare), and it may not
  be one the shell manages (`PATH`, `HOME`, `PWD`, `IFS` and so on). The
  command may hold no `cd`, `pushd` or `popd`, no `read`, `export`, `unset`,
  `eval`, `source`, `.`, loop or `if` keyword, no `{ }` group, and the push
  no `git -C` or `--git-dir`. After `&&` the name must also be unset in the
  environment, since the assignment may not run. So both shapes pass from a
  feature branch and are denied on main. `"$BRANCH"` with no assignment is
  denied.
- **A push that sweeps branches counts as a push to main.** `--mirror` and
  `--all` (or `--branches`) push every branch, with or without force. So
  does the matching refspec `:` or `+:`, which pushes every branch both
  sides have. belt judges each like `git push origin main`. git reads any
  unique prefix of a long option, so `--mir` and `--al` count too. belt
  reads the options the way git does. A bundle of short options reads
  letter by letter. A value option takes the next word, even `--`. A `--`
  or `--end-of-options` ends the options. An exempt repo still takes
  `--mirror`, `--all` and `:` when the push plainly goes to origin.
- **Config that picks what a push sends counts too.** A push that names no
  refspec counts as a push to main when config may pick what it pushes.
  That is a `-c` or `--config-env` for `push.default` (any value but
  `simple`, `current` or `nothing`), `remote.<name>.push`,
  `remote.<name>.mirror` or `branch.<name>.merge`. An `include.path` or
  `includeIf.<condition>.path` counts too, since the file may set any of
  them. It is also a command that sets config another way: a
  `GIT_CONFIG*`, `HOME` or `XDG_CONFIG_HOME` assignment, `sudo` in front of
  git, or a step that drops the exemption below. Either way the exemption
  is gone. belt reads only the command, so config set in an earlier command
  is out of its reach.
- **A refspec with no `:dst` can be mapped.** git sends a refspec like
  `feature` where `remote.<name>.push` maps it, or to its upstream when
  `push.default` is `upstream`. So such a push counts as a push to main
  under a `remote.<name>.push` whose destination may be main, a
  `push.default` of `upstream` or `tracking`, a `branch.<name>.merge`, an
  include, or config the command sets another way. A refspec that names its
  destination, or a `--delete`, is judged on what it names.
- **Options and the remote must be plain text.** A variable, a
  substitution, a `$'...'` string, a brace or a glob in an option, an option
  value or the remote may turn into `--mirror` or a refspec. So may one in
  the subcommand word when a `push` follows it, as in `git $X push`. The
  push is then denied on any branch, in an exempt repo too.
- **Allows when** the push directory's origin remote resolves to a repo on
  the top-level `direct_main_repos` list or on
  `guards.git-push-main.allow_repos`, and the push plainly targets `origin`.
  Both take an exact `host/owner/repo`
  or a trailing `/*` org wildcard, the same patterns `git_identity[].repos`
  takes. A machine class where direct pushes are fine (a personal machine,
  say) disables the guard in its rendered config instead. belt reads no
  machine profile, from ralph or anywhere else (docs/adr/0010).
- **The exemption is for the repo the session is really on.** A push to a URL
  or a second remote name, a `--repo`, a `-c remote.origin.pushurl=` or
  `-c url.*.pushInsteadOf=`, or a `GIT_CONFIG*`/config-env override in front
  is not exempt and is denied even on an allowlisted repo. A bare push reads
  its push remote via `git for-each-ref … %(push:remotename)` (empty means
  origin), and the repo identity comes from `git remote get-url --push
  origin`.
- **No exemption after a config change in the same command.** belt reads
  the config before the command runs. So a step anywhere in the command
  that can move the push drops the exemption. Such a step is a `git config`
  write (`--global`, `-f`, `--add`, `--replace-all` and `--unset` included),
  `git remote add`, `set-url`, `rename` or `remove`, or `git branch -u`. So
  is a redirect into a `.git/` path, a `.gitconfig`, the XDG `git/config` or
  a `GIT_CONFIG*` file, and any other command that names one in its words
  (`cp`, `mv`, `tee`, `sed -i`, a `python3 -c` script) or takes the output
  of one through a pipe (`grep -l url .git/config | xargs sed -i ...`). A
  command that only
  reads or prints counts only through a redirect: `echo`, `printf`, `cat`,
  `head`, `tail`, `less`, `more`, `grep`, `rg` (not with `--pre`), `ls`,
  `stat`, `wc`, `diff` and `file`. So `cat .git/config; git push origin
  main` keeps the exemption, before the push or after it. Reads such as
  `git config --get`, `--get-all` or `--list`, `git remote -v`, `git remote
  show` and `git remote get-url` keep it too. A comment and a heredoc body no
  shell runs change nothing, so a commit message that names `.git/config`
  keeps it.
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
  strings (a shell's `-c` in a bundle too, as in `bash -lc`), heredocs,
  `bash < x.sh` stdin redirects, `cat x.sh | bash` pipes, and a heredoc or
  here-string passed through `cat`, `head`, `tail` or `tee` into an
  interpreter (`cat <<EOF | sh`). Wrappers (`sudo`, `doas`, `env`,
  `command`, `builtin`, `nohup`, `setsid`, `nice`, `ionice`, `stdbuf`,
  `caffeinate`, `chronic`, `arch`, `flock`, `timeout`, `gtimeout`, …) are
  read past first with their options, and so are the runners `watch`,
  `parallel` and `script`, and the reserved words that open a command
  (`if`, `then`, `do`, `!`, `{`). A command word that names an interpreter
  through a variable reads as that interpreter: `$SHELL`, `$BASH`,
  `$ZSH_NAME`, any name that ends in `SHELL`, and a variable the command
  sets to an interpreter (`S=sh`, `B=/bin/bash`).
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
- **A message or print that only names a command is data, not code.** Before
  the per-line scan, the guard blanks the text a carrier command carries
  without running it (`scriptmask.go`): the words of `echo` and `printf`
  (not `-v`), a `git commit` message (the value of `-m` or `--message`, which
  may be a `$(cat <<'EOF')` body, and the heredoc only with `-F -`), and a
  `gh pr`/`issue`/`release`/`gist` title, body or notes value with its
  `-F -` heredoc. Every other word of the command stays as written. So a
  denied command named only in a commit message or an echo string passes, a
  quoted `;` or `| xargs` in the string and an earlier `bash -c` in the same
  command included. The mask is off when a top-level pipe, a group word
  (`{`, `if`, `for`, …), `exec` or a `>( )` target could carry the output to
  something that runs it. It is off when something else in the command could
  run beside it or after the command ends, since such a job could read a
  file the echo or the commit writes. That covers a background `&` anywhere
  (inside a `-c` string too), a `coproc`, a `trap`, and words such as
  `nohup`, `setsid`, `at`, `open`, `Popen` or `fork`. It is off when the
  reader cannot follow the whole command. It is off for a heredoc body with
  a line that starts with its delimiter without being it: inside `$( )` bash
  ends the body at a line `EOF)` and runs the rest of that line, zsh does
  not. A `$(cat <<'EOF')` body is masked only when bash 3.2, macOS's
  `/bin/bash`, runs no more of it than `cat` (`bash32Closes`, the rule the
  git guards use). bash 3.2 knows no heredoc inside `$( )` and finds its
  `)` twice, each time at the first `)` it cannot match, so a `)` in the
  body can end it early. Its parse step ends the word. belt reads it as bash
  3.2 does: quoted strings and backtick pairs, `$( )`, `${ }` and `$[ ]`
  inside double quotes, backslash escapes, and a `#` comment only outside
  double quotes. Its expansion ends the command it runs. It takes a `#`
  after a blank for a comment inside double quotes too, so a `'` or `)` on a
  `#` line counts in one scan and not in the other. The body passes when
  both scans end where belt does and the text stays as written: no
  backslash-newline, `$'...'` or `$"..."`. Inside double quotes it also
  passes when the earlier of the two closes is past the first line, as in a
  numbered list (`1) ...`). The rest from there must hold no `"`, backtick,
  `$` or backslash, since only then do both scans read it as text of the
  same word. It passes as well when the parse step, read on through the
  rest of the command, never closes the `$( )`, as a lone apostrophe does
  when nothing after it closes the quote (`bash32Unclosed`). bash 3.2 then
  stops at a syntax error and runs nothing of the command. This holds only
  for the command as typed, not for a nested text or a `-c` script, and only
  when the text in front of the `$( )` holds no `$`, backtick, backslash,
  `<( )` or `>( )`, since an earlier `$( )` bash 3.2 closes later than belt
  does can take this one in. Any other body, one whose lone apostrophe a
  later word closes included, is scanned, so such a body that names a denied
  command denies. The reason then points at the forms that stay masked: `git commit -F -` (or `gh ... --body-file -`) with a quoted
  heredoc, or a message file. A backtick body is scanned when it holds a
  backslash, which every shell takes off before it reads the body. What can
  still run is scanned
  raw: every `$(…)`, backtick or `<( )` body wherever it stands (so
  `echo "$(rm -rf x)"` and `x=$(rm -rf x)` deny), a backtick body read with
  its escapes taken off as the push guard reads it, and an
  `echo`/`printf` piped through `cat`/`tee` into a stdin-reading interpreter
  (`echo '…' | sh`). The write-then-run and chmod scans keep reading the raw
  command, so a file written then run is unaffected.
- **`curl | bash` is denied outright** (`wget` too): nothing can read what
  would run. Download to a file in one call, then run the file in a later
  call.
- **Any code belt cannot read is denied the same way** (`scriptdeny_unread.go`).
  A download or decoded text that a shell, `eval` or `source` runs denies
  in these shapes:
  - `bash <(curl …)`, `source <(curl …)`, `bash < <(curl …)` and a
    here-string.
  - `sh -c "$(curl …)"`, `eval "$(curl …)"`, and a download as the command
    word.
  - A script named `/dev/stdin` or `/dev/fd/N` whose input is a download:
    `sh /dev/stdin <<< "$(curl …)"`, `bash /dev/fd/3 3< <(curl …)`.
  - A variable set to a download, then run: `X=$(curl …); eval "$X"`,
    `bash -c "$X"`, `echo "$X" | sh`, or `X=$(curl …) bash -c "$X"`. A
    variable copied from one counts too (`Y="$X"`).
  - A fetch anywhere in a pipeline into a shell: `curl … | tee x | sh`,
    `curl … | sh -s -- -y`, `| source /dev/stdin`. The fetch may sit in a
    group (`{ curl …; } | sh`, `if …; then curl …; fi | sh`), and the shell
    in a group, a subshell or a loop (`curl … | (sh)`, `curl … | { cd x;
    sh; }`). An `xargs` that runs its items as code counts as a shell:
    `curl … | xargs -0 sh -c`, `xargs -I{} sh -c '{}'`.
  - A shell named by a variable: `"$SHELL" -c "$(curl …)"`,
    `X=$(curl …); "$SHELL" -c "$X"`, `curl … | "$SHELL"` and `S=sh; curl …
    | $S`. `exec "$SHELL" -l` and `"$SHELL" -lc 'make test'` pass.
  - A `>( )` whose command runs its input as code, for what the command
    writes into it: `curl … | tee >(sh)`, `curl … > >(sh)`, `curl -o >(sh)
    …`. An `xargs` that runs its items as code reads them from its `-a`
    file the same way: `xargs -a <(curl …) sh -c`. Its options are read
    as getopt reads them, so a bundle counts too:
    `xargs -0a <(curl …) sh -c`, `xargs -0I{} sh -c '{}'`.
  - A file the same command writes a download to, then runs
    (`scriptdeny_saved.go`): `curl … > i.sh && bash i.sh`, `curl -o i.sh …`,
    `curl -O`, `wget -O i.sh`, plain `wget URL` by the name it saves, a
    `tee` after a fetch, then `sh i.sh`, `source i.sh`, `./i.sh`,
    `bash < i.sh` or `cat i.sh | sh`. A file written in one call and run in
    a later one is read from disk then.
  - Decoded text into a shell: `base64 -d`, `openssl … -d`, `xxd -r`,
    `uudecode` and the decompressors (`gunzip`, `zcat`, `gzip -d`, …), in
    every shape above.
  - `node`, `deno run`, `deno eval` and `bun` read as interpreters do:
    `deno run <(curl …)` and `bun <(curl …)` deny.

  The same shapes deny behind a wrapper, after a reserved word, in a brace
  group or a function body (`{ bash <(curl …); }`, `if …; then …; fi`,
  `f() { eval "$(curl …)"; }`), and inside a `-c` string, an `eval`, a
  subshell, a substitution and a runner's shell text. A `<( )` or `$( )`
  that only prints files with `cat`, `head` or `tail` is read instead: its
  files are scanned, so `bash <(cat x.sh)` passes when `x.sh` runs nothing
  denied. `bash <(echo …)` passes too, since the echo text is scanned as a
  substitution. A variable set to a readable substitution passes
  (`X=$(git rev-parse HEAD); eval "echo $X"`), and so does a download saved
  and only read (`curl -o f.json … && jq . f.json`). Not read yet:
  - A variable a builtin sets (`read X < <(curl …)`), a held variable in a
    heredoc body, and a variable that holds a `cat` of a file.
  - A download moved, copied or unpacked before it runs (`mv`, `cp`,
    `tar`), and a runtime that fetches a URL itself (`deno run https://…`).
  - A shell's name only the shell can work out: `"${SHELL:-sh}"`, or a
    `$( )` as the command word (`$(which sh)`).
  - A wrapper that starts a shell with no command, which then reads the
    pipe: `curl … | sudo -s`, `doas -s`, `su`.
  - A shell fed by a pipe of its own inside `( )` or `>( )`:
    `curl … | ( cat | sh )`, `curl … | tee >(cat | sh)`.
- **A command behind a wrapper is read past it** (`scriptdeny_indirect.go`).
  The permission system matches a command by the words it starts with, so
  `nice kubectl delete` and `script -q /dev/null rm -rf x` slip past it. A
  wrapper that runs its command as written (`sudo`, `nohup`, `nice`,
  `timeout`, `flock FILE`, BSD `script FILE CMD`, `coproc`, zsh's `noglob`
  and `nocorrect`, …) is denied when that command starts with a deny
  entry. So `coproc rm -rf /x` and `coproc NAME { rm -rf /x; }` deny. A
  runner that hands its words to a shell (`watch`, `parallel`, `script -c`,
  `env -S`, `sudo -s`, `flock FILE -c`) gets its text scanned to the end of
  the command, as a `-c` string is. A wrapper's options are read as the
  wrapper reads them (`sudo -u root`, `env -i`, `command -p`, `arch -arm64`
  and `arch -arch NAME`). `wrapperOptions` is the one table of wrappers,
  and every reader that skips one looks it up there. An option belt does
  not know fails closed: the words after the wrapper are scanned as shell
  text. A wrapper
  named by a path (`./nice`, `/usr/bin/env`) is read both ways, as the
  script of that name it may be and as the wrapper. The reserved words that
  open a command (`if`, `then`, `do`, `!`, `{`) and a function definition's
  head are read past too, without counting as a wrapper, and after a
  wrapper as well (`time { bash <(curl …); }`).
- **A command a name hides is read through the name.**
  - An alias or a function the same text defines runs as its value or body,
    with the call's arguments: `alias k=kubectl; k delete pod x` and
    `f() { kubectl "$@"; }; f delete pod x` deny.
  - A variable the text assigns a literal runs as that literal:
    `X="rm -rf"; $X /x` denies. A value built from literals and variables
    is resolved through the literals, so `Y=rm; Z="$Y -rf /x"; eval "$Z"`
    denies. A variable in it the text does not set is read by its name, as
    below.
  - A variable the text does not set comes from the user's profile, since
    shell state does not carry between Bash calls, so its name is all belt
    has. It denies when the name holds a deny entry's command word, case
    aside, and its arguments match the entry's words after that word:
    `$RM -rf /x` and `"$KUBECTL" -n prod delete pod` deny. A one-word entry
    and a `re:` entry name no such words. So `$X -rf /x`, `$GIT rm file`,
    `$NPM rm pkg`, `$GO test ./...` and `"$EDITOR" notes.md` pass.
- **Patterns come from** the `permissions.deny` `Bash(...)` entries of
  `~/.claude/settings.json` and `settings.local.json`, read live on every
  invocation so the guard and the permission system cannot drift.
  `claude_settings.enabled: false` in the belt config turns that read off,
  leaving only the extra patterns. Patterns also come from
  `guards.script-deny-list.extra_patterns` for things the settings file only
  lists under "ask" (like `rm -rf`). An extra pattern starting `re:` is
  compiled as a case-insensitive regex. It is the escape hatch for argument
  wildcards, and for flag reordering in a tool the argv match below does not
  read.
- **Matching is** per non-comment line, case-insensitive, whole-word, and
  whitespace-normalized: `kubectl delete` matches `  kubectl   delete pod x`
  but not `kubectl deleted`, in shell lines and Python
  `subprocess`/`os.system` strings alike. Backslash-continued lines are
  joined before matching, and a second pass collapses quotes, commas, and
  brackets so `kubectl "delete"` and list-form `subprocess.run(["rm",
  "-rf", …])` match the same patterns as their plain forms.
- **An rm, kubectl, gcloud or terraform entry also matches on argv**, the
  way the tool reads its arguments (`scriptdeny_argv.go`). Each command on
  the line is read from any of its words, so `sudo rm` and `xargs -0 rm`
  count.
  - `rm -rf` means rm with recursive and force, in any spelling: `-r -f`,
    `-fr`, `-Rfv`, `--recursive --force` and GNU prefixes such as `--rec`
    and `--for`. An option after an operand counts, since GNU rm reads it.
    `--` ends the options. `-d` (`--dir`) is not recursive.
  - For kubectl, gcloud and terraform, the flags before and between the
    verbs are skipped, so `kubectl -n prod delete`, `gcloud -q sql
    instances delete` and `terraform -chdir=x destroy` match. Any flag may
    take the next word as its value, so an unknown global flag still counts
    as a match. A gcloud release track (`alpha`, `beta`, `preview`) may be
    skipped too, so `gcloud beta sql instances delete` matches.
  - `terraform apply -destroy` counts as `terraform destroy`. `terraform
    plan -destroy` does not, since it only plans.
  - A word the shell expands (`$X`, `"$(…)"`) counts as a possible flag
    before a verb. It is never read as an rm option, since that would deny
    every `rm -f "$file"`.
  - An entry with a flag in it for a verb tool (`kubectl delete --all`),
    and an entry for any other tool, matches as text only.
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
  `reset --hard` or `--merge`, a path checkout (`checkout -- <path>`, `checkout <ref> --
  <path>`, `checkout .`, a patch checkout) or a forced one, `switch
  --discard-changes`/`-f`, `restore` unless the only target is the index
  (`--staged` without `--worktree`), and `clean` with `-f` or `-i` (a `-n`
  dry run passes). A clean with neither counts when `clean.requireForce`
  may be off. Six commands that write the tree count too: `worktree
  remove`, `apply`, `read-tree -u` or `--index-output`, `checkout-index
  -f`, `rm -f` and `mv -f`. So does a forced `submodule deinit` or
  `update`. `stash list|show|apply|pop|branch` are allowed. It also
  fires on `stash drop`, `stash clear` and a reflog or ref delete of
  `refs/stash`, which delete stash entries. It fires on a reflog expire,
  gc, prune or repack that deletes sooner than git's default, a reflog
  delete or drop, a call or config write with such a gc expiry, and a
  fetch, pull or `remote update` that may force-update or prune a local
  branch. Each invocation is
  judged in the directory it runs in: a tracked `cd`/`pushd`, a `git -C`
  target, else the session cwd, with `$PWD` and `$(git rev-parse
  --show-toplevel)` read as git-push-main reads them. Heredoc bodies are
  read as git-push-main reads them, so a commit or tag message may name
  `reset --hard` in plain words, and a PR body may in a code span. So are
  a wrapped command, an `eval` or a `sh -c` in a group, a loop, an `if` or
  a function's body, and an `xargs` that runs its items as code.
- **Reads reset, clean, checkout and switch options as git does.** Each
  option comes from a table of the options `git <cmd> -h` lists. Any prefix
  that names one option alone counts as its full name. So `reset --ha` is
  `--hard`, `clean --fo` is `--force`, `clean --d` is `--dry-run` and
  `checkout --forc` is `--force`. `switch --fo` names two options, so it
  counts. Short options may come in a bundle (`-xdf`). In a bundle, a
  letter that takes a value takes the rest of the word, so `checkout -fb
  new` forces and `-bf` creates a branch named `f`. An option that needs a value takes
  the next word, even `--` or one that starts with `-`. In `clean -e -n -f`
  and `clean -fen`, the `n` is a pattern, so the clean deletes. The last
  `-n` or `--no-dry-run` decides whether a clean is a dry run. `clean -i`
  counts, since it deletes what the user picks without `-f`. Anything belt
  cannot read this way counts as a discard. That is an option not in the
  table, a prefix of two options, or a value on an option that takes none.
  It is also a word the shell may still turn into an option (`$X`,
  `$(...)`, a leading glob or brace), or a command in front of git that may
  hand it more words (`xargs git reset`, `xargs git switch`).
- **Counts a clean without `-f` when `clean.requireForce` may be off.**
  git then deletes without `-f` or `-i`. The command may set config for
  the clean itself: `-c`, `--config-env`, a `GIT_CONFIG*` or `HOME`
  variable, or a `git config` write earlier in the command. Any of these
  counts, whatever the key. Otherwise belt reads `git -c
  core.fsmonitor=false config --type=bool --get clean.requireForce` in the
  repo, so a repo or global `false` counts too. A value git cannot read
  counts as `false`.
- **Reads `reset --merge` as git 2.55 runs it.** It resets the index and
  keeps a file whose worktree copy differs from the index. So a staged edit
  with no unstaged change on top is lost, and an unstaged edit is kept. git
  refuses the whole reset when a file with an unstaged change also differs
  between the index and the target. The proof checks what `--hard` drops,
  less what `--merge` keeps, and passes a reset git refuses.
- **Reads a checkout without `--` as git does.** belt asks git whether the
  first operand names a branch, tag or commit (`git rev-parse --verify
  <operand>^{tree}`). All the lookups of one call share a 2 second budget,
  so many checkouts in one command cannot run the hook past its timeout.
  If the operand is a ref and stands alone, the checkout is a switch. If
  it is a ref and more operands follow, they are paths restored from it,
  as in `git checkout HEAD f`. If it is not, every operand is a path
  restored from the index, as in `git checkout f`. An
  operand that is both a branch and a file reads as the branch: git switches
  to it when it stands alone and refuses it as ambiguous otherwise. An
  operand belt cannot look up reads as a path: a word the shell may rewrite,
  a directory belt cannot name, a lookup past the budget. Alone that
  reading drops the most, since a switch drops nothing. In patch mode or
  with more operands git may restore from it instead, so the proof cannot
  vouch and only a clean tree passes. A branch only a remote has also
  reads as a path; git creates it from `origin/<name>`. On its own that
  checkout still passes, since the proof finds no file of that name to
  drop. A checkout whose paths belt cannot see is a path checkout too:
  `--pathspec-from-file`, or one a command in front of git may hand more
  words, such as `xargs git checkout`.
- **Reads a patch checkout as taking every hunk.** `-p` (`--patch`, or in a
  bundle like `-qp`) with no path walks the whole tree, so belt reads
  `git checkout -p` as `git checkout -- .` and `git checkout -p HEAD` as
  `git checkout HEAD -- .`. Taking every hunk is the most it drops. From a
  tree it also deletes a file only the index has, so the proof checks
  those files too.
- **Reads six tree writers the same way.** Each takes its options from a
  table of what `git <cmd> -h` lists, read as reset and clean are. The last
  of an option and its `--no-` form decides.
  - `git worktree remove --force` (`-f`, twice for a locked worktree)
    deletes a linked worktree, untracked files included. A plain remove
    refuses a dirty tree, so only the commit check below applies to it.
    belt judges the worktree it removes, not the one it runs in. git finds
    that worktree by its path or by the end of its path (`remove wt`), so
    belt checks every worktree either reading could name. The name gets the
    shell's `~` and quote expansion. git starts a relative name from the
    real directory, so from a symlinked cwd `..` goes to the link target's
    parent. belt checks the name from the cwd as written and with its
    symlinks resolved. A remove deletes ignored files too, forced or not,
    and belt does not count them, as git's own check does not.
  - Any `git worktree remove`, forced or not, denies when the worktree's
    HEAD and HEAD reflog are the last refs that hold some commits. That
    holds on a detached HEAD and on a branch alike. A commit made on a
    detached HEAD and left behind by a checkout lives only in the reflog.
    git removes the worktree without a warning, and its HEAD and HEAD
    reflog go with it. The main worktree is not counted, since git refuses
    to remove it.
  - A `git worktree prune` denies the same way. It deletes what is left of
    each worktree whose folder is gone, HEAD and HEAD reflog included.
    belt asks git which ones with a dry run, `git worktree prune -n -v`,
    and passes on the prune's own `--expire`. An expire value the shell may
    still glob or unquote drops it, so the dry run names the most a prune
    deletes. One the shell may still expand, such as `$WHEN`, denies.
    `-n` and `--dry-run` pass.
  - belt counts the commits those HEADs and reflogs reach that nothing
    left reaches. What is left is every branch, tag, remote-tracking
    branch, stash entry and note, every reflog of them, and the HEAD and
    HEAD reflog of every other worktree. The count is one `git rev-list`
    on the repository's common dir: `--exclude` drops the removed HEAD
    from `--all`, and `--single-worktree --reflog` reads the shared
    reflogs. The other worktrees' HEAD reflogs come from `git rev-list
    --walk-reflogs`, since `--exclude` does not reach `--reflog`. A count
    git cannot give denies. The reason names the count and the newest lost
    commit, and says to run `git branch <name> <commit>` first.
  - A worktree on an unborn branch, as `git worktree add --orphan` leaves
    it, holds no commit, so its remove or prune passes. That holds while
    its HEAD keeps no reflog. A reflog there may hold commits git cannot
    walk from that HEAD, so the command denies.
  - A false deny belt accepts: after `git rebase -i --autosquash` with a
    fixup in a branch worktree, `git worktree remove` denies. The pick
    from before the fixup lives only in that worktree's HEAD reflog.
    `git branch keep-<x> <commit>` gets past it, and so does running the
    remove by hand.
  - A known gap: two removes in one command pass when a commit sits only
    in both worktrees' HEAD reflogs, as in `git worktree remove ../w1 &&
    git worktree remove ../w2`. Each remove counts the other worktree as
    kept.
  - `git apply` rewrites the files its patches touch. `-R` reverts edits,
    and a forward hunk replaces the lines it matches, so both count.
    `--cached` only changes the index, like `git reset`, so it passes. So
    do `--check` and a `--stat`, `--numstat` or `--summary` without
    `--apply`.
  - `git read-tree -u` with `--reset` overwrites every changed file, and
    with `-m` a staged one. `--index-output` writes the index over the
    file it names, with or without `-u`. Without either only the index
    changes. `-n` writes nothing.
  - `git checkout-index -f` overwrites files from the index. Without `-f`
    it leaves a file that exists alone.
  - `git rm -f` deletes a file with changes. A plain `git rm` refuses one,
    and `--cached` keeps the file.
  - `git mv -f` overwrites what stands at the destination, tracked,
    untracked or ignored. A plain `git mv` refuses an existing one.
- **Reads a forced `git submodule deinit` or `update` as its script
  does.** `deinit -f` deletes each submodule work tree it names, and
  `update -f` checks out over it. belt reads the words as git-submodule.sh
  reads them. An option counts only when written in full, so a bundle or a
  prefix makes git stop with its usage. `--reference`, `--depth`, `-j`,
  `--filter` and `--ref-format` take the next word. The first other word
  starts the pathspecs, so `deinit a -f` names a path `-f` and does not
  force. `git ls-files` matches the pathspecs with their magic, as the
  submodule helper does. A word the shell may rewrite, a command in front
  of git, `--all`, no pathspec and a global option touch every submodule.
  So does `git submodule--helper deinit` or `update`.
  - The call passes when each submodule it touches is clean. That is a
    missing folder, an empty one, or a checkout with an empty `git status
    --porcelain --ignore-submodules=none --untracked-files=normal`. A
    folder with files but no `.git` denies, since deinit deletes the files
    with it. A dirty file in the superproject does not count, and neither
    does a submodule on another commit than the one recorded.
  - The same rule holds for each submodule nested in a checkout, to any
    depth. git status in a checkout does not list the files in a nested
    folder that is not checked out, and deinit deletes them all the same.
  - A checkout whose git directory sets `core.worktree` to another folder
    denies. Its git status reads that folder, while deinit deletes this
    one. belt compares `git rev-parse --show-toplevel` with the path.
  - Every git call runs with `core.fsmonitor` off, so a repository config
    starts no program inside the hook.
  - All the git calls of one check share a 2 second budget.
  - A plain deinit or update passes, since git refuses to drop changes
    without `-f`.
  - A known gap: a deinit, forced or not, also deletes ignored files, and
    belt does not count them. Neither does it read the command a `git
    submodule foreach` runs.
- **Reads `core.worktree` from the command too.** A `-c core.worktree=<dir>`,
  a `--config-env`, a `GIT_CONFIG_*` variable or an earlier `git config
  core.worktree` write moves the work tree git writes. belt reads git
  status before the command runs, in the old tree. So such a discard
  denies, as one in a directory belt cannot name does. So does an
  `include.path` or `includeIf.<cond>.path` setting, which may set it, and
  a key the shell may still rewrite.
  - Dropping the key moves the tree just as much. So an earlier step, or
    one beside the discard, that may change the key counts the same. That
    is a `git config` unset of `core.worktree`, a removal or rename of the
    `core` section, an edit, or a word the shell may rewrite in a `git
    config` call that is no plain read. A write into a git config file by
    other means counts too: a redirect, `sed -i` or `tee`.
  - A `core.worktree` the command leaves alone needs nothing: git status
    where the command runs reads the moved tree.
- **Decides by** the working tree there. A clean tree (`git status
  --porcelain` empty, with `--ignored` added for `clean -x`/`-X`/`-e`, a
  clean belt cannot read and `mv -f`) allows,
  since nothing would be lost. belt passes `--untracked-files=normal`, so a
  repo's `status.showUntrackedFiles=no` cannot hide an untracked file. A
  worktree remove reads the status of the worktree it removes. A command that also writes outside the tree needs
  the proof: `checkout-index --prefix`, `apply --unsafe-paths` or
  `--build-fake-ancestor`, and `read-tree --index-output`. `allow_repos`
  exempts repos by canonical identity.
- **Denies a stash drop or clear on any tree.** The loss is the stash
  entry, not the tree. Every worktree of a repo shares one stash list, so
  an entry may be another session's set-aside work. A dropped entry comes
  back only through `git fsck`, until the next gc. git takes the stash
  subcommand from the first word alone, so `git stash drop -q`, `git -C dir
  stash drop` and an alias the command sets (`-c alias.x='stash clear'`)
  count. `git stash -q drop` is a push, which git refuses. A first word the
  shell may still rewrite (`$X`, `$(...)`, a glob or brace) counts, and so
  does a bare `xargs git stash`, which may get the subcommand from its
  input. A comment or a heredoc body no shell runs does not count. `pop`
  and `branch` pass, since the work comes back into the tree. The reason
  points to `git stash list` to see the entries, and to a WIP commit to set
  work aside.
- **Denies a reflog or ref delete of `refs/stash` the same way.** The
  entries live in its reflog. That covers `reflog delete stash@{0}`, a
  `reflog expire` or `reflog drop` of `refs/stash` or `--all`, `update-ref
  -d refs/stash` and `update-ref --stdin`, whose commands belt never sees.
  A `-n` dry run passes. A ref the shell may still rewrite counts.
- **Denies the forms that make a lost commit final, on any tree.** A
  commit off its branch after `branch -D`, `branch -f` or a reset stays in
  a reflog, or as an object `git fsck` finds, until a gc prunes it. git
  keeps an unreachable object for 2 weeks by default, a reflog entry for
  90 days, and an entry no branch holds for 30. These delete that last
  copy sooner:
  - A `reflog expire` with `--expire` sooner than 90 days ago or
    `--expire-unreachable` sooner than 30 days ago.
  - A `reflog delete` of any entry, unless it is a `-n` dry run, and a
    `reflog drop` of any log.
  - A `gc --prune=<date>` sooner than 2 weeks ago.
  - A `git prune` without an `--expire` date at least 2 weeks ago. Plain
    `git prune` counts: without `--expire` it deletes every unreachable
    loose object, however new. `--no-expire` reads as never.
  - A `repack -a -d` (or `-ad`), which packs everything and deletes the
    old packs with the unreachable objects in them. `-k` keeps those
    objects in the new pack, `-A` turns them loose and `--cruft` packs
    them apart, so those pass unless `--unpack-unreachable` or
    `--cruft-expiration` is sooner than 2 weeks ago. An
    `--unpack-unreachable` date turns the newer objects loose under `-a`
    too, so `repack -a -d --unpack-unreachable=3.weeks.ago` passes. An
    empty `--unpack-unreachable=` counts, since git reads it as now.
  - A `git config` write that sets `gc.pruneExpire`, `gc.reflogExpire` or
    `gc.reflogExpireUnreachable` (also `gc.<pattern>.reflogExpire`) sooner
    than its default, which every later gc applies.
  - Any git call that runs with such a setting: its own `-c` or
    `--config-env`, or `GIT_CONFIG_KEY_<n>` and `GIT_CONFIG_VALUE_<n>` or
    `GIT_CONFIG_PARAMETERS` set in front of it or earlier in the command.
    A commit, merge, rebase or fetch may start a gc, and a hook or alias
    may run one, so `git -c gc.pruneExpire=now commit` counts. A read
    (`status`, `log`, `diff`, `reflog show` and the like) and a dry run
    pass.

  `now` and `all` count, and `never` and `false` pass. belt has git read
  any other date (`git config --type=expiry-date`, the parse prune, gc and
  reflog use), so `1.second.ago`, `2099-01-01` and a future date count. A
  date git cannot read counts too. Options are read as for reset, so
  `gc --pru=now`, `prune --exp=all` and `reflog expire --expire-u=now`
  count. An option belt cannot read, a word the shell may still rewrite
  and `xargs` in front of git count too. A `-n` dry run, `gc --prune` with
  no date and an expiry at least as old as git's default pass. This is a
  rule with no proof. The reason says belt cannot see the user's message:
  keep what is needed on a branch first, or hand the user `! <command>`.
  `branch -D`, `branch -f`, `branch -M`, `update-ref -d`, `checkout -B` and
  `switch -C` pass, since what they move off stays in a reflog or as an
  unreachable object until then.
- **Denies a forced fetch or pull into a branch that holds unpushed
  work.** That is a refspec with a leading `+` (`git fetch origin
  +main:feat`, `git pull origin +main:feat`), or `-f` with `src:dst`,
  whose destination git reads as a branch: `refs/heads/x`, `heads/x` or a
  bare name. A `--refmap` value counts as a refspec, and so does each
  `remote.<name>.fetch` value: one the call sets with `-c` or a
  `GIT_CONFIG_*` variable, one an earlier step of the same command writes
  (`git config`, `--add`, `--replace-all`, `git config set`, or the
  `+refs/*:refs/*` a `git remote add --mirror=fetch` writes), and one the
  repository's config holds, which belt reads with one `git config
  --get-regexp`. A configured refspec counts with refspecs on the command
  line too, since git still maps the fetched refs through it. A `git
  remote update`, with or without remote or group names, is a fetch with
  no refspec words of its own, so the settings and the configured
  refspecs count. So is a `git remote add -f` or `--fetch`, which fetches
  the new remote through the refspecs it writes: `git remote add -f
  --mirror=fetch x <url>` force-moves every branch in one command. belt runs `git rev-list -n 1 --ignore-missing <branch>
  --not --remotes` in the repo, within the 2-second proof budget, and
  denies when it prints a commit. The reason names the branch and the
  commit, for `git branch <name> <commit>`. A branch with nothing unpushed
  passes, and so does one that does not exist. So do a destination under
  `refs/remotes/` or a tag, git's default `remote.origin.fetch`, a fetch
  with no destination (`git fetch origin main`) and a `--dry-run`. A
  pattern destination (`'+refs/heads/*:refs/heads/*'`) checks every branch
  it matches, and `refs/*` every branch. It checks a branch the remote does
  not have too, since telling would need `git ls-remote`, a network call.
  A destination belt cannot read checks every local branch: a word the
  shell may still rewrite, `--stdin`, an option belt does not know,
  `xargs` in front of git, or a config read that fails. git refuses to
  fetch into a branch a worktree has checked out, but a pull lets it, so
  belt checks those too. A bare mirror (`git clone --mirror`) passes: in
  a repository git reads as bare, the refspecs of a remote whose
  `remote.<name>.mirror` is true in the config files count for nothing,
  since its `+refs/*:refs/*` keeps every local ref a copy of the remote's.
  The same config read takes `core.bare`, and only when that is true does
  belt confirm with `git rev-parse --is-bare-repository --git-common-dir`;
  a call that fails or runs out of time counts as no bare repository. A
  fetch that does not prune passes in a bare mirror with linked worktrees
  too, since git refuses to update a branch a worktree has checked out. A
  mirror remote
  in a clone with a work tree (`git remote add --mirror`) counts, and so
  does one in a linked worktree of a bare mirror, which reads `core.bare`
  as true too: there the branches hold the user's work.
- **Denies a pruning fetch that deletes a branch holding unpushed work.**
  A fetch, pull or `remote update` that prunes deletes the local branches
  a pattern refspec's destination matches when the remote lacks them, with
  their reflogs, forced or not: `git fetch --prune origin
  'refs/heads/*:refs/heads/*'` deletes a local-only branch. It prunes with
  `--prune` or `-p`, or with neither when `fetch.prune` or
  `remote.<name>.prune` may be true: in the config files, from `-c`, a
  `GIT_CONFIG_*` variable or an earlier `git config` write. `--no-prune`
  turns it off. belt checks those branches the same way, within the same
  budget, and the reason says to push first. Plain `git fetch --prune` and
  `git remote update --prune` with git's default refspec pass with the one
  config read. An exact refspec passes, since git refuses the fetch when
  the remote lacks its source, and so does `--prune-tags`, which only adds
  `refs/tags/*:refs/tags/*`. A `git remote prune` fetches nothing and
  prunes through every refspec the same way, a `+` one included. It also
  prunes through an exact refspec whose destination is written as
  `refs/heads/<name>`, since git compares the remote's refs with the
  source as written: `main:refs/heads/x` deletes x even when the remote
  has main. belt checks that branch whether the remote has the source or
  not. `-n` and `--dry-run` pass, and with git's default refspec it
  passes with the one config read. A bare mirror with no linked worktrees
  passes here too. In a bare mirror with linked worktrees every fetch,
  pull, `remote update` and `remote prune` that prunes denies, with a
  reason that names the linked worktrees: the mirror's refspecs delete a
  branch made in a worktree when the remote lacks it (`- [deleted] (none)
  -> <branch>`), and belt does not check which branches hold work. belt
  finds the worktrees with the same `git rev-parse` call and a read of
  `<common dir>/worktrees`, where an entry whose folder holds a `gitdir`
  file counts; a read that fails counts as a worktree.
- **The fetch checks leave some things unread.** They miss a config
  write later in a loop body, which the next round of the loop runs
  before the fetch, and one in a step after the fetch that runs beside it
  (`&` or a pipeline). They do not read the remote a fetch names, so
  every remote's refspecs and prune keys count, nor a `-c` or written
  `remote.<name>.mirror` or `core.bare`. They skip a config file another
  variable names (`GIT_CONFIG_GLOBAL`, `HOME`) or another tool edits, and
  `submodule update`, which fetches too. They do not read `git remote
  <word the shell may rewrite> -f --mirror=fetch x <url>` as a mirror
  fetch; only that unsure subcommand word is missed, and an `xargs` in
  front of git or a `$M` in the option slot is read. Some calls deny where
  git loses nothing (false denies): a `git remote add -f --mirror` inside a
  bare mirror when a branch holds a commit, a `-c remote.<name>.fetch`
  refspec inside a bare mirror, and every fetch that prunes in a bare
  mirror with linked worktrees, since that check fails closed.
- **Treats a subcommand belt cannot see as any.** One case is a command in
  front of git that may hand it the subcommand (`echo stash clear | xargs
  git`). Another is a subcommand word only a shell can resolve (`git $X
  clear`). The last is an alias body belt cannot read (`git -c
  alias.x="$B" x`). Only a clean tree passes one. It denies on any tree
  when it may drop stash entries. That is always so behind `xargs`. After a
  shell word it is so when a `drop` or `clear` word follows, or no plain
  word does. For an alias it is so when the body may run `stash`, `reflog`
  or `update-ref`. A plain alias body led by another builtin (`log $Y`)
  keeps the entries, since git splits it without a shell.
- **Passes a dirty tree when nothing is lost.** A checkout or restore of
  paths, a `clean -f`, a `reset --hard [<commit>]`, a `reset --merge
  [<commit>]` and the six tree writers
  pass when every file version they drop is already committed at the same
  path. The commits that
  count are HEAD, its upstream (`@{u}`) and origin's default branch
  (`refs/remotes/origin/HEAD`, only while it points at a branch under
  `refs/remotes/origin/`), plus what the command itself writes there.
  That covers the leftovers of a half-finished pull: untracked files or
  edits that match upstream. The check covers the worktree file and, where
  the command replaces the index, the staged copy. Both are compared byte
  for byte as git blob ids; only content counts, so a mode change alone is
  not a loss. A `reset --hard` to another commit must also keep every
  commit HEAD reaches reachable from the target, the upstream or origin's
  default branch.
- **Lists the paths through git.** A restore checks the index paths it
  matches, or the source tree's paths when it restores from a tree. A
  clean checks `git ls-files --others` under the same ignore rules: `-x`
  adds ignored files, `-X` lists only them. A reset checks the paths `git
  status` reports changed and, for another commit, any untracked or ignored
  file at a path the target adds. A `read-tree -u` checks what a reset to
  its tree checks, and keeps the branch where it is. A `checkout-index -f`
  checks the index paths it names, or all under the directory for `-a`. An
  `rm -f` checks the index paths it matches, with their staged copies. An
  `mv -f` checks what stands at each destination; a directory destination
  takes each source by its base name. An `apply` checks the paths `git
  apply --numstat` lists for its patches, and their staged copies with
  `--index` or `--3way`. A `worktree remove --force` checks the changed and
  untracked files of that worktree. Every form also checks a file or symlink
  that stands where a path it writes needs a directory, since git replaces
  it with the directory.
- **Proves only a discard that runs on its own.** The proof reads the tree
  before the command runs, so another step in the same command that may
  change the tree first leaves the clean-tree rule alone. That is any step
  before the discard, and any step after it when the command holds a pipe
  or a lone `&` that may run it beside the discard. A read-only command
  (`cat`, `ls`, `grep` and the like) or a plain `git status`, `log`,
  `diff`, `show`, `rev-parse` or `ls-files`, with no redirection, does not
  count. Without a pipe or a lone `&`, a step after the discard, such as
  `&& git pull`, runs only once the discard is done and does not count
  either.
- **Still denies** a command whose losses it cannot list exactly. That is
  an option it does not read (`--ours`, `checkout -f`, `checkout -m`,
  `--pathspec-from-file`, `clean -i`, `apply --reject`, `rm --sparse`), a
  glob, pathspec magic or a word the shell may still rewrite. An `apply`
  whose patch comes from stdin is one (`git diff | git apply -R`), since
  belt cannot read it a second time, and so is a patch that renames or
  copies a file. A `clean -e` is one: a `!` pattern brings an
  ignored file in, and with `-X` a plain one adds untracked files. So is a
  global git option other than `-C`, a config override or a word in front
  of `git` (`GIT_ICASE_PATHSPECS=1`, `env`, `xargs`), since the proof runs
  git with literal pathspecs. So is an index entry marked assume-unchanged or
  skip-worktree, which `git status` skips; for a reset, one anywhere in
  the index counts. It is also a conflicted path, a submodule (a dirty one
  shows even under `ignore = dirty`, and a reset is refused whenever
  `submodule.recurse` is on and the index holds a submodule), a nested
  repository, more than 256 paths, or a proof that runs past its 2-second
  budget. `git stash`, `switch` and a subcommand belt cannot see keep the
  clean-tree rule alone. The reason names the first file
  that would lose content, or why the losses could not be listed. It also
  says that belt cannot see the user's message, so a go-ahead does not
  pass the check: save the work first, or hand the user `! <command>`.
- **Why it exists**: a session audit found agents stashing, hard-resetting,
  and checking out over the user's uncommitted work without being asked.
  Worktree changes are in no reflog, so this is damage that is hard to undo.
  The nothing-lost pass came later: the clean-tree rule alone refused the
  cleanup after a half-finished pull, files equal to upstream, even after
  the user asked for it.
- **Fails closed**: a directory only a shell can name, or a `git status`
  that fails there, denies. So does a discarding command in text the reader
  could not follow, where no directory is certain, and a nothing-lost proof
  that cannot finish. `mode: soft` turns a deny into a warn event.

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

### file-discard

Blocks a shell delete that may drop work no commit holds. It applies
git-discard's rule to the commands git does not run.

- **Fires on** `rm`, `unlink` and `shred`, wherever the shell runs them: a
  chain, a pipeline, a subshell, a `$(...)` substitution, a `bash -c`
  script, an `eval`, a heredoc a shell reads. The GNU names Homebrew
  installs (`grm`, `gunlink`, `gshred`) count too. Behind a wrapper the
  script-deny-list guard reads past (`sudo`, `env`, `timeout`, `nice`,
  `flock` and the rest of its list), or behind `busybox` or `toybox`, the
  first `rm`, `unlink` or `shred` word counts (`sudo -u x rm`). Text no
  shell runs is not one:
  an `echo` string, a comment, a commit message, a heredoc `cat` prints.
  `git rm` is git-discard's.
- **Also fires on** the commands that delete by their own syntax, behind
  the same wrappers:
  - `find R... -delete`, and `-exec`, `-execdir`, `-ok` or `-okdir` with
    `rm`, `unlink` or `shred`. Each root (default `.`) is a tree. Tests in
    front of the action made only of `-name`, `-iname` and `-type` (with
    `-maxdepth`, `-mindepth`, `-prune` and the like) narrow it to the paths
    they match. For `-delete` and `-exec rm` the name must match; for
    `-exec rm -r` a folder above may match too. find runs the part after an
    `-o` only where the part before it is false. So an action after an
    `-o` outside parentheses keeps the tests between the two:
    `find . -name node_modules -prune -o -name '*.pyc' -delete` takes only
    `*.pyc` files. Any other part (`!`, parentheses, `-path`, `-newer`)
    puts the whole root at risk. GNU and BSD options before the roots are
    read (`-H`, `-L`, `-P`, `-E`, `-x`, `-f path`).
  - `rsync ... --delete` (every `--delete` form and `--del`): the local
    destination is a tree. A remote one (`host:path`, `rsync://`), a dry
    run (`-n`) and `--backup` pass. With `--remove-source-files` each local
    source is dropped too.
  - `xargs rm` and `rm $(feeder)` or backticks: the paths the feeder
    prints. `git ls-files` and `git grep -l` hand the files git tracks under
    the shell's directory, so an untracked or ignored file there is not at
    risk. `grep -rl`, `rg -l`, `rg --files` and `find` hand their path
    operands (default `.`) as trees, `find` narrowed by its tests. `fd` and
    `fdfind` hand their roots as trees, read the way `fd -x` is read below,
    so an fd option belt does not read denies here too. Any other
    feeder (`cat list`, `echo`, `ls`) denies. So does a find feeder that
    runs a command (`-exec`, `-execdir`, `-ok`) or prints more than paths
    (`-printf`, `-ls`, `-fprint`). So does an fd feeder that runs a
    command (`-x`, `-X`) or prints more than paths: `--format`, `-l`, `-a`,
    and `--color` or `--hyperlink` set to `always`. An xargs whose input comes from a
    redirect (`< list`, `0<list`, a heredoc or a herestring) denies, and
    so does a feeder whose error output reaches the pipe (`2>&1`, `|&`).
  - xargs cuts what it reads at blanks, quotes and backslashes, and
    `xargs -d X` cuts at X. An unquoted `$(...)` cuts at blanks and then
    expands globs in each piece. A path the feeder prints that holds one
    of those bytes reaches the command as other paths. So it denies
    wherever it lies: a tracked file, an ignored one, a temp folder or a
    nested repository. A newline cuts every form but `-0`. git without
    `-z` prints an odd name (a quote, a backslash, a byte past ASCII) in C
    quotes, as `core.quotePath` says, and that denies too, as does
    `git -c core.quotepath`. NUL-separated paths (`find -print0`,
    `git ls-files -z` or `rg -0` into `xargs -0`) and a quoted `"$(...)"`
    reach the command whole.
  - GNU `parallel rm`: the paths it hands the command. With no input
    source it reads its input like xargs, one path per line, so the feeder
    rules above apply. The words after `:::` are paths. Each one lands at
    `{}`, at the `-I` text, or after the command's own operands. A `::::`
    or `-a` file denies, unless it is a `<(...)` that runs a feeder belt
    reads. `-0` and `--null` cut at NUL, and `-d X` cuts at X.
    `--dry-run` passes. An option belt does not read denies: `--pipe`,
    `--colsep`, `-L`, a `--trim` other than `n`, a profile (`-J`), and a
    long option cut short. So does `PARALLEL` set in the command. So does
    a replacement string other than `{}` (`{.}`, `{/}`), or a path built
    around one (`{}.bak`).
  - `fd` and `fdfind` with `-x`/`--exec` or `-X`/`--exec-batch`: the
    entries fd matches, read like find. Each root (the paths after the
    pattern and each `--search-path`, default `.`) is a tree. fd skips
    hidden entries, `.git` among them, unless `-H` or `-u`. It skips what
    git ignores unless a `--no-ignore` form or `-u`. One `-e`, the `-t`
    letters and a `--glob` pattern narrow the tree. A regex pattern does
    not. The command takes each entry at `{}`, or after its own words when
    it names none. `{/}`, `{//}`, `{.}` and `{/.}` are paths belt cannot
    name, and deny. The options are those `fd --help` lists for fd 10.5.1.
    Any other option denies, and so do `-L`, `--base-directory` and
    `--path-separator`.
  - The command xargs, parallel, fd and find `-exec` run is judged one
    way. `rm`, `unlink` and `shred` take the paths as above. A `sh -c` or
    `bash -c` script that hands its arguments to one (`rm "$1"`,
    `rm -f "$@"`) takes them too. Some commands run their input as code:
    `eval`, a `-c` with no script, and a `-c` script or a command word
    that holds the replacement text (`{}`, the `-I` text). Such a command
    denies when the text that builds the input
    names a delete: the pipe in front, the `:::` words, a here-string or a
    heredoc. A find or rsync it runs denies when its words hold `-delete`
    or an rsync delete option. A command that deletes nothing passes.
- **Decides by** each path the command drops, resolved without a shell. A
  literal path, `~`, `$HOME` and `${HOME}` resolve, and so does a variable
  the command sets to a literal. A relative path counts from the directory
  the shell is in, cd tracking included. `$PWD` is that directory too, in
  the command's own text. The cd before it must surely have run, and
  nothing in the command may set `PWD`. A path is judged under the name
  its folder lists, so `B.txt` on a volume that ignores case is judged as
  `b.txt`, the name git knows. A glob is expanded the way bash and zsh
  expand it by default: `*` and `?` skip names that start with a dot, `**`
  matches any run of folders (hidden ones and symlinks aside), and a
  trailing `/` matches folders only. Its folder takes rule 1, then each
  match is judged on its own. The first rule that fits decides:
  1. The root, the home directory or a folder that holds it: denied. Only
     the user may run it, with `! <command>`.
  2. Inside a temp folder, or inside a folder `mktemp` made earlier in the
     same command: passes. A glob of a temp folder's own entries passes
     whole. One the kernel may follow through a symlink, as with a
     trailing `/` or a glob before a deeper part, is judged match by match.
     A path at or under a cache folder passes too: `~/Library/Caches`,
     `~/.cache` and `~/Library/Developer/Xcode/DerivedData`. The path is
     judged after its symlinks resolve. So a link in a cache folder that
     leads into a work tree is judged where it leads. `~/Library` and
     `~/Library/Developer` are not cache folders.
  3. Missing, an empty file or folder, or a symlink `rm` removes as itself:
     passes. So does a folder `rm` without `-r` leaves alone.
  4. Outside every git work tree: denied.
  5. The top of a work tree, or `.git` and anything in it: denied. For a
     linked worktree the reason names `git worktree remove <path>`. A find
     or a feeder may start at the top when it hands nothing in `.git`:
     belt reads `.git` against the find tests, `git ls-files` lists none,
     `rg` skips hidden folders, and `grep` needs `--exclude-dir=.git`.
  6. Inside a work tree: one `git status` per tree over the dropped paths.
     No output passes. An untracked file denies. A changed file passes
     only when it equals the version in HEAD, its upstream or origin's
     default branch. git status does not read a file marked
     assume-unchanged or skip-worktree, so `git ls-files -v` finds them,
     and one that exists and differs from the index counts as changed. A
     file a sparse checkout leaves out is missing and passes. An ignored
     file passes, so build output does.
  7. A folder dropped whole must hold no work tree of the repository (`git
     worktree list`), which denies: Claude Code keeps its worktrees in the
     ignored `.claude/worktrees`. Every other repository in it, a nested
     one or one inside an ignored folder that git status does not look
     into, passes only on a short trust list. belt does not try to prove
     such a repository safe in detail. A clone, or a bare repository (a
     folder that holds `HEAD`, `objects/` and `refs/`), passes only when
     all three of these hold:
     - No linked worktree of it still has its folder. git keeps that
       worktree's index, HEAD and refs in the repository, so the drop
       breaks it, staged changes included. The reason names
       `git -C <repo> worktree remove <path>`. A worktree whose folder is
       gone is judged by the third rule.
     - Its `git status` lists nothing. Ignored files are fine. No file
       marked assume-unchanged or skip-worktree differs from the index,
       as in step 6. A rebase or
       merge stopped with `--autostash` keeps the user's change in a
       commit no ref lists. So `rebase-merge/autostash`,
       `rebase-apply/autostash` or `MERGE_AUTOSTASH` denies, in the
       repository or in a worktree whose folder is gone.
     - Every commit git keeps there is on a remote branch
       (`refs/remotes/<name>/`) of a remote git reaches across the network.
       Its URL has a scheme other than `file://`, or is `host:path`. Each
       URL and push URL of that remote must be one, as written and as each
       `url.<base>.insteadOf` or `pushInsteadOf` rule turns it. git keeps
       what each ref and each reflog holds: HEAD, the branches, the tags,
       the stash, `refs/bisect`, `refs/notes` and any other ref. It also
       keeps each linked worktree's HEAD and its own refs. belt asks git
       for that list, for both ref backends, and reads no ref file itself.
       The remote's `refs/prefetch`, which `git maintenance` fills, counts
       too. The logs of its remote branches are left out, since the remote
       held each commit they name, as after a fetch of a force-pushed
       branch.

     Nothing else counts as kept. The remote branches of a remote in a
     local folder do not count, since the same delete may take that
     folder. Nor does a remote branch such a remote may fill through its
     fetch refspec, nor a mirror's refs, nor a tag. One exception covers
     SwiftPM's package cache. SwiftPM's scratch folder holds its
     `workspace-state.json`: `.build`, `.build/index-build` for
     SourceKit-LSP's index build, or a `--scratch-path`. A bare mirror
     directly in its `repositories` folder passes whole when it has no
     linked worktree and each of its remotes is a mirror across the
     network. SwiftPM clones each checkout in `checkouts` from that mirror
     with `git clone --shared`, so the checkout's origin is the mirror's
     folder. Take a checkout whose origin is such a mirror in its own
     scratch folder. Its origin counts as a network remote, and the
     mirror's refs count as kept too, so a checkout on a release tag
     passes. The three rules still apply to the checkout. A drop that a filter
     narrows (find's tests, a feeder) takes only what the filter hands
     from a nested repository. That repository is judged as a work tree of
     its own, by its own `git status`. Part of a bare repository is git's
     own data and denies.

     git keeps each submodule's repository in `.git/modules`, and the drop
     takes those too. A submodule that `git submodule deinit` or `git rm`
     took out of the work tree lives only there. So each one is judged as
     a repository with no work tree: the first and third rules, an
     autostash, and an index that must hold what its HEAD holds, since a
     deinit keeps what was staged. A checked-out submodule is also judged
     where it lies, by its own `git status`.
- **Answers with** the first loss and the rule. When `toss-bin` is on PATH
  it adds `toss-bin --safe-mode -r` with the targets as written, which moves
  them to the trash. For find it offers
  `-exec toss-bin --safe-mode -r {} +`, for xargs
  `xargs toss-bin --safe-mode -r`, for fd `-X toss-bin --safe-mode -r`,
  and for rsync `--backup`. The reason
  ends like git-discard's: belt cannot see the user's message, so save the
  work first or hand the user `! <command>`. An rm whose targets are all
  temp gets rm-temp's answer instead, since rm-temp runs first.
- **Why it exists**: `rm -rf` is the one shell command that loses work
  outright, and git-discard covers only git. A wrong `~`, a glob from the
  wrong folder or a cd that failed can take a home directory or a dirty
  tree with it.
- **Fails closed.** A path belt cannot resolve denies and names the path: an
  unset variable, a `$(...)` that runs no feeder belt reads, braces,
  `~user`. So does a glob belt does not expand the way the shell does: `.*`,
  which can match `..`, zsh's `***`, a class such as `[[:alpha:]]`, a `..`
  after a glob part. A relative path or `$PWD` denies when the cd before it
  may not have run, as after `cd x;` or `cd x ||`. Name the path in full, or
  join the cd to the delete with `&&`. A pipe after `cd x &&` runs only
  once the cd has, so `cd x && find . -name '*.pyc' | xargs rm` reads from
  `x`. `cd x || exit` and `cd x || exit 1` leave the shell when the cd
  fails, so what comes after runs in `x`. `cd x || return 1` does not
  count, since bash and sh run on after a `return` outside a function.
  Text the reader cannot follow denies
  when it names `rm`, `unlink`, `shred`, `-delete` or an rsync delete
  option. So does code a shell runs that belt cannot read, when the text
  that builds it names one of those. Examples are
  `sed 's/^/rm /' list | sh`, `eval "$(sed ...)"`, `bash <(sed ...)`,
  `bash <<< "$(sed ...)"`, a `$(...)` in an unquoted heredoc a shell reads,
  and a `$(...)` as the command word. belt scans the words of the pipe in
  front of the shell, or of the substitution, a backtick one with one level
  of escapes taken off. Code built from text that
  names none passes:
  `eval "$(mise activate zsh)"`, `source <(fzf --zsh)`,
  `echo 'make test' | sh`. A feeder belt cannot read denies, and so do `find -L`, `grep -R`
  and `rg -L`, which follow symlinks out of their folders. One call reads at
  most 1024 folders for its globs and 256 matches. Past either, a glob whose
  matches stay in its folder is judged as that folder, one tree, and any
  other glob is denied. The search for repositories in a nested or ignored
  folder, and the read of `.git` against find's tests, stop at 20,000
  folders, and past that the call is denied. Each folder is searched once
  per call. In a `node_modules` folder the search reads each package and
  each `@scope` package, not their insides, so a large one stays far under
  the cap. A command that cuts names reads every folder. `git ls-files`,
  `git grep` and `rg` without `-u` or `--no-ignore` print nothing in an
  ignored folder, so no ignored folder is searched for them. The
  repositories a delete takes whole are checked four at a time. The git
  checks and the folder
  walks share one 2-second budget and one cap of 256 changed paths per call,
  and past either the call is denied. belt's own git runs with
  `core.fsmonitor` off and `GIT_NO_LAZY_FETCH=1`, so a partial clone
  fetches nothing, and not at all while a `GIT_DIR`-style variable is
  set. `mode: soft` turns a deny into a warn event. There is no repo list.
- **Accepted gaps.** Any delete outside work trees, temp folders and the
  three cache folders is denied. An ignored file counts as rebuildable,
  so an ignored `.env` passes. A find at the top of a work tree that holds
  a linked worktree in an ignored folder denies, whatever its tests. The
  trust list denies some repositories that lose nothing. A nested clone
  whose only remote is a local folder denies, and so does a SwiftPM
  package resolved from a local folder. So do a mirror outside SwiftPM's
  layout and a nested repository with a linked worktree whose folder is
  there. A commit only a tag holds denies, since belt does not ask the
  network. An annotated tag made in the repository and never pushed
  passes when a remote branch holds its commit, and its message is lost:
  a fetched tag and a local one look the same. SwiftPM's cache
  counts every ref as its remote's, so a commit pushed into that mirror
  passes. A remote branch counts as kept though the remote may have
  dropped it since. belt reads remotes from git config only. So a ref
  written by hand into a network remote's `refs/remotes` counts as kept
  too: `git update-ref`, or a fetch with its refspec on the command line.
  That covers a local commit fetched from the repository itself
  (`git fetch . feat:refs/remotes/origin/feat`), which then counts as
  pushed. A
  repository outside the drop that borrows objects from one inside it
  (`objects/info/alternates`, which `git clone --shared` writes) breaks,
  since belt does not search the disk for borrowers. `rm -rf
  .build/repositories` does that to SwiftPM's checkouts.
  A repository deeper inside a `node_modules` package, as in a nested
  `node_modules`, goes unseen. An empty
  repository denies, and so does a `git clone --bare`, which has no fetch
  refspec. A feeder between other filters
  (`git ls-files | grep x | xargs rm`) denies. Under a folder whose name
  git quotes (`café/`), `git ls-files` without `-z` denies, even where it
  prints the names below it plain. A command that sets `IFS` cuts names
  where belt does not look. The tree is judged before the command runs, so
  `mv work build/ && rm -rf build` passes. A glob is read with the default
  shell options: a user's `setopt globdots` or `shopt -s dotglob` makes `*`
  match `.git` where belt does not. A delete spelled in pieces
  (`awk '{print "r" "m " $0}' list | sh`) passes, since the scan is there
  for slips, not for code that hides. A commit that only the log of a
  remote branch holds passes, though the remote may have dropped it since.
  A `PARALLEL` variable or a
  `~/.parallel/config` from outside the command adds options belt does
  not see. A `-c` script that holds the replacement text denies even
  over a clean set (`xargs -I{} sh -c 'rm "{}"'`), since the path is
  read as code. Write `sh -c 'rm "$1"' _ {}` instead. GNU `sem`,
  `awk` with `system()`, and a script a command writes and then runs
  are not read.

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
- **The call fits the file.** `csl_ask` reads a file whole or skips it,
  and by default it reads 6000 lines in one call. For a longer file the
  call sets `max_lines` to the line count rounded up to a hundred. The
  limits come from `kit/cslask`, the package csl's `csl_ask` uses, so the
  two cannot drift apart. No call may ask for more than 20000 lines. For a
  file past that, the reason hands over no `csl_ask` call. It states the
  line count, says `csl_ask` cannot answer from the file, and says to read
  it in ranges with `offset` and `limit`.
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
  `.gitignore`. The NUL check reads the whole file and counts its lines
  the way `csl_ask` does, for `max_lines`. It runs after the line count and
  the index lookup, and before the transcript check.
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

### main-checkout

Keeps a repo's main checkout read-only and points the change at a linked
worktree. One id on two events: `write` and `bash`.

- **Fires on** an Edit or Write of a file inside a main checkout, and on a
  git command that changes one: `commit`, `add`, `rm`, `mv`, `reset`,
  `restore`, `merge`, `rebase`, `cherry-pick`, `revert`, `am`, `bisect`,
  `update-index`, `stash` (all but `list` and `show`), `apply` (all but
  `--check` and the report-only `--stat`, `--numstat`, `--summary`, read
  as git-discard reads them, so an option belt cannot read counts),
  `sparse-checkout` (all but `list` and `check-rules`), `submodule` (all
  but bare, `status` and `summary`), `clean` with `-f`, a `branch -m` or
  `-M` that renames the checked-out branch, a `checkout` or `switch` to
  anything but a default branch, and a `pull` that is not a sync. Each git
  invocation is judged in the directory it runs in. The walk git-push-main
  and git-discard share finds it: a tracked `cd`/`pushd`, a `git -C`
  target, a subshell, a substitution, an `sh -c` script. Heredoc bodies and
  quoted text are not commands.
- **A main checkout** is git's own test: in a linked worktree `git
  rev-parse --git-dir` and `--git-common-dir` differ. belt reads that from
  files, with the lookup the csl checks use (`cslindex.RepoRoot` and
  `cslindex.LinkedWorktree`), so no git runs. Every checkout that is not a
  linked worktree counts as main, a submodule included. A path is resolved
  through its symlinks first, so an edit of a dotfile linked from the home
  directory is judged in the checkout it lands in.
- **Lets through** read-only git and anything that does not change the
  checkout: `status`, `log`, `diff`, `fetch`, `branch`, `tag`, `push` (that
  is git-push-main's), and `worktree add`, the command the reason hands
  over. A `checkout` or `switch` to `main` or `master`, with at most `-q`,
  passes, since it puts the default branch back; when the checkout's
  `refs/remotes/origin/HEAD` file names one of the two, only that one. A
  `branch -m old new` passes when `old` is not the checked-out branch. A
  `pull` passes when the checkout is on `main` or `master`, the operands
  name at most the remote and that same branch, and no option merges any
  other way (`--no-ff`, `--squash`, `--no-commit`, `--edit`, a strategy,
  `-Xtheirs` and `-sours` attached included). The value of an option that
  takes one, such as `--depth 1`, is not an operand. A checkout that keeps
  the rule has no commits of its own, so such a pull fast-forwards. A
  branch git cannot read within 2 seconds passes.
- **The reason** names the checkout and hands over the command for a
  worktree: `git -C <checkout> worktree add <worktree_root>/<repo>/<slug>
  -b <branch>`. For an edit it also names the same file under the new
  worktree. `worktree_root` defaults to `~/.worktrees`. On a machine where
  another tool makes the worktrees, `worktree_commands` names that tool's
  command for the checkouts at or under a path (the longest wins). The
  reason then hands over that command, with `{repo}`, `{checkout}`,
  `{slug}` and `{branch}` filled in. That tool picks the folder, so the
  reason says to edit the file, or run the git command, in the new worktree
  without naming a path.
- **Mode.** `hint` (the default) lets the call through. The hook writes the
  reason as PreToolUse `additionalContext` with no permission decision, so
  Claude Code's own permission check still runs and the model reads the
  advice beside the result. The event is archived at `info`. `deny`
  refuses the call with the reason. A hint is not a clean pass, so no allow
  approves the call.
- **Exempt**: `allow_repos` lists repos by canonical identity, for the
  stores whose workflow is direct work on the default branch. A local store
  with no origin remote has no identity, so `exclude_paths` lists checkout
  paths too. The guard does not read `direct_main_repos`: a repo can push
  straight to main and still take its changes in worktrees.
- **Subagents**: PreToolUse runs for a subagent's tool calls with the same
  payload plus its agent fields, so the guard judges them the same way.
- **Why it exists**: an edit in a main checkout collides with other
  sessions on the same repo. A checkout left dirty or on another branch is
  one the next sync skips. Prose asking agents to use worktrees did not
  hold every time.
- **Fails open**: a relative path, a path outside any checkout, a directory
  only a shell can name, and a command in text the reader could not follow
  all pass. So does git with `--git-dir`, `--work-tree` or `GIT_DIR`.

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
Path-based exceptions use `exclude_paths` (write-internal-names,
script-deny-list, main-checkout): a `~/`- or `/`-prefixed entry matches as
a directory prefix, anything else as a substring. `belt config` prints every list in effect after fallbacks, and
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
  that store, so its index does not load twice. belt reads the setting
  from the files Claude Code reads it from for the session's `cwd`, with
  Claude Code's precedence: the managed settings files win, then the
  project local `.claude/settings.local.json`, then the shared
  `.claude/settings.json` in the cwd, then `~/.claude/settings.json`. Inside
  a git repository the local file sits at the main checkout root, also for
  a linked worktree. Claude Code keeps the local file in the cwd when you
  do not own the repository root or its `.git` or `.claude` entry. git
  refuses a repository you do not own, so belt reads the cwd file there as
  well. belt does not check ownership itself: when `safe.directory` lets
  git open such a repository, belt reads the root file.
  `~/.claude/settings.local.json` is local scope, so it counts only for a
  session started in the home directory. A leading `~` is expanded and
  symlinks are resolved on both sides, so a store linked into a synced
  folder still matches. The store left out counts as absent, and the other
  store gets the whole budget. A missing setting, broken JSON or a wrongly
  typed key in any of those files, or a path that does not resolve keeps
  the store. So does a `git rev-parse` that fails for any reason but status
  128 (not a git repository), a timeout or a missing git included. Without
  the flag, or with any other value, both stores are injected, which is
  what Codex and pi need. `belt doctor` prints one line for a store a
  setting names, read for doctor's own working directory. The line names
  the scope and file the setting came from, then says either that the flag
  leaves the store out or why auto memory may be off. When belt cannot
  tell, doctor prints one `cannot tell` line with the error instead. The
  skip line also says when a SessionStart entry in the Claude settings
  runs the hint without the flag.
  It reads the entry the way belt parses its arguments, so the flag may sit
  before or after the event, a `--config` may sit before `hint`, and the
  binary may be named by a full path. Install a belt that knows the flag
  before a settings entry passes it: an older belt fails on the unknown flag
  and the session starts with no session-start hints.
- **Keeps the store while auto memory is off**, since Claude Code then loads
  nothing from the directory. One switch is `autoMemoryEnabled: false` in
  the highest file that sets it. Another is `CLAUDE_CODE_DISABLE_AUTO_MEMORY=1`
  in the session's environment, where `0` keeps auto memory on whatever the
  files say. The last is `permissions.blockReadsOutsideWorkingDirectories`
  on in any file, when the directory came from a project file. belt does not run git to tell a
  local file you own from one the repository supplies, so under that block
  every project value keeps the store. belt cannot see `--settings`,
  `--bare`, a device management profile or server-managed settings. It
  ignores `CLAUDE_CONFIG_DIR` too and always reads `~/.claude/settings.json`.
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
  `large_file_lines` or more (300 by default) inside a repo csl indexes,
  or in a linked worktree of one. The lookup is the large-read guard's
  (`cslindex.Resolve`), over the same shard listing prefer-csl reads. The
  file must be a type `csl_outline` has symbols for: Go, TypeScript,
  Python, Java, protobuf, shell, or markdown.
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
  `csl_ask` call first, for when only an answer is needed. It is the same
  call the large-read guard hands over, built by the same code. The
  question is a placeholder, and the file goes in as an `f:` query on its
  path. csl reads the path and the repo as regexes, so both are escaped
  and anchored. Then comes the `csl_outline` call with repo and path filled
  in, and `csl_read` or a ranged Read for a region the agent will edit,
  which needs the real lines. The outline call always sets
  `include_tests: true`, because csl leaves test files out of an outline by
  default. For markdown it adds `kinds: ["section"]`, since headings are
  left out otherwise.
- **In a linked worktree** both calls set `worktree` to the worktree's
  root in place of `repo`, so csl answers from the worktree's own index,
  not the main checkout's.
- **The call fits the file**, as the guard's does. Past 6000 lines it sets
  `max_lines` to the line count rounded up to a hundred. Past 20000 lines
  no `csl_ask` call can read the file, so the advice says so and points at
  `csl_read` or a ranged Read instead, then at the outline.
- **Once per file per agent per session**, through the seen file. The
  marker names the file by repo and repo-relative path, so a Read and a
  `csl_read` of one file share a nudge. In a linked worktree the marker
  names the worktree's root in place of the repo, since its file is not
  the main checkout's. A subagent shares its parent's
  session id, so it gets its own nudge, and its read doesn't use up the
  main thread's. A ranged read of the file doesn't use one up either.
- **Silent** without a session id, on a relative path, and outside an
  indexed repo. It's silent on every file type `csl_outline` returns no
  symbols for: JSON, YAML, TOML, plain text, CSV, lock files, JavaScript,
  Dockerfiles and the rest. It's also silent on a response it can't read:
  an image, notebook or PDF read, or a line count that is missing or of
  another JSON type.
- **Silent on a file the `csl_ask` call cannot answer from**, after a
  Read. The rule is the large-read guard's (`cslindex.Skipped`): a
  generated or hidden directory on the path, a `.cslignore` match, a
  symlink, a file over 2 MiB or with a NUL byte. The check reads the file,
  so it runs only once the seen file says the nudge is still due. The
  silence does not use up the nudge. A `csl_read` names no checkout on
  disk, so its file gets no such check.
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
- **Only a commit that landed**: the advice names a commit, so the
  repo's newest HEAD reflog entry (`git reflog -1`, bounded at one
  second) must be a commit (`commit:`, or `commit (amend)`, `(initial)`,
  `(merge)`, `(cherry-pick)`) from the last minute. A commit that
  failed, found nothing staged, or that a pre-commit hook stopped leaves
  the reflog as it was, so it draws nothing. The hook payload cannot say
  so: a Bash `tool_response` carries stdout and stderr, not an exit
  code, and a commit inside a chain (`git commit -m x; echo done`) exits
  on its own anyway. A dry run (`--dry-run`, or `--short`,
  `--porcelain`, `--long`, `-z`/`--null`, which imply it, also inside a
  short bundle such as `-qz`) is skipped from the command, so it stays silent right after an earlier commit
  too. `-n` is `--no-verify` for commit, not a dry run. The minute
  covers what ran after the commit in the same call. A failed commit
  within a minute of a landed one still draws the advice, which then
  names that earlier commit, still on the branch.
- **Deliberately silent** on feature branches, opted-out repos, detached
  HEAD, and repos with no resolvable origin remote. A scratch `git init`
  repo lives its whole life on its default branch and has no upstream to
  protect. Silent too on a commit whose directory only a shell can name (a
  variable, `cd -`, a substitution): a hint that cannot tell says nothing,
  where the push guard fails closed. Silent as well when the call moved
  HEAD again after its commit (`git commit && git pull --rebase`), and
  in a repo with no HEAD reflog (`core.logAllRefUpdates` off). Silent
  also when what ran after the commit in the same call took over a
  minute (`git commit -m x && make test`), and when `GIT_COMMITTER_DATE`
  dates the commit in the past, because the reflog entry carries that
  date. In each case the push guard still stops the push.
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
  csl's shard naming. `large-read` and `read-size` name a linked
  worktree's main checkout the same way, found through the worktree's
  `commondir` file.
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
| temp folders | `/tmp`, `/var/tmp`, `/private/tmp`, `/private/var/tmp`, `~/.tmp` | rm-temp (the folders `toss-bin --temp-only` accepts), file-discard |
| memory block budget, prefix and headers included | 9,000 bytes | agent-memory |
| share held for the work store when both have facts | 2,250 bytes | agent-memory |
| in-flight files asked of moondb | 200 | moondb-inflight |
| in-flight paths per fire | 10 | moondb-inflight |
| published-text excerpt | 200 bytes | humanizer-check |
| session dedupe store | `~/.cache/belt/seen-<session>` | all once-per-session hints |
| agent-memory store paths | `~/.config/agent-memory[-work]/MEMORY.md` | agent-memory |
| csl shard listing | `~/.config/csl/search-index/` | prefer-csl, read-size, large-read |
| csl index file size limit | 2 MiB (`kit/cslignore.SizeMax`, the limit csl's index takes) | large-read, read-size |
| csl_ask lines per call | 6000 by default, 20000 at most (`kit/cslask`, the limits `csl_ask` applies) | large-read, read-size |
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
