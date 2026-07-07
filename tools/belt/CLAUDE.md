# belt — Claude Code guard hooks

Central PreToolUse hook CLI. Guards inspect tool calls before they run and deny risky ones with a reason the model reads. Pairs with `suspenders` (git-hook layer): suspenders guards commits, belt guards the Claude Code session before anything reaches git.

Born out of the July 2026 session retrospective: a session pushed straight to master and tried to self-merge, and internal names reached `git commit` twice before the pre-commit hook caught them. belt moves both checks to the earliest possible point.

## Architecture

- `cmd/belt/` — entrypoint.
- `internal/cli/` — cobra commands: `hook <event>`, `check`, `version`.
- `internal/hook/` — PreToolUse payload parsing + deny JSON emission (deny travels in `hookSpecificOutput.permissionDecision`, exit is always 0).
- `internal/guard/` — the guards. Ordered per event, first deny wins. Every deny reason is prefixed `belt[<guard-id>]:` so a block is always attributable to the guard that fired.
- `internal/config/` — reads `~/.config/belt/config.toml` (toggles), `~/.config/ralph/config.local.toml` (machine profile), `~/.config/suspenders/config.yaml` (internal-name guard section — shared source of truth with the pre-commit guard), and the `permissions.deny` Bash entries of `~/.claude/settings.json` + `settings.local.json` (shared source of truth with the permission system for the script guard).
- `internal/notify/` — synchronous best-effort event emission to events.this on every deny (the standard per-tool copy; synchronous because a hook CLI exits immediately).

## Guards

| id | event | rule |
|----|-------|------|
| `git-push-main` | `bash` | Deny `git push` targeting main/master unless the ralph profile is `personal`. Unknown profile fails closed. Resolves bare `git push`/`HEAD` refspecs via `git rev-parse --abbrev-ref HEAD` in the payload cwd (or `git -C` dir). |
| `script-deny-list` | `bash` | Deep deny inspection: apply the Bash deny list inside scripts, closing the "write it to a script, then run the script" bypass. Scans executed/sourced script files (`bash x.sh`, `python x.py`, `./x.sh`, `source x.sh`, relative paths resolved against the payload cwd), `-c` strings, and heredocs piped into an interpreter. Patterns = `permissions.deny` `Bash(...)` entries read live from `~/.claude/settings.json` + `settings.local.json`, plus `extra_patterns` in the belt config (carries `rm -rf`/`rm -fr`, since settings only has `rm` under "ask"). Matching is per non-comment line, whole-word, whitespace-normalized — catches shell lines and Python `subprocess`/`os.system` strings alike. Unreadable files and `python -m` allow; `exclude_paths` skips trusted script dirs. |
| `write-internal-names` | `write` | Deny Write/Edit content that mentions internal names when the target file is in a github.com repo. Names = suspenders `guard.blocked_words` + top-level dir names under `guard.workspace_dirs`, minus `guard.allowlist`. Only github.com remotes count as public — other git hosts and non-repo paths are exempt. Per-guard `exclude_paths` (substring match) skips paths that deliberately carry internal references — they live in the consuming repo's belt config overlay, not here (see docs/adr/0006). |

Adding a guard: implement the `Guard` interface in `internal/guard/`, register it in `ForEvent`, and add a toggle to the consuming repo's belt config overlay (`~/.config/belt/config.toml`). Only a guard that needs a new tool matcher also needs a `hooks.PreToolUse` entry in the consuming repo's Claude settings recipe.

## Commands

```
belt hook bash|write     # hook entrypoint: payload on stdin, deny JSON on stdout
belt check bash "git push origin main"                # dry-run, one verdict line per guard
belt check write --file <path> --content "text"
belt version
```

## Build & test

```
make build / make install / make test / make lint
```

## Gotchas

- The bash-command parser is token-based, not a shell parser. It splits on `&&`/`||`/`;`/`|`/newlines and matches bare `git … push` sequences. Pushes buried in quoted strings or subshell tricks are not caught — belt is a guardrail against habit, not an adversary-proof sandbox.
- Same for `script-deny-list`: flag reordering (`rm -fr` is covered, `rm -r -f` is not), `curl | bash`, and Python list-form `subprocess.run(["rm", "-rf", …])` slip through. Add variants to `extra_patterns` as they come up.
- `script-deny-list` reads the settings deny list at hook time, so a settings edit applies to the script guard immediately — unlike hook *registration* changes, which need a new session.
- Hook changes (settings or binary behavior) take effect in the NEXT Claude session; a running session keeps its loaded settings.
- `config.Load()` never errors: missing config files mean zero values. Missing ralph profile ⇒ `git-push-main` fails closed (denies pushes to main everywhere), missing suspenders config ⇒ `write-internal-names` has an empty name list and allows every write — keep suspenders configured.
- Keep literal internal hostnames and org names OUT of belt's own source and docs; this repo is heading public and the suspenders pre-commit guard blocks them. The public/internal split is derived (`github.com` in the remote = public), never enumerated.
