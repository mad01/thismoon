# Suspenders

Offline git secret scanning and hook orchestration. Detects checked-in secrets, manages pre-commit and post-merge hooks across repositories, and guards against leaking internal repository names into public repos.

## Language

**Allowlist**:
Exact-match secret *values* known to be safe (e.g. a test fixture token), optionally restricted to path globs. Exists at global config scope and per-repo scope; the scanner treats both the same at runtime.
_Avoid_: safe references, whitelist

**Safe references**:
Org/repo names the guard permits in staged diffs of public repos (e.g. `grpc/grpc-go`). Lives under `guard.allowlist` in config YAML for compatibility, but in prose and code discussion it is "safe references", never "allowlist".
_Avoid_: guard allowlist (in prose)

**Blocked name**:
A term the guard rejects in public repos — in staged diffs at pre-commit, in tracked files during `scan`, and in added lines and messages during history scan/clean. Collected from workspace-dir repo names (org/repo and directory basename) plus `blocked_words` (brand names, internal domains, doc links; `*` wildcards allowed), minus safe references. Repo excludes do not remove blocked names — that is deliberate.
_Avoid_: banned word, internal name (as the mechanism's name)

**Repo exclude**:
Glob patterns for repositories that hook management (`install/uninstall --all`) skips. Does NOT affect guard name collection — excluded repos in workspace dirs still contribute blocked names. Top-level `exclude` in config.
_Avoid_: allowlist, ignore

**Ignore**:
The umbrella verb for making a finding not appear without disabling the rule globally. Mechanisms: per-repo rule ignore, path ignore, pattern ignore, allowlist entry, and the inline `suspenders:ignore` marker.
_Avoid_: suppress, mute, skip (as the mechanism's name)

**Rule exclusion**:
Global removal of a rule via `scan.exclude_rules` — the rule never runs, in any repo. Distinct from a rule ignore.
_Avoid_: global ignore, disable

**Rule ignore**:
Per-repo entry in `.suspenders.yaml` `rules:` — the rule still runs, but its findings are dropped in that repo.
_Avoid_: rule exclusion (per-repo), suppression

**External hook**:
A user-configured shell command in `hooks.pre_commit` / `hooks.post_merge`, optionally filtered by file patterns and repo globs. Runs as part of a hook run, before the built-in checks.
_Avoid_: custom hook, script hook

**Built-in checks**:
The guard and the scan, collectively — the checks suspenders itself performs during a hook run, after external hooks.
_Avoid_: internal hooks

**Hook run**:
One execution of the full pipeline for a git event (`suspenders hook run <event>`): external hooks in config order, then guard, then scan.
_Avoid_: hook execution, pipeline (as the term)

**Ignore file**:
The per-repo `.suspenders.yaml` holding rule/path/pattern ignores, a repo-scoped allowlist, and repo-scoped watch rules.
_Avoid_: override file, repo config
