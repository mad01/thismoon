# ADR-0032: belt may answer allow, for read-only `git -C` calls

**Date:** 2026-11-12
**Status:** Accepted

**Scope:** `tools/belt`. Amends ADR-0008. Decided in MAD-557.

## Context

Claude Code allow rules match the command text as written.
`Bash(git status:*)` does not match `git -C <path> status`, so that form
goes to the auto-mode classifier or a prompt. Sessions that work across
worktrees run it all the time.

A settings rule cannot fix this safely. `Bash(git -C * status *)` also
matches `git -C x -c core.fsmonitor=<cmd> status`, which runs code. A rule
sees text. It cannot tell where git's global options end.

A PreToolUse hook can answer `allow`, which skips the permission prompt.
Per the Claude Code hooks documentation, it does not override a settings
deny or ask rule. belt already reads every Bash call at that point, but
until now it only ever answered deny.

## Decision

belt carries a fourth concept. ADR-0008's "belt carries two concepts",
which ADR-0024 made three, now reads four:

- An **allow** sits at PreToolUse and returns an approval or nil. It has
  no denial path. The hook runs the allows only after a clean guard pass:
  no guard denied, no custom guard failed open (timed out, failed to
  start, or exited with neither 0 nor 1), and no guard in soft mode let
  the call through with a warning. An unreadable payload or config denies
  before either runs. So a deny, or a warning that stands in for one,
  always wins.

The one allow is `git-c-read`. It answers only a plain Bash tool call:
the tool name is exactly `Bash`, neither `dangerouslyDisableSandbox` nor
`run_in_background` is true, and the input holds no key belt does not
know. Every command in the call must be `git -C <dir> <subcommand> [args]`:

- The call has no shell syntax beyond `;`, `&&`, `||`, newlines and
  quotes. No pipes, redirects, substitutions, variables, globs, wrappers
  or env prefixes. A `~` that starts a word has no quote before its first
  unquoted `/`, since zsh and bash read `~'/x'` differently.
- Exactly one `-C` comes right after `git`, with no other global option.
- The subcommand only reads: status, log, diff, show, rev-parse, ls-files,
  and the listing forms of branch, remote and worktree.
- No argument writes a file or runs a program (`--output`, `--ext-diff`,
  `--textconv`, `--no-index`, `-O` and others, abbreviations included).
  No argument names a path outside the repo by its text: an absolute, `~`
  or `..` path gets no answer as a positional argument or as an
  `--option=value` value, and so do the options that read a file their
  value names (`--exclude-from`, `-X`, `--resolve-git-dir`). Arguments are
  judged by their text only, so a relative one that names a symlink in the
  work tree can still make git read the file behind it.
- The dir resolves, symlinks followed the way git follows them, and the
  repo git would discover from it is one belt can vouch for. belt finds it
  by walking up the way git does, without running git:
  - No directory from the dir up to the top of the work tree looks like a
    git directory (a `HEAD` beside `objects`, `refs`, `commondir` or
    `config`). git would use one, config and all, and a clone can carry
    one as tracked files.
  - The top is the first directory with a `.git` entry. That entry is a
    real directory git accepts, or the `.git` file `git worktree add`
    writes, naming `<main>/.git/worktrees/<name>` with a `commondir` and a
    `gitdir` that lead back. A symlinked `.git`, a broken `.git` directory
    git would walk past, a submodule's `.git` file, or one naming anything
    else gets no answer.
  - The work tree and every git directory git reads sit inside a root. A
    linked worktree needs its own folder and its main checkout's `.git`
    under roots.
  - The repo's `config` and `config.worktree` set no `core.worktree`, no
    true `core.bare`, no `core.hooksPath`, and no `include` or `includeIf`.
    A file the scan cannot read with certainty counts as setting one.
  - belt's own environment sets no `GIT_DIR`, `GIT_WORK_TREE`,
    `GIT_COMMON_DIR`, `GIT_INDEX_FILE` or `GIT_OBJECT_DIRECTORY`.

The roots are the work tree of the session's cwd, unless that is home or
above it, plus `allows.git-c-read.roots`. Each root resolves the same
physical way as a `-C` dir. A root that is `/`, home or above home, or
that does not resolve, is dropped with a `belt doctor` warning. That list
is the safety boundary. It is purpose-named and read by this allow alone
(ADR-0013).

Every allow is off until `allows.<id>.enabled` is set. Guards and hints
default on, because a missing config must not disarm them. An allow
defaults off, because a missing config must not approve anything.

When the reader is unsure, it gives no answer, and the call goes to Claude
Code's own permission check as before. An approval posts no event: it
would be belt's most frequent one and push guard denials out of the events
service's per-source history.

## Consequences

- The roots carry a trust assumption. Any repo under a root can carry
  config or an embedded git directory from wherever it came. The check
  refuses embedded git directories and the config keys above. It trusts
  the rest of a repo's own `.git/config`, which a clone never writes: the
  user or a tool on the machine wrote it. The read subcommands still run
  code that config names. `git status` and `git diff` run `core.fsmonitor`
  and clean filters. `git status` runs the `post-index-change` hook when it
  refreshes the index. `git diff` runs `diff.external` and diff drivers,
  and `git diff`, `git log -p` and `git show` run textconv filters, picked
  per path by a tracked `.gitattributes`. A partial clone fetches missing
  objects from its remote. `Bash(git status:*)` already accepts this for
  the cwd repo. `-C` widens it to every repo under the roots. So a tree
  that carries its own `.git` directory without being a clone, such as an
  unpacked archive, must stay out of them: whoever made the archive wrote
  its config.
- belt sees only its own environment. A `GIT_DIR` or `GIT_CONFIG_*` that
  the session's shell sets in an rc file is out of its sight.
- belt checks the files at hook time. A process that changes the repo
  between that check and git's run is out of scope.
- Whether a hook allow also skips the auto-mode classifier is not
  documented and not verified. To verify it, turn the allow on, run a live
  auto-mode session that calls `git -C <repo> status`, and check the
  transcript for a classifier call on that tool use.
- Revisit when the classifier effect is known, since an allow that does
  not skip it buys only the prompt. Revisit too when Claude Code rules
  learn to parse git's global options. A repo under a root found with a
  hostile config is another trigger, and so is a proposed second allow.
- Adding an allow means implementing the `Allow` interface in
  `internal/guard`, registering it in `guard.Allows`, and documenting its
  `allows.<id>` key. It stays off by default.
