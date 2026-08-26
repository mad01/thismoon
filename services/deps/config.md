# deps configuration

deps has two separate configuration surfaces: root flags/environment
variables that locate the store, the registry, and the discovery config
file, and the discovery config file itself, which trims the repo set that
`deps serve` scans.

## Where config lives

Root-level settings resolve from persistent flags, each with a matching
environment variable, each with a built-in default. Flags win when both are
set. There is no settings file for these; they name the paths deps uses.

The discovery config is a separate file, TOML, optional:

1. `--config <path>`
2. `$DEPS_CONFIG`
3. `~/.config/deps/config.toml` (default; the recipe symlinks it here)

A leading `~` is expanded before the file is opened. A missing file is not
an error: discovery runs with no extra exclusions. A malformed file is a
`deps config` parse error; the effective config still resolves to empty
lists so the scan keeps running.

## Flags

- `--workdir <path>` (string, default `~/.local/share/deps`, env
  `DEPS_WORKDIR`): directory holding `scan.json`, the store `deps serve`
  reads and writes.
- `--port <int>` (int, default `7429`, env `DEPS_PORT`): port the HTTP
  server listens on (`serve`) or the client talks to (`scan`, `check`,
  `resolve`, `notify`, `mcp`).
- `--base-url <url>` (string, default empty, env `DEPS_BASE_URL`): the URL
  the MCP server hands back in tool responses as the human-facing link.
  Empty means `http://localhost:<port>`. It has no effect on which server
  the client actually talks to: the HTTP client always targets
  `localhost:<port>`, so pointing `--base-url` at `http://deps.this` only
  changes the link text, not the request destination.
- `--registry <path>` (string, default `~/.config/catalog/registry.yaml`,
  env `DEPS_REGISTRY`): the catalog registry listing repos to scan. The
  same file `catalog` reads; deps does not maintain its own repo list.
- `--config <path>` (string, default `~/.config/deps/config.toml`, env
  `DEPS_CONFIG`): the discovery config file, see below.
- `serve --interval <duration>` (duration, default `24h`): max age of the
  last full scan before `serve` runs another. `serve` wakes every 15
  minutes to check staleness, so a wake-from-sleep past the interval
  triggers the catch-up scan on the next heartbeat rather than waiting for
  the next tick.

## Environment variables

Each mirrors a flag above and is overridden by it when both are set:
`DEPS_WORKDIR`, `DEPS_PORT`, `DEPS_BASE_URL`, `DEPS_REGISTRY`,
`DEPS_CONFIG`.

## Discovery config keys

`~/.config/deps/config.toml`. Both keys are optional glob patterns
(`filepath.Match` syntax), matched against both the full path and the
basename. The file only trims the repo set the catalog registry produces;
it never adds to it.

- `exclude_repos` ([]string, default `[]`): skip a whole repo. Matched
  against the repo's absolute path and its basename.
- `exclude_paths` ([]string, default `[]`): skip a directory while walking
  a repo. Matched against the repo-relative path and the directory's
  basename.

Git worktrees, nested checkouts, and `.git`/`vendor`/`node_modules`/
`testdata` are skipped by the walker regardless of this file. Edits take
effect on the next scan; `serve` does not need a restart.

## Example

```toml
# ~/.config/deps/config.toml
exclude_repos = ["archive-*", "scratch-repo"]
exclude_paths = ["third_party", "internal/gen/*"]
```

`deps config` prints which file resolved and the effective values (empty
lists when there is no file); `deps config --help` prints this same
annotated reference from inside the binary.
