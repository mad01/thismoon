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
3. `$XDG_CONFIG_HOME/deps/config.toml`, when that variable holds an absolute
   path
4. `~/.config/deps/config.toml` (default; the recipe symlinks it here)

A leading `~` is expanded before the file is opened, and a home directory
that cannot be resolved is an error rather than a path relative to the
working directory.

A missing file is not an error: discovery runs with no extra exclusions. A
file that is present but malformed (bad TOML, or an exclude pattern that
does not compile) **is** an error naming the key and the pattern. `deps
config` reports it as `parse error: <err>` and still prints the empty
effective config; `deps serve`'s scan cycle fails and logs it, then retries
on the usual backoff. Nothing silently scans a repo the config asked to
skip.

## Flags

- `--workdir <path>` (string, default `~/.local/share/deps`, env
  `DEPS_WORKDIR`): directory holding `scan.json`, the store `deps serve`
  reads and writes.
- `--port <int>` (int, default `7429`, env `DEPS_PORT`): port the HTTP
  server listens on (`serve`) or the client talks to (`scan`, `check`,
  `resolve`, `notify`, `mcp`).
- `--base-url <url>` (string, default empty, env `DEPS_BASE_URL`): the URL
  the MCP server hands back in tool responses as the human-facing link.
  Empty means `http://localhost:<port>`. It is display-only: it has no
  effect on which server the client actually talks to, since the HTTP client
  always targets `localhost:<port>`. Pointing `--base-url` at
  `http://deps.this` changes the link text, not the request destination.
- `--registry <path>` (string, default `~/.config/catalog/registry.yaml`,
  env `DEPS_REGISTRY`): the catalog registry listing repos to scan. The
  same file `catalog` reads; deps does not maintain its own repo list, and
  resolves the default through catalog's config directory so
  `XDG_CONFIG_HOME` moves both together.
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

A `~` that cannot be expanded, meaning no resolvable home directory, is an
error before any subcommand runs. That happens under a launchd agent with a
stripped environment. Earlier releases stripped the `~` and read a path relative
to the working directory instead, which silently found nothing. An
unparseable `DEPS_PORT` warns once on stderr and falls back to the
compiled-in default.

To see what a given environment actually resolved to, run `deps doctor`; an
agent with no shell gets the same report from the `deps_doctor` MCP tool.

## Discovery config keys

`~/.config/deps/config.toml`. Both keys are optional lists of glob patterns.
The file only trims the repo set the catalog registry produces; it never
adds to it.

- `exclude_repos` ([]string, default `[]`): skip a whole repo.
- `exclude_paths` ([]string, default `[]`): skip a directory while walking
  a repo, matched against the repo-relative path.

Git worktrees, nested checkouts, and `.git`/`vendor`/`node_modules`/
`testdata` are skipped by the walker regardless of this file. Edits take
effect on the next scan; `serve` does not need a restart.

### How a pattern is matched

A pattern is tested a path segment at a time against **every trailing run of
whole segments** in the path, shortest to longest. For the repo at
`/Users/you/code/src/github.com/mad01/thismoon` that means `thismoon`, then
`mad01/thismoon`, then `github.com/mad01/thismoon`, and so on up to the
absolute path itself. A pattern therefore names a repo by whichever of those
is most stable, without hardcoding where a machine keeps its checkouts.

The syntax is `gobwas/glob` with `/` as the separator (the same matcher
`prs` uses for its excludes): `*` matches within one segment and never
crosses a `/`, `**` crosses, `?` is one character, and `{a,b}` alternates.
A pattern that does not compile is a config error naming the key and the
pattern. It never silently matches nothing.

Two worked examples:

- `mad01/*` excludes every repo whose parent directory is `mad01`, matching
  the `mad01/thismoon` suffix. It does not reach further up the path, so
  `someone-else/thismoon` is unaffected.
- `*/archive/*` excludes a repo two segments below any `archive` directory,
  matching the `code/archive/retired-tool` suffix of
  `/Users/you/code/archive/retired-tool`.

The same rules apply to `exclude_paths`, so `third_party` skips that
directory wherever it appears in a repo and `internal/gen/*` skips each
child of any `internal/gen`.

## Example

```toml
# ~/.config/deps/config.toml
exclude_repos = ["scratch-repo", "archive-*", "mad01/*", "*/archive/*"]
exclude_paths = ["third_party", "internal/gen/*"]
```

`deps config` prints which file resolved and the effective values (empty
lists when there is no file); `deps config --help` prints this same
annotated reference from inside the binary.
