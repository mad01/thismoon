# suspenders architecture

## Overview

suspenders is a tool that scans git repositories for secrets and internal
references, orchestrates pre-commit and post-merge hooks, and can rewrite git
history to remove a secret that already landed. Everything runs against local
git data on the machine where the commit happens; there are no network calls
apart from a best-effort event POST to the local events service when a commit
is blocked. The boundary is git itself: suspenders reads the working tree, the
index, and the object database, and writes only hook scripts and (during a
history clean) rewritten refs.

## Structure

```
cmd/suspenders/
  main.go            thin entry point, calls commands.Execute()
  commands/          one cobra command per file: scan, hook, history, version
internal/
  config/            Config struct (YAML), loaded from the XDG path
  scanner/           84 content rules + 7 filename rules, entropy gating,
                     per-repo ignore handling, the Finding type and redaction
  guard/             internal-reference guard: derives the block list from
                     checked-out repos, checks staged diffs and tracked files
  hook/              hook script generation, checksum, install/update/uninstall
  history/           commit walker (history scan) and the fast-export stream
                     rewriter plus Clean orchestration (history clean)
  repo/              concurrent repository discovery (32 workers), remote parsing
tests/integration/   Docker-based end-to-end hook tests
```

## Data flow

Scan: `commands/scan.go` loads config, resolves the per-repo `.suspenders.yaml`,
and calls `scanner.ScanDir` over git-tracked files (from `git ls-files`) or
`scanner.ScanStaged`, which reads content from the index via `git show :<path>`
so the scan sees exactly what would be committed
(tools/suspenders/docs/adr/0001). Every line runs through all rules, then
entropy gates, allowlists, and inline `suspenders:ignore` markers; findings are
redacted before printing. When the guard is enabled, `guard.CheckDir` (or
`guard.Check` for staged) runs the same pass for blocked names.

Hook run: a generated hook script calls `suspenders hook run <event>`. For
pre-commit the pipeline is external `hooks.pre_commit` entries in config order,
then the guard, then the scan; any non-zero step blocks the commit and emits a
warn event. Post-merge runs external entries only and downgrades failures to
warnings.

History: `history.Walk` streams one `git log -p` over the whole history and
attributes added lines to their introducing commit (`history scan`). `history
clean` collects replacement strings, writes a backup bundle, then pipes
`git fast-export` through the pure stream transform in `history/rewrite.go`
into `git fast-import` (tools/suspenders/docs/adr/0002) and resets the refs.

## Storage

- `~/.config/suspenders/config.yaml` (or `$XDG_CONFIG_HOME/...`): YAML config, created with defaults on first run. Read and written once; everything else only reads it.
- `.suspenders.yaml` / `.yml` at a repo root: per-repo overrides, read only.
- `.git/hooks/pre-commit` and `.git/hooks/post-merge`: generated shell scripts carrying a SHA-256 checksum line; `hook update` compares checksums to detect stale scripts.
- `.git/hooks/<event>.backup`: a foreign hook found at install time, chained to by the generated script and restored on uninstall.
- `.git/suspenders-backup-<timestamp>.bundle`: mandatory backup written before every history clean; `git clone <bundle>` restores the original history.

The guard's block list is deliberately never persisted: it is recomputed on
every run from the repos checked out under `guard.workspace_dirs`.

## Interfaces

CLI commands: `scan [path]` (`--staged`, `--fail-on-findings`), `hook
install|update|status|uninstall [--all]`, `hook run <event>` (what the
generated scripts call), `history scan` (`--branch`), `history clean`
(`--replace`, `--replace-map`, `--redact-file`, `--dry-run`, `--yes`),
`doctor [path]` (which opens with the installed build), and `version [-o
json]` (the bare version token, or the four-key build metadata object shared
across the repo's components).

Config surfaces: the global YAML file, the per-repo override file, and the
inline `suspenders:ignore` line marker. The guard section is shaped like
belt's `internal_names` section on purpose, but neither tool reads the
other's file (docs/adr/0010). Hook scripts resolve the tool by bare name
through `$PATH`. There is no web or MCP surface.
