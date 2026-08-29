# suspenders, offline git secret scanner and hook orchestrator

A Go CLI tool for offline git secret scanning and hook orchestration. Detects checked-in secrets using 84 built-in content rules plus 7 filename rules, manages pre-commit and post-merge hooks across repositories, and guards against leaking internal repository names into public repos. `suspenders history scan` runs the same checks over every commit on the public branch (attributed to the introducing commit); `suspenders history clean` rewrites history to remove flagged strings via a native fast-export/fast-import pipeline (see docs/adr/0002). Strings can be blanket-replaced with `***REDACTED***` or mapped to specific replacements via a replace table (`--replace-map` / config `history.replace_table`); table entries are processed longest-key-first to avoid partial matches. Files that are secrets wholesale (key/token files) are redacted in place via `--redact-file` globs / config `history.redact_files`: the blob content becomes `***REDACTED***` in every commit while the path stays in the tree; auto-collected file-name findings feed this list.

## Module layout

```
cmd/suspenders/
  main.go                    Thin entry point, calls commands.Execute()
  commands/
    root.go                  Cobra root command
    scan.go                  suspenders scan: standalone secret scanning + blocked-name
                             check (guard.CheckDir, or guard.Check for --staged)
    hook.go                  suspenders hook: install/update/status/uninstall/run
    history.go               suspenders history: scan/clean across full git history
                             clean flags: --replace, --replace-file, --replace-map,
                             --redact-file, --dry-run, --yes
    doctor.go                suspenders doctor: installed build + guard config in effect
                             + derived blocked names for a repo (never persisted,
                             derived per run)
    configdoc.go             suspenders config: config path + the settings in
                             effect (annotated setting reference in --help)
                             suspenders config init: writes a default config
                             file — the only path that creates one
    docs.go                  suspenders docs: prints the embedded operating doc
                             (operating.md, rendered via kit/agentdoc from the
                             component root's embed.go + facts.go)
    version.go               suspenders version: bare token, or -o json for the
                             four-key build metadata object (shared buildinfo package)

internal/
  config/
    config.go                Config struct (YAML), Load/LoadFrom, Path/PathFor
                             (kit/confdir; --config and $SUSPENDERS_CONFIG),
                             WithDefaults, Write, ExpandPath
                             Types: ScanConfig, GuardConfig, HistoryConfig, ExternalHook, HooksConfig
  guard/
    guard.go                 Internal-reference guard: CollectNames, Check (staged diff),
                             CheckDir (tracked working-tree files), StagedFiles
                             Walks workspace dirs via kit/repofind, regex match on content
  scanner/
    rules.go                 84 content rules (DefaultRules) + 7 filename rules (DefaultFileRules)
                             Rule fields: MinEntropy, ExcludeFiles, SkipOverlapping, Filter
    scanner.go                New, ScanDir, ScanStaged (reads index via git show :<path>),
                             AddAllowance; inline `suspenders:ignore` marker; fail-closed errors
    entropy.go                shannonEntropy: entropy gating for generic rules
    ignore.go                 Per-repo .suspenders.yaml suppression
    result.go                 Finding type, Redact function, ErrFindingsFound
  history/
    walk.go                   Commit walker: streams one `git log -p` over the whole
                             history, attributes added lines/new files to their commit
    rewrite.go                 Rewriter: pure fast-export stream transform, string
                             replacement in blobs+messages (blanket or per-string via
                             ReplaceTable), whole-file redaction (RedactPaths globs),
                             sig strip
    clean.go                   Clean/DryRun: preconditions, backup bundle,
                             fast-export | Transform | fast-import, reset
  hook/
    hook.go                    Manager type: HookScript, Checksum, Install, Uninstall,
                             Update, IsInstalled, NeedsUpdate, Status
                             Event type: PreCommit, PostMerge

Makefile                     part of module github.com/mad01/thismoon (no own go.mod)
```

