# belt, Claude Code guard and hint hooks

Go CLI. Central hook binary for Claude Code sessions, with two halves: **guards** inspect a tool call before it runs and deny risky ones with a reason the model reads, **hints** inspect a tool call after it ran and add advisory context the model reads next to the result. Pairs with `suspenders` (the git-hook layer): suspenders guards commits, belt guards the session before anything reaches git.

The split is load-bearing (docs/adr/0008): a guard exists to prevent damage that is hard to undo, a hint exists to improve a choice that was merely suboptimal. A hint has no denial path in its interface, so it cannot block a tool call even by mistake.

Born out of a July 2026 session retrospective: a session pushed straight to master and tried to self-merge, and internal names reached `git commit` twice before the pre-commit hook caught them. belt moves both checks to the earliest point a hook can intervene.

## Module layout

```
belt/
  cmd/belt/          - entrypoint
  internal/
    cli/             - cobra commands: `hook <event>`, `hint <event>`, `check`, `doctor`, `config`, `version` (build metadata from the shared buildinfo package)
    hook/             - payload parsing + JSON emission for both events
    guard/            - the guards (git-push-main, script-deny-list, write-internal-names)
    hint/             - the hints (prefer-csl, keep-assertions, keep-consult) + csl index lookup, response parsing, session dedupe
    config/           - belt config (profiles, internal_names, toggles) + ralph/suspenders fallbacks + Claude-settings deny list
    notify/           - synchronous best-effort event emission to events.this on every deny and hint
  Makefile            - package path github.com/mad01/thismoon/tools/belt (monorepo module, no own go.mod)
```

## How it works

`internal/hook` parses a PreToolUse payload from stdin, runs the guards registered for that event, and writes a deny decision to stdout. Deny always travels in `hookSpecificOutput.permissionDecision` with exit 0: a hook must not break tool calls on its own bugs, so the deny is data, not a process exit code. Guards for an event run in registration order and the first denial wins; every deny reason is prefixed `belt[<guard-id>]:` so a block is always attributable to the guard that fired.

`internal/config` treats `~/.config/belt/config.yaml` as the belt-owned surface: per-guard toggles, exclude paths, extra patterns, `profiles`, and the `internal_names` section (a legacy `config.toml` beside it is read only when the YAML file is absent, and a present-but-broken file of either format yields defaults rather than the other file). Two settings fall back to the tool that originated them when belt doesn't set them: `profiles` to `~/.config/ralph/config.local.toml`, and `internal_names` to the `guard:` section of `~/.config/suspenders/config.yaml`. `Config` records which source won (`ProfileSource`/`NamesSource`, shown by doctor). The `permissions.deny` Bash entries of `~/.claude/settings.json` + `settings.local.json` are always read live (shared source of truth with the permission system for the script guard). Every file is optional: `config.Load()` never errors, and a missing file yields zero values.

### Guards

| id | event | rule |
|----|-------|------|
| `git-push-main` | `bash` | Deny `git push` targeting main/master unless the ralph profile is `personal` or the target repo is on the guard's `allow_repos` allowlist (canonical `host/owner/repo`, e.g. `github.com/mad01/dotfiles`, resolved from the push working dir's `origin` remote). Unknown profile, unresolved repo, and off-allowlist repos all fail closed. Resolves bare `git push`/`HEAD` refspecs via `git rev-parse --abbrev-ref HEAD` in the payload cwd (or `git -C` dir). |
| `script-deny-list` | `bash` | Deep deny inspection: apply the Bash deny list inside scripts, closing the "write it to a script, then run the script" bypass. Scans executed/sourced script files (`bash x.sh`, `python x.py`, `./x.sh`, `source x.sh`, relative paths resolved against the payload cwd), `-c` strings, and heredocs piped into an interpreter. Patterns = `permissions.deny` `Bash(...)` entries read live from `~/.claude/settings.json` + `settings.local.json`, plus `extra_patterns` in the belt config (carries `rm -rf`/`rm -fr`, since settings only lists `rm` under "ask"). Matching is per non-comment line, whole-word, whitespace-normalized; it catches shell lines and Python `subprocess`/`os.system` strings alike. Unreadable files and `python -m` allow; `exclude_paths` skips trusted script dirs. |
| `write-internal-names` | `write` | Deny Write/Edit content that mentions internal names when the target file is in a github.com repo. Names = `internal_names.blocked_words` plus, for every git repo `kit/repofind` discovers under `internal_names.workspace_dirs`, the org and repo segments of its origin remote (path fallback) and the checkout dir basename — each a separate name, never the combined `org/repo` — minus `internal_names.allowlist` and anything under three characters. The same derivation the suspenders pre-commit guard uses, so the two layers agree when their configs do. Only github.com remotes count as public; other git hosts and non-repo paths are exempt. Per-guard `exclude_paths` skips paths that deliberately carry internal references (`~/` and absolute entries match as directory prefixes, others as substrings); those live in the consuming repo's belt config overlay, not here (see docs/adr/0006). |

Adding a guard: implement the `Guard` interface in `internal/guard/`, register it in `All`, and add a toggle to the consuming repo's belt config overlay (`~/.config/belt/config.yaml`). Only a guard that needs a new tool matcher also needs a `hooks.PreToolUse` entry in the consuming repo's Claude settings recipe.

### Hints

