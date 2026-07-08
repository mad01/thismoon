# belt, Claude Code PreToolUse guard hooks

Go CLI. Central PreToolUse hook for Claude Code sessions: guards inspect a tool call before it runs and deny risky ones with a reason the model reads. Pairs with `suspenders` (the git-hook layer): suspenders guards commits, belt guards the session before anything reaches git.

Born out of a July 2026 session retrospective: a session pushed straight to master and tried to self-merge, and internal names reached `git commit` twice before the pre-commit hook caught them. belt moves both checks to the earliest point a hook can intervene.

## Module layout

```
belt/
  cmd/belt/          - entrypoint
  internal/
    cli/             - cobra commands: `hook <event>`, `check`, `version`
    hook/             - PreToolUse payload parsing + deny JSON emission
    guard/            - the guards (git-push-main, script-deny-list, write-internal-names)
    config/           - reads belt/ralph/suspenders/Claude-settings config surfaces
    notify/           - synchronous best-effort event emission to events.this on every deny
  Makefile            - package path github.com/mad01/thismoon/tools/belt (monorepo module, no own go.mod)
```

## How it works

`internal/hook` parses a PreToolUse payload from stdin, runs the guards registered for that event, and writes a deny decision to stdout. Deny always travels in `hookSpecificOutput.permissionDecision` with exit 0: a hook must not break tool calls on its own bugs, so the deny is data, not a process exit code. Guards for an event run in registration order and the first denial wins; every deny reason is prefixed `belt[<guard-id>]:` so a block is always attributable to the guard that fired.

`internal/config` reads four surfaces so belt's guards never drift from the systems they overlap with: `~/.config/belt/config.toml` (per-guard toggles, exclude paths, extra patterns), `~/.config/ralph/config.local.toml` (the machine profile), `~/.config/suspenders/config.yaml` (the internal-name guard section, shared source of truth with the pre-commit guard), and the `permissions.deny` Bash entries of `~/.claude/settings.json` + `settings.local.json` (shared source of truth with the permission system for the script guard). Every file is optional: `config.Load()` never errors, and a missing file yields zero values.

### Guards

| id | event | rule |
|----|-------|------|
| `git-push-main` | `bash` | Deny `git push` targeting main/master unless the ralph profile is `personal`. Unknown profile fails closed. Resolves bare `git push`/`HEAD` refspecs via `git rev-parse --abbrev-ref HEAD` in the payload cwd (or `git -C` dir). |
| `script-deny-list` | `bash` | Deep deny inspection: apply the Bash deny list inside scripts, closing the "write it to a script, then run the script" bypass. Scans executed/sourced script files (`bash x.sh`, `python x.py`, `./x.sh`, `source x.sh`, relative paths resolved against the payload cwd), `-c` strings, and heredocs piped into an interpreter. Patterns = `permissions.deny` `Bash(...)` entries read live from `~/.claude/settings.json` + `settings.local.json`, plus `extra_patterns` in the belt config (carries `rm -rf`/`rm -fr`, since settings only lists `rm` under "ask"). Matching is per non-comment line, whole-word, whitespace-normalized; it catches shell lines and Python `subprocess`/`os.system` strings alike. Unreadable files and `python -m` allow; `exclude_paths` skips trusted script dirs. |
| `write-internal-names` | `write` | Deny Write/Edit content that mentions internal names when the target file is in a github.com repo. Names = suspenders `guard.blocked_words` + top-level dir names under `guard.workspace_dirs`, minus `guard.allowlist`. Only github.com remotes count as public; other git hosts and non-repo paths are exempt. Per-guard `exclude_paths` (substring match) skips paths that deliberately carry internal references; those live in the consuming repo's belt config overlay, not here (see docs/adr/0006). |

Adding a guard: implement the `Guard` interface in `internal/guard/`, register it in `ForEvent`, and add a toggle to the consuming repo's belt config overlay (`~/.config/belt/config.toml`). Only a guard that needs a new tool matcher also needs a `hooks.PreToolUse` entry in the consuming repo's Claude settings recipe.

## Build / install / test

```bash
make build    # produces ./belt binary
make install  # build + cp to ~/code/bin/belt + adhoc codesign
make test     # go test ./...
make lint     # golangci-lint run ./...
```

## Commands

```
belt hook bash|write     # hook entrypoint: payload on stdin, deny JSON on stdout
belt check bash "git push origin main"                # dry-run, one verdict line per guard
belt check write --file <path> --content "text"
belt version
```

## Gotchas

- **The bash-command parser is token-based, not a shell parser.** It splits on `&&`/`||`/`;`/`|`/newlines and matches bare `git … push` sequences. Pushes buried in quoted strings or subshell tricks aren't caught; belt is a guardrail against habit, not an adversary-proof sandbox.
- **Same for `script-deny-list`.** Flag reordering (`rm -fr` is covered, `rm -r -f` isn't), `curl | bash`, and Python list-form `subprocess.run(["rm", "-rf", …])` slip through. Add variants to `extra_patterns` as they come up.
- **`script-deny-list` reads the settings deny list at hook time**, so a settings edit applies to the script guard immediately, unlike hook *registration* changes, which need a new session.
- **Hook changes (settings or binary behavior) take effect in the next Claude session.** A running session keeps its loaded settings.
- **`config.Load()` never errors**: missing config files mean zero values. Missing ralph profile means `git-push-main` fails closed (denies pushes to main everywhere); missing suspenders config means `write-internal-names` has an empty name list and allows every write. Keep suspenders configured.
- **Keep literal internal hostnames and org names out of belt's own source and docs.** This repo is heading public and the suspenders pre-commit guard blocks them. The public/internal split is derived (`github.com` in the remote means public), never enumerated.

## See also

- Recipe: `recipes/belt/recipe.toml` (this repo: build/install only, wave 0, builds before the consuming repo's claude recipe registers the hooks)
- Config overlay + hook registration: the consuming repo's companion recipe carries `~/.config/belt/config.toml` (with its internal-name `exclude_paths`) and the `hooks.PreToolUse` settings entries (machine-private wiring, see docs/adr/0006)
- Pairs with `suspenders` (git pre-commit guard layer, same internal-name source of truth)
