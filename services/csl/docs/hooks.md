# Hooks

> **Deprecated.** csl no longer installs or recommends `post-merge` git hooks. [suspenders](https://github.com/mad01/suspenders) is now the single git-hook manager. `csl hooks install` still works but prints a deprecation notice; `csl hooks uninstall` and `csl hooks status` remain fully functional so you can remove existing csl-managed hooks. See [Ownership](#ownership-who-writes-the-queue-vs-who-drains-it) below.

## Ownership: who writes the queue vs. who drains it

The reindex flow is split in two:

- **suspenders writes the queue.** suspenders manages each repo's `post-merge` hook. Add a `csl-reindex` entry to suspenders' `post_merge` config; on every `git pull` it appends the merged repo's root path to `~/.config/csl/reindex.queue`.
- **csl drains the queue.** csl still owns reading that queue and indexing. Run `csl sync` or `csl index --drain` to batch-index every queued repo in a single process. This contract is unchanged from the old csl-managed hooks.

Configure `csl-reindex` under suspenders' `post_merge` (see the [suspenders docs](https://github.com/mad01/suspenders)); a hook entry of that name writes:

```sh
printf '%s\n' "$(git rev-parse --show-toplevel)" >> "${HOME}/.config/csl/reindex.queue"
```

### Migrating off csl-managed hooks

1. Run `csl hooks uninstall` to remove any `post-merge` hooks csl previously wrote.
2. Add the `csl-reindex` entry to suspenders' `post_merge` config.
3. Keep running `csl sync` / `csl index --drain` to drain the queue (unchanged).

---

The rest of this page documents the **deprecated** `csl hooks install` path. It is retained only so existing managed hooks can be inspected and removed.

`csl hooks install` wrote a `post-merge` git hook into every repo `csl` discovers, so the search index was refreshed automatically after each `git pull`. The hook appended the repo path to `~/.config/csl/reindex.queue`, which `csl sync` / `csl index --drain` then drained.

## Configure

> This section applies to the deprecated `csl hooks install` path. New setups should use suspenders (see [Ownership](#ownership-who-writes-the-queue-vs-who-drains-it)).

Hooks are configured in `~/.config/csl/config.yaml` under a `hooks` block. The block is optional and defaults to disabled; if it is missing or `enabled: false`, `csl hooks install` refuses to run with `hooks.post_merge.enabled is false in config — nothing to install (csl post-merge hooks are deprecated; use suspenders)`.

### Schema

| Field | Type | Default | Description |
|---|---|---|---|
| `hooks.post_merge.enabled` | bool | `false` | Master switch for the installer. `csl hooks install` is a no-op when false. |
| `hooks.post_merge.exclude` | list of strings | `[]` | Repos to skip. Each entry matches against the repo's absolute path *or* its `org/repo` name (exact match, no globs). Tildes are expanded. |

### Example

```yaml
dirs:
  - ~/code/src/github.com
  - ~/workspace

hooks:
  post_merge:
    enabled: true
    exclude:
      - ~/workspace/large-monorepo
      - myorg/big-monorepo
```

### Exclusion semantics

Each entry in `exclude` is matched twice for every discovered repo: once against the absolute filesystem path, once against the `org/repo` name extracted from the origin remote. Either match excludes the repo. There are no globs and no regex.

Use a path entry when the org/repo name is ambiguous or when the repo has no remote. Use a name entry when the same repo could live under different parent directories on different machines.

`csl hooks status` reports the exclusion explicitly:

```
myorg/large-monorepo                   excluded     matched config exclude
```

## `csl hooks install`

> **Deprecated.** Prints a deprecation notice and points you to suspenders. Use suspenders' `post_merge` `csl-reindex` entry instead.

Install or update the `post-merge` hook in every non-excluded repo.

### Synopsis

```sh
csl hooks install
csl hooks install --dry-run
csl hooks install --repo <path>
csl hooks install --force
```

### Description

`csl hooks install` walks the `dirs` from your config, applies `hooks.post_merge.exclude`, and for every remaining repo writes `<repo>/.git/hooks/post-merge` with the script shown below in [The post-merge hook script](#the-post-merge-hook-script).

The hook file is identified as csl-managed by a marker line near the top:

```sh
# csl-managed-hook: post-merge v1
```

Re-running `install` compares the on-disk file byte-for-byte with the script that would be written. Identical files are skipped, so repeated runs cost only stat + read calls. Bumping the version number in the marker (currently `v1`) is how a future csl release would force a rewrite of older managed hooks.

**Foreign hooks.** If `<repo>/.git/hooks/post-merge` exists and does not contain the marker, csl assumes another tool wrote it (commonly `git-lfs`) and refuses to overwrite. The repo is reported as `foreign` in the run summary and in `csl hooks status`. Pass `--force` to overwrite anyway. There is no merge mode.

`install` writes the file with mode `0755`. The parent `.git/hooks/` directory is created if missing.

### Flags

| Flag | Default | Description |
|---|---|---|
| `--dry-run` | `false` | Print planned writes without touching the filesystem. |
| `--force` | `false` | Overwrite foreign (non-csl-managed) hooks. |
| `--repo <path>` | `""` | Operate on a single repo by absolute path. Skips the `dirs` walk and the `enabled` check. Useful for debugging or for a one-off install on a repo outside the configured tree. |

### Examples

**Preview before writing:**

```sh
csl hooks install --dry-run
```

Output:

```
  would write /Users/you/code/src/github.com/myorg/foo/.git/hooks/post-merge
  would write /Users/you/code/src/github.com/myorg/bar/.git/hooks/post-merge
  skip /Users/you/workspace/large-monorepo — excluded by config
  skip /Users/you/code/src/github.com/myorg/dotfiles — foreign hook present (use --force to overwrite)

2 changed, 1 excluded, 1 foreign, 0 errors
```

**Install for real:**

```sh
csl hooks install
```

**Install on one repo without touching the rest:**

```sh
csl hooks install --repo ~/code/src/github.com/myorg/foo
```

**Force-overwrite a foreign hook:**

```sh
csl hooks install --force --repo ~/code/src/github.com/myorg/dotfiles
```

`install` exits non-zero only if at least one repo failed with an I/O error. A repo being foreign or excluded is not an error.

## `csl hooks uninstall`

Remove the csl-managed `post-merge` hook from every repo.

### Synopsis

```sh
csl hooks uninstall
csl hooks uninstall --dry-run
csl hooks uninstall --repo <path>
```

### Description

`uninstall` walks the same `dirs` as `install`, but instead of writing it deletes any `post-merge` hook that contains the csl marker. Foreign hooks (no marker) are left in place and reported.

There is no `--force` for uninstall: csl will not delete a hook it didn't write.

### Flags

| Flag | Default | Description |
|---|---|---|
| `--dry-run` | `false` | Print planned removals without touching the filesystem. |
| `--repo <path>` | `""` | Operate on a single repo by absolute path. |

## `csl hooks status`

Show hook installation state for every discovered repo.

### Synopsis

```sh
csl hooks status
csl hooks status --json
```

### Description

`status` reports one row per repo. The `HOOK` column takes one of:

| Value | Meaning |
|---|---|
| `installed` | csl-managed hook present (marker found). |
| `missing` | No `post-merge` hook in `.git/hooks/`. |
| `foreign` | A `post-merge` hook exists but doesn't contain the csl marker. `install` will skip; `--force` would overwrite. |
| `excluded` | Repo matches a `hooks.post_merge.exclude` entry. |
| `error` | The hook file couldn't be read. The cause is in the `NOTE` column. |

### Example output

```
REPO                                               HOOK         NOTE
myorg/foo                                          installed
myorg/bar                                          missing
myorg/dotfiles                                     foreign
myorg/large-monorepo                             excluded     matched config exclude
```

JSON output is one object per repo with fields `repo`, `path`, `status`, `excluded`, `reason`.

## The post-merge hook script

Every csl-managed `post-merge` hook is exactly this:

```sh
#!/usr/bin/env sh
# csl-managed-hook: post-merge v2
# Edit csl config (~/.config/csl/config.yaml), not this file —
# `csl hooks install` will overwrite it.
# csl sync suppresses hooks via core.hooksPath=/dev/null, so this only
# fires on direct git pull (outside csl sync).
mkdir -p "${HOME}/.config/csl" && \
  printf '%s\n' "$(git rev-parse --show-toplevel)" >> "${HOME}/.config/csl/reindex.queue"
exit 0
```

A few notes on the choices:

- The hook only enqueues; it does not index inline. A later `csl sync` or `csl index --drain` drains `~/.config/csl/reindex.queue` and batch-indexes every queued repo in one process, avoiding concurrent `state.json` writes.
- `git rev-parse --show-toplevel` resolves the repo root from inside the hook regardless of where in the worktree the merge ran.
- The marker line is the contract. Bumping the version (`v2`) is how a csl release forces a rewrite of older managed hooks.

The suspenders-managed `csl-reindex` entry writes the same `printf … >> reindex.queue` line, so the drain side is identical no matter which tool owns the hook.

## Single-repo reindex (`csl index --repo`)

The hook calls `csl index --repo <path>`, which is the only way to reindex one repo without scanning every other configured directory. It is also useful from the terminal:

```sh
csl index --repo ~/code/src/github.com/myorg/foo
```

The flag bypasses the global staleness check, calls [`finder.Inspect`](../internal/repo/finder/walker.go) to resolve the repo's name and remote, and updates the per-repo entry in `state.json` so the next `csl index` run won't redundantly re-process it. See [CLI reference](cli.md#csl-index) for the full `csl index` flag list.

## Re-applying with ralph

`install` is safe to call on every `ralph apply`. The byte-for-byte compare described in [`csl hooks install`](#csl-hooks-install) means an unchanged hook costs only a stat + read. Wire it in via the recipe's `[hooks]` block:

```toml
[hooks]
post_apply = ["csl hooks install || true"]
```

The `|| true` keeps `ralph apply` green when csl isn't installed yet (for example, on a freshly bootstrapped machine where the csl binary hasn't been built).