| id | event | rule |
|----|-------|------|
| `prefer-csl` | `bash` | After a bash command sweeps multiple files inside a csl-indexed repo, hand back the equivalent `csl_search` call with the pattern translated to zoekt. Indexed-repo lookup reads the shard listing in `~/.config/csl/search-index/` directly (no csl process launch). Silent on pipe filters (`cmd \| grep x`), single-file greps, `ls`/`cat`, and paths outside an indexed repo. |
| `keep-assertions` | `search` | After a csl search, surface keep assertions about the code the search hit. Queries `keep serve` on `KEEP_PORT` (default 7431) with a 400ms budget; keep being down means silence. Caps at 3, drops retracted, marks stale, dedupes per session via `~/.cache/belt/seen-<session>`. |
| `keep-consult` | `session-start` | When a session opens, surface the cwd repo's keep assertions before any searching happens. Derives org/name from the git origin remote (exec `git remote get-url origin`), queries the same `keep serve` API, and matches the repo-name boundary so `thismoon` does not drag in `thismoon-arcade`. Caps at 5; outside a repo, or with keep down, silence. Rides the SessionStart hook, not PostToolUse. |

Adding a hint: implement the `Hint` interface in `internal/hint/`, register it in `hint.ForEvent`, and add a `[hints.<id>]` toggle to the consuming repo's config overlay. A hint returns `*Advice` or nil — there is no way for it to deny.

## Build / install / test

```bash
make build    # produces ./belt binary
make install  # build + cp to ~/code/bin/belt + adhoc codesign
make test     # go test ./...
make lint     # golangci-lint run ./...
```

## Commands

```
belt hook bash|write     # PreToolUse entrypoint: payload on stdin, deny JSON on stdout
belt hint search|bash    # PostToolUse entrypoint: payload on stdin, additionalContext JSON on stdout
belt hint session-start  # SessionStart entrypoint: same contract, emits hookEventName SessionStart
belt check bash "git push origin main"                # dry-run, one verdict line per guard
belt check write --file <path> --content "text"
belt doctor              # build metadata + resolved config: surfaces loaded, guard/hint state, blocked names
belt config              # config file locations + every setting in effect, incl. resolved fallbacks and claude deny patterns (annotated reference in --help)
belt version [-o json]   # bare version token, or the four-key build metadata object
```

## Gotchas

- **The bash-command parser is token-based, not a shell parser.** It splits on `&&`/`||`/`;`/`|`/newlines and matches bare `git … push` sequences. Pushes buried in quoted strings or subshell tricks aren't caught; belt is a guardrail against habit, not an adversary-proof sandbox.
- **Same for `script-deny-list`.** Flag reordering (`rm -fr` is covered, `rm -r -f` isn't), `curl | bash`, and Python list-form `subprocess.run(["rm", "-rf", …])` slip through. Add variants to `extra_patterns` as they come up.
- **`script-deny-list` reads the settings deny list at hook time**, so a settings edit applies to the script guard immediately, unlike hook *registration* changes, which need a new session.
- **Hook changes take effect immediately, including registration.** Verified 2026-07-18 by adding a `PostToolUse` block to `~/.claude/settings.json` mid-session: the next tool call ran the new hook. This corrects an earlier note here claiming a running session keeps its loaded settings and that registration needs a new session — that is not the behavior. Binary changes apply immediately for the same reason: the hook is a fresh process per tool call, so `make install` is live at once.
- **`prefer-csl` does not reuse `guard.splitSegments`.** That splitter preserves byte offsets and treats `|` exactly like `&&`, which is right for finding a denied command anywhere in a pipeline and wrong for this hint: telling a pipe filter apart from a standalone search is its entire precision requirement. The hint has its own quote-aware splitter so a `|` inside a grep pattern does not read as a pipe.
- **keep subjects are shallower than search hits.** Assertions get labelled at the component level (`.../services/csl`) while searches return hits deeper (`.../services/csl/internal/semantic`), and keep matches subjects by prefix — so querying the hit's own subject finds nothing. `keep-assertions` queries the repo and narrows by ranking on shared path segments. Changing that to a narrower query silently returns zero results rather than erroring.
- **`config.Load()` never errors**: missing config files mean zero values. No profiles anywhere (belt config and ralph fallback both empty) means `git-push-main` fails closed (denies pushes to main everywhere); no name config anywhere (no `internal_names` in the belt config and no suspenders guard section) means `write-internal-names` has an empty name list and allows every write. Keep one of the two name sources configured.
- **Version probe convention.** `belt version -o json` returns the shared four-key build metadata object (`version`, `commit`, `tag`, `build_time`, every key present and `""` when unknown) from `github.com/mad01/thismoon/buildinfo`, injected by `buildinfo.mk` at link time. Plain `belt version` stays a bare token — ralph and status parse it as one. `belt doctor` opens with the same metadata, so a doctor report always names the binary that produced it.
- **Keep literal internal hostnames and org names out of belt's own source and docs.** This repo is heading public and the suspenders pre-commit guard blocks them. The public/internal split is derived (`github.com` in the remote means public), never enumerated.

## See also

- Recipe: `recipes/belt/recipe.toml` (this repo: build/install only, wave 0, builds before the consuming repo's claude recipe registers the hooks)
- Config overlay + hook registration: the consuming repo's companion recipe carries `~/.config/belt/config.yaml` (with its internal-name `exclude_paths`) and the `hooks.PreToolUse` settings entries (machine-private wiring, see docs/adr/0006)
- Pairs with `suspenders` (git pre-commit guard layer, same internal-name source of truth)