Repository discovery and remote parsing live in the shared
`github.com/mad01/thismoon/kit/repofind` package (Find with 32 concurrent
workers, IsRepo, InsideWorkTree, ParseRemote) — shared with belt so the
pre-commit guard and the write-time firewall derive the same names. Blocked-
commit events go through `github.com/mad01/thismoon/kit/notify`
(`EmitEventSync`; the hook process exits immediately after).

## How it works

### Hook execution order

When git triggers a hook, the generated script calls `suspenders hook run <event>`. The execution order for each event:

**pre-commit:**

1. External `hooks.pre_commit` entries (in config order, stop on first failure)
2. Built-in guard (if `guard.enabled` is true)
3. Built-in scan (if `scan.enabled` is true, which is the default)

**post-merge:**

1. External `hooks.post_merge` entries (in config order)

Any non-zero exit from a step blocks the git operation (for pre-commit) or logs a warning (for post-merge).

### Conventions

- Config: YAML via `gopkg.in/yaml.v3`, lives at `~/.config/suspenders/config.yaml` (XDG-aware via `kit/confdir`; `--config` / `$SUSPENDERS_CONFIG` relocate it). Loading never writes: a missing file yields the in-memory defaults, and only `suspenders config init` creates one. A present-but-unparseable file is an error every command propagates, so the pre-commit hook fails closed.
- CLI: `github.com/spf13/cobra`, each command in its own file, registered via `init()`
- Enable pattern: `*bool` field; nil means enabled (backwards compat for ScanConfig, ExternalHook)
- Hook scripts use PATH-based binary resolution (`suspenders` not absolute path)
- Hook scripts call `suspenders hook run <event>`, not `suspenders scan` directly
- Hook scripts chain to `<event>.backup` (foreign hook preserved at install) before running suspenders
- Per-repo overrides via `.suspenders.yaml` or `.suspenders.yml` (ignore rules, paths, patterns, allowlist, and a `guard` section whose allowlist/blocked_words append to the global guard config); resolved from the scan root or the enclosing git top-level (`repoConfigPath` in commands/scan.go)
- Guard allowlist filtering and name dedup are case-insensitive, matching the case-insensitive matcher
- Guard exemption: repos inside `guard.workspace_dirs` or whose org/repo name matches a top-level `exclude` glob are never guard-blocked (`guardExempt` in commands/hook.go, used by hook run, scan, and history); name collection is unaffected
- Repository discovery via the shared `kit/repofind` package, which walks dirs concurrently and extracts org/repo from remotes; each repo contributes its org and repo name as separate blocked names
- Glob matching for excludes and repo filters via `github.com/gobwas/glob`

## Configuration

```yaml
dirs: []string                 # repo discovery directories
exclude: []string              # glob patterns to exclude repos (skipped by --all discovery, exempt from the guard)

scan:
  enabled: *bool               # nil/true = enabled (default)

guard:
  enabled: bool                # false by default
  workspace_dirs: []string     # directories to scan for internal repo names
  blocked_words: []string      # always-blocked terms; literal match, * = any non-space run
  allowlist: []string          # org/repo names safe to reference
  file_patterns: []string      # file globs to check in staged diff

watch: []WatchRule             # custom detection rules
allowlist: []AllowEntry        # global known-safe exact matches

history:
  replace_table:               # old -> new string mappings for history clean
    old.internal.net: new.example.com
    oldBrand: newBrand
  redact_files:                # path globs whose blob content is fully redacted
    - id_rsa                   # bare names match in any directory
    - "certs/*.p12"

hooks:
  pre_commit:                  # external pre-commit hook scripts
    - name: string
      command: string          # shell command via sh -c
      file_patterns: []string  # only run if staged files match
      repos: []string          # only run in matching repos (glob)
      enabled: *bool           # nil/true = enabled
  post_merge:                  # external post-merge hook scripts
    - (same fields as pre_commit entries)
```

### Key files

