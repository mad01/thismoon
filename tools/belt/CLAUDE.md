# belt, Claude Code guard and hint hooks

Go CLI. Central hook binary for Claude Code sessions, with two halves: **guards** inspect a tool call before it runs and deny risky ones with a reason the model reads, **hints** inspect a tool call after it ran and add advisory context the model reads next to the result. Pairs with `suspenders` (the git-hook layer): suspenders guards commits, belt guards the session before anything reaches git.

The split is load-bearing (docs/adr/0008): a guard exists to prevent damage that is hard to undo, a hint exists to improve a choice that was merely suboptimal. A hint has no denial path in its interface, so it cannot block a tool call even by mistake.

Born out of a July 2026 session retrospective: a session pushed straight to master and tried to self-merge, and internal names reached `git commit` twice before the pre-commit hook caught them. belt moves both checks to the earliest point a hook can intervene.

## Module layout

```
belt/
  cmd/belt/          - entrypoint
  internal/
    cli/             - cobra commands: `hook <event>`, `hint <event>`, `check`, `doctor`, `config`, `override`, `docs`, `version` (build metadata from the shared buildinfo package)
    hook/             - payload parsing + output emission per event (deny JSON, additionalContext JSON, or plain stdout for prompt)
    guard/            - the guards (git-push-main, git-identity, commit-guard, script-deny-list, write-internal-names) + config-registered custom guards
    hint/             - the hints (prefer-csl, kof-assertions, kof-consult, kof-deposit, agent-memory, humanizer-check) + csl index lookup, response parsing, session dedupe
    config/           - belt config (internal_names, claude_settings, toggles, guard rules) + gated Claude-settings deny list
  Makefile            - package path github.com/mad01/thismoon/tools/belt (monorepo module, no own go.mod)

Event archiving goes through the shared `github.com/mad01/thismoon/kit/notify`
package (`EmitEventSync` — belt exits immediately after a deny, so the async
form would be killed before the POST lands).
```

## How it works

`internal/hook` parses a PreToolUse payload from stdin, runs the guards registered for that event, and writes a deny decision to stdout. Deny always travels in `hookSpecificOutput.permissionDecision` with exit 0: a hook must not break tool calls on its own bugs, so the deny is data, not a process exit code. Guards for an event run in registration order and the first denial wins; every deny reason is prefixed `belt[<guard-id>]:` so a block is always attributable to the guard that fired.

`internal/config` treats `~/.config/belt/config.yaml` as the belt-owned surface: per-guard toggles, exclude paths, extra patterns, `claude_settings`, and the `internal_names` section (`$XDG_CONFIG_HOME` moves the directory; `--config`/`$BELT_CONFIG` relocate the file itself. A legacy `config.toml` beside the default file is read only when the YAML file is absent, and a present-but-broken file of either format never falls back to the other one). The config is standalone and has no machine-profile concept (docs/adr/0010): belt never reads another tool's file, an absent `internal_names` section means an empty name set, and the provisioning layer renders one config per machine class — a guard or rule meant for only some machines is simply absent (or disabled) in the other classes' files. Exactly one non-belt surface is read: the `permissions.deny` Bash entries of `~/.claude/settings.json` + `settings.local.json`, read live for the script guard (shared source of truth with the permission system), gated by `claude_settings.enabled` (default true).

A missing config file yields the armed defaults with no error. A config file that is present and fails to parse or validate is an error `config.Load` returns, and `belt hook` turns it into a `belt[config]:` deny on every tool call — belt cannot distinguish "no rules" from "the rules did not load", so it blocks rather than run unarmed (`doctor` and `config` still print, flagging that the state shown is the defaults). Validation covers the keys that parse and then do nothing: a `custom_guards.<name>.event` outside `bash`/`write`, and `mode:` values that are neither `hard` nor `soft` or that sit on a guard outside `config.SoftModeGuards`.

### Guards

