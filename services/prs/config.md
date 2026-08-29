# prs configuration

## Where config lives

prs resolves its settings from persistent CLI flags on the `prs` command,
each with an environment variable fallback and a compiled-in default
(precedence: flag over environment variable over default), plus one YAML
config file for the polling scope. The flags are persistent, so they apply
to every subcommand, though only `serve` reads the workdir and config for
anything beyond talking to a running `prs serve`.

## Flags

- `--workdir` (string, default `~/.local/share/prs`, env `PRS_WORKDIR`):
  directory holding the PR cache (`repos.jsonl`). Only `prs serve` writes
  it. A leading `~` expands at startup.
- `--port` (int, default `7427`, env `PRS_PORT`): the port `prs serve`
  listens on, and the port every other command connects to as an HTTP
  client. `serve` binds to `127.0.0.1` only: the PR list is personal and
  the API has no authentication, so it must never be reachable off
  localhost.
- `--base-url` (string, default `http://localhost:<port>`, env
  `PRS_BASE_URL`): the human-facing URL the MCP server returns in tool
  responses (for example `http://prs.this`). Display-only: the MCP client
  itself always calls `localhost:<port>`.
- `--config` (string, default `$XDG_CONFIG_HOME/prs/config.yaml` when that
  variable holds an absolute path, otherwise `~/.config/prs/config.yaml`,
  env `PRS_CONFIG`): the YAML config path.

A `~` that cannot be expanded — no resolvable home directory, which happens
under a launchd agent with a stripped environment — is an error at startup.
Earlier releases stripped the `~` and read a path relative to the working
directory instead, which silently found no config. An unparseable `PRS_PORT`
warns once on stderr and falls back to the compiled-in default.

To see what a given environment actually resolved to, run `prs doctor`; an
agent with no shell gets the same report from the `prs_doctor` MCP tool.

## Config file

`~/.config/prs/config.yaml` (or the XDG path above). A missing file is not
an error — serve runs
with defaults and polls nothing until `dirs` is set. An invalid file
(unparseable YAML, bad glob, bad interval) fails at startup rather than
being silently ignored.

```yaml
# Directories walked for git repos (the polling scope). ~ expands.
dirs:
  - ~/code/src

# org/repo globs to skip. Matched against the origin remote's org/repo.
exclude:
  - someorg/noisy-*

# org/repo globs that override every exclusion, built-in defaults included.
# The live-test fixture repo mad01/prs-testbed is excluded by default;
# a test machine opts back in here.
include: []

# Optional git-host allowlist. Omit (or leave empty) to poll every host
# discovered from the checkouts' origin remotes.
hosts:
  - github.com

# How often the poller refreshes. Go duration syntax; default 5m.
poll_interval: 5m
```

No tokens live here. Auth comes from the gh CLI's existing logins:
`gh auth token --hostname <host>` is exec'd once per host and the token is
cached in memory. Add a host with `gh auth login --hostname <host>` as the
user serve runs as.

## Example

A typical supervised launch:

```bash
prs serve --port 7427 --workdir ~/.local/share/prs --base-url http://prs.this
```

The machine's real config file is machine-private wiring and lives in the
consuming repo's overlay (docs/adr/0006), delivered next to the recipe.