- `~/.config/suspenders/config.yaml`: user configuration
- `.suspenders.yaml` (or `.suspenders.yml`): per-repo ignore/allowlist/guard overrides, found at the repo root even when scanning a subdirectory
- `.git/hooks/pre-commit`: generated hook script (calls `suspenders hook run pre-commit`)
- `.git/hooks/post-merge`: generated hook script (calls `suspenders hook run post-merge`)
- `.git/hooks/<event>.backup`: backup of pre-existing foreign hooks

## Build / install / test

```bash
make build              # build to ./suspenders
make install             # install to ~/code/bin
make test                # run unit tests
make test-integration    # run Docker-based integration tests
make lint                # run golangci-lint
make fmt                 # format source with golines & gofumpt
```

Build metadata is embedded via `-ldflags` into the shared `github.com/mad01/thismoon/buildinfo` package by `buildinfo.mk` (short HEAD commit, full commit, the newest `suspenders/v*` tag, and the build time), read by `suspenders version` and printed at the top of `suspenders doctor`.

## Commands

| Command | What it does |
|---------|---------------|
| `scan [path]` | Scan for secrets and blocked names. `--staged` (index only), `--fail-on-findings` |
| `hook install\|uninstall\|update\|status [path]` | Manage pre-commit/post-merge hooks. `--all` for every discovered repo |
| `hook run <event>` | Run the hook pipeline for one event directly (what generated hooks call) |
| `history scan` | Walk full git history for findings. `--branch`, `--fail-on-findings` |
| `history clean` | Rewrite history to remove flagged strings / redact files. `--replace`, `--replace-file`, `--replace-map`, `--redact-file`, `--dry-run`, `--yes` |
| `doctor [path]` | Explain the guard for a repo: the installed build, config in effect, per-repo overrides, exemption status, and the derived blocked-name list |
| `config` | Print the config file location and the settings in effect; `--help` carries the annotated reference of every setting |
| `config init` | Write a config file with the built-in defaults; refuses to overwrite an existing one |
| `docs` | Print the embedded operating doc: hook pipeline, config locations, failure modes when a commit is blocked, first moves |
| `version` | Print the bare version token; `-o json` prints the four-key build metadata object |

## Gotchas

- **Runtime debugging lives in `operating.md`** (embedded in the binary,
  printed by `suspenders docs`): the hook pipeline, blocked-commit failure
  modes, the overrides that exist, and version-skew checks. Keep those facts
  there, not here.
- **Version probe convention.** `suspenders version -o json` returns the shared four-key build metadata object (`version`, `commit`, `tag`, `build_time`, every key present and `""` when unknown) from `github.com/mad01/thismoon/buildinfo`. Plain `suspenders version` stays a bare token — status parses it as one; it used to print `suspenders <sha>` (two tokens), which broke that convention, so anything still parsing the old two-token line needs updating.
- **Keep the fixture-bearing paths in this component real.** The monorepo root `.suspenders.yaml` suppresses scan findings under `tools/suspenders/` (`*_test.go`, `rules.go`, `tests/integration/**`, `README.md`). These are secret-shaped fixtures by design; don't replace them with dummy values, or the tests stop exercising real detection.

## See also

- Recipe: `recipes/suspenders/recipe.toml` — public layer only (binary build/install); the config overlay and hook install/uninstall wiring stay in the consuming repo's companion recipe (docs/adr/0006).
- `README.md`: user-facing reference; install, usage, full config schema, detection rule catalog.
- `CONTEXT.md`: domain vocabulary (allowlist vs. safe references vs. blocked name; ignore vs. rule exclusion vs. rule ignore).
- `docs/working-on-it.md`: hands-on guide; build loop, add/tune a rule, debug a false positive/negative.
- `docs/history-clean.md`: step-by-step walkthrough of the history rewrite.
- `docs/adr/0001-fail-closed-staged-scanning.md`, `docs/adr/0002-native-history-rewrite.md`: design decisions.