| id | event | rule |
|----|-------|------|
| `git-push-main` | `bash` | Deny `git push` targeting main/master unless the target repo is on the guard's `allow_repos` allowlist (canonical `host/owner/repo`, e.g. `github.com/mad01/dotfiles`, or a trailing `/*` org wildcard, resolved from the push working dir's `origin` remote). Unresolved repos and off-allowlist repos fail closed; a machine class where direct pushes are fine disables the guard in its rendered config. Resolves bare `git push`/`HEAD` refspecs via `git rev-parse --abbrev-ref HEAD` in the payload cwd (or `git -C` dir). |
| `git-identity` | `bash` | Deny `git commit` when the repo's effective `git config user.email` does not match the first `git_identity` rule covering the repo. Rules are repo-pattern scoped (exact `host/owner/repo` or trailing `/*` org wildcard) rather than profile scoped: the repo decides the expected identity whatever machine the commit happens on, so a work machine committing to a personal repo in the evening still gets the personal email enforced. `mode: soft` downgrades the deny to a warn event. No config, an unresolved repo or email, and uncovered repos all fail open. |
| `commit-guard` | `bash` | The work-hours nudge: `git commit` to repos matching a `commit_guards` rule inside its local-time window (`block_hours`, `block_days` defaulting to mon–fri) is denied (`mode: hard`) or warned about via the events service (`mode: soft`, the default). A rule meant for one machine class lives in that class's rendered config. `always_allow` exempts repos needed at any hour; a rule's `override` names a timed switch (`belt override set <name> --reason "..."`, 10m default, `--for` to size, `extend` to push; set and extend require a non-blank `--reason` archived to the events service) that suppresses it with a warn event until it expires or is cleared. Unresolved repos and malformed windows fail open. |
| `script-deny-list` | `bash` | Deep deny inspection: apply the Bash deny list inside scripts, closing the "write it to a script, then run the script" bypass. Scans executed/sourced script files (`bash x.sh`, `python x.py` incl. versioned names, `perl`/`ruby`/`node`/`osascript`, `uv run`, `./x.sh`, extensionless `./x` with a shebang, `source x.sh`, relative paths resolved against the payload cwd), `-c`/`-e` strings, heredocs, `bash < x.sh` stdin redirects, `cat x.sh \| bash` pipes, and the command after the indirection heads `eval`, `xargs`, and `find -exec`. A file written and run in one command (`echo '…' > s.sh && bash s.sh`, `tee` too) gets the raw command text scanned — at check time the file doesn't exist yet. `curl\|wget` piped into an interpreter denies outright: nothing can read what would run. Patterns = `permissions.deny` `Bash(...)` entries read live from `~/.claude/settings.json` + `settings.local.json` (read gated by `claude_settings.enabled`, default true), plus `extra_patterns` in the belt config (carries `rm -rf`/`rm -fr`, since settings only lists `rm` under "ask"); a `re:` prefix compiles the rest as a case-insensitive regex for flag-reordering shapes. Matching is per non-comment logical line (backslash continuations joined), whole-word, whitespace-normalized, with a second pass that collapses quotes/commas/brackets so `kubectl "delete"` and list-form `subprocess.run(["rm", "-rf", …])` match too. Unreadable files and `python -m` allow; `exclude_paths` skips trusted script dirs; `mode: soft` turns denials into warn events for pattern rollout. |
| `write-internal-names` | `write` | Deny Write/Edit content that mentions internal names when the target file is in a github.com repo. Names = `internal_names.blocked_words` plus, for every git repo `kit/repofind` discovers under `internal_names.workspace_dirs`, the org and repo segments of its origin remote (path fallback) and the checkout dir basename — each a separate name, never the combined `org/repo` — minus `internal_names.allowlist` and anything under three characters. The same derivation the suspenders pre-commit guard uses, so the two layers agree when their configs do. `internal_names.allow_phrases` entries are neutralized in the content before matching, so a sanctioned compound carrying a blocked name passes while the bare name still denies. Only github.com remotes count as public; other git hosts and non-repo paths are exempt. Per-guard `exclude_paths` skips paths that deliberately carry internal references (`~/` and absolute entries match as directory prefixes, others as substrings); those live in the consuming repo's belt config overlay, not here (see docs/adr/0006). |

Adding a guard: implement the `Guard` interface in `internal/guard/`, register it in `All`, and add a toggle to the consuming repo's belt config overlay (`~/.config/belt/config.yaml`). Only a guard that needs a new tool matcher also needs a `hooks.PreToolUse` entry in the consuming repo's Claude settings recipe.

**Custom guards** skip the Go step entirely: a `custom_guards` entry in the belt config registers a named guard that execs an external command with the tool-call fields as JSON on stdin (`{"command","cwd"}` for bash, `{"file_path","content","cwd"}` for write). Exit 0 allows; exit 1 denies with stdout line one as the reason (prefixed `belt[<name>]:`); exit 2+, a 5s timeout, or a start failure allow with a warn event, so a broken external fails open. An optional `match` substring gates when the external is exec'd at all, and `mode: soft` turns denials into warn events. Custom guards run after the built-ins, alphabetical by name; `belt doctor` lists them with a PATH reachability check. The commands run with the user's full environment on purpose — they are user-configured, not agent-configured, and belt never writes its own config.

### Hints

| id | event | rule |
|----|-------|------|
| `prefer-csl` | `bash` | After a bash command sweeps multiple files inside a csl-indexed repo, hand back the equivalent `csl_search` call with the pattern translated to zoekt. Indexed-repo lookup reads the shard listing in `~/.config/csl/search-index/` directly (no csl process launch). Silent on pipe filters (`cmd \| grep x`), single-file greps, `ls`/`cat`, and paths outside an indexed repo. |
| `kof-assertions` | `search` | After a csl search, surface kof assertions about the code the search hit. Queries `kof serve` on `KOF_PORT` (default 7431) with a 400ms budget; kof being down means silence. Caps at 3, drops retracted, marks stale, dedupes per session via `~/.cache/belt/seen-<session>`. |
| `kof-consult` | `session-start` | When a session opens, surface the cwd repo's kof assertions before any searching happens. Derives org/name from the git origin remote (exec `git remote get-url origin`), queries the same `kof serve` API, and matches the repo-name boundary so `thismoon` does not drag in `thismoon-arcade`. Caps at 5; outside a repo, or with kof down, silence. Rides the SessionStart hook, not PostToolUse. |
| `agent-memory` | `session-start` | Inject the agent memory indexes when a session opens — the deterministic replacement for the "read the index at session start" instruction prose. Two stores: the personal `~/.config/agent-memory/MEMORY.md` (every machine) and `~/.config/agent-memory-work/MEMORY.md` (work machines only); each renders when present, missing file = silence, so personal machines never see the work store. Fact bullets only (header prose dropped), no per-session dedupe (the index is the thing to see every session), capped at 30 facts per store with a store-hygiene flag past that. |
| `kof-deposit` | `prompt` | Once per session, nudge a session that did substantial work (≥30 tool_use blocks in the transcript) but never called `kof_assert` to deposit what it derived. Matches the `"name":"mcp__kof__kof_assert"` JSON key form, not the bare name — prose mentions in echoed instructions must not count as a deposit. Rides UserPromptSubmit and emits plain stdout text (that event adds stdout to context; it does not take the additionalContext envelope). Skips before the transcript read once the seen-file marker is set; no session id means no nudge. |
| `humanizer-check` | `external-text` | Once per session, after an MCP call publishes text off this machine (issue and PR comments, Slack messages), point at `humanizer_detect` while the wording is still editable. Which tools reach the hint is the consuming repo's matcher (docs/adr/0006); belt only drops calls whose operation names a read verb at either end (`get_file_contents`, `issue_read`), so a server-wide matcher does not spend the nudge on a fetch. Silent once the seen-file marker is set, when the transcript already holds an `mcp__humanizer__` tool_use block, or with no session id. |

Adding a hint: implement the `Hint` interface in `internal/hint/`, register it in `hint.All` (`ForEvent` filters that list by event), and add a `[hints.<id>]` toggle to the consuming repo's config overlay. A hint returns `*Advice` or nil — there is no way for it to deny.

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
belt hint prompt         # UserPromptSubmit entrypoint: advice as plain stdout text, not JSON
belt check bash "git push origin main"                # dry-run, one verdict line per guard
belt check write --file <path> --content "text"
belt doctor              # build metadata + resolved config: surfaces loaded, guard/hint state, custom guards + reachability, overrides, kof reachability, blocked names
belt override            # list overrides with state (remaining time / expired / legacy / malformed); set <name> [--for 10m], extend, clear (a commit_guards rule naming one stops applying while active)
belt config              # config file locations + every setting in effect, incl. resolved fallbacks and claude deny patterns (annotated reference in --help)
belt docs                # print the embedded operating doc: execution model, failure modes, first moves
belt version [-o json]   # bare version token, or the four-key build metadata object
```

## Gotchas

- **The bash-command parser is token-based, not a shell parser.** It splits on `&&`/`||`/`;`/`|`/newlines and matches bare `git … push` sequences. Pushes buried in quoted strings or subshell tricks aren't caught; belt is a guardrail against habit, not an adversary-proof sandbox.
- **Same for `script-deny-list`.** `curl | bash`, list-form `subprocess.run`, quoted tokens, line continuations, and same-command write-then-run are covered now, but variable indirection (`k=kubectl; $k delete`), command substitution, shell function bodies, and a written file made executable with `chmod +x` and no shebang still slip through. Flag reordering (`rm -r -f`) is an `extra_patterns` job — use a `re:` pattern rather than enumerating literal variants.
- **Runtime debugging lives in `operating.md`** (embedded in the binary, printed by `belt docs`): the no-daemon execution model and changes-apply-immediately behavior, the config fallback and fail-closed/fail-open matrix, deny false-positive paths, version-skew checks. Keep those facts there, not here.
- **`prefer-csl` does not reuse `guard.splitSegments`.** That splitter preserves byte offsets and treats `|` exactly like `&&`, which is right for finding a denied command anywhere in a pipeline and wrong for this hint: telling a pipe filter apart from a standalone search is its entire precision requirement. The hint has its own quote-aware splitter so a `|` inside a grep pattern does not read as a pipe.
- **kof subjects are shallower than search hits.** Assertions get labelled at the component level (`.../services/csl`) while searches return hits deeper (`.../services/csl/internal/semantic`), and kof matches subjects by prefix — so querying the hit's own subject finds nothing. `kof-assertions` queries the repo and narrows by ranking on shared path segments. Changing that to a narrower query silently returns zero results rather than erroring.
- **Version probe convention.** `belt version -o json` returns the shared four-key build metadata object (`version`, `commit`, `tag`, `build_time`, every key present and `""` when unknown) from `github.com/mad01/thismoon/buildinfo`, injected by `buildinfo.mk` at link time. Plain `belt version` stays a bare token — ralph and status parse it as one. `belt doctor` opens with the same metadata, so a doctor report always names the binary that produced it.
- **Keep literal internal hostnames and org names out of belt's own source and docs.** This repo is heading public and the suspenders pre-commit guard blocks them. The public/internal split is derived (`github.com` in the remote means public), never enumerated.

## See also

- User-facing hook reference: `docs/hooks.md` (per-hook decision sequences, fail modes, the settings.json wiring block, and the silent-hook debugging order — keep it in sync when adding or changing a guard or hint)
- Recipe: `recipes/belt/recipe.toml` (this repo: build/install only, wave 0, builds before the consuming repo's claude recipe registers the hooks)
- Config overlay + hook registration: the consuming repo's companion recipe carries `~/.config/belt/config.yaml` (with its internal-name `exclude_paths`) and the `hooks.PreToolUse` settings entries (machine-private wiring, see docs/adr/0006)
- Pairs with `suspenders` (git pre-commit guard layer; standalone configs by design, same name derivation — docs/adr/0010)
