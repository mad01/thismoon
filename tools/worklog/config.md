# worklog configuration

## Where config lives

worklog reads an optional YAML config file at `~/.config/worklog/config.yaml`,
overridable with `$WORKLOG_CONFIG`. The file is entirely optional, and so is
every key in it: a missing file, an unreadable one, or one that fails to
parse all leave worklog running on its built-in defaults rather than failing
to start. `worklog config` prints the file path, whether it loaded, and the
settings actually in effect after defaults are applied; the command's
`--help` also carries the full annotated reference shown under Example below.

A handful of other settings are environment-variable-only, with no config
file equivalent; see Environment variables.

## Keys

### `scan`

Ticket-firewall strings for `worklog scan`, which classifies a session as
personal or internal from its working directory and keeps ticket ids from
the two worlds from co-mingling. A key set in the file replaces its default
outright; it does not extend it.

- `scan.linear_prefixes` ([]string, default `[MAD]`): `TEAM-NN` key prefixes
  routed to the personal (Linear) ticket world. Any other prefix is treated
  as internal (Jira).
- `scan.personal_path_markers` ([]string, default `[github.com/mad01/]`): a
  session working directory containing one of these strings is classified
  personal.
- `scan.internal_path_markers` ([]string, default `[/workspace/]`): a
  session working directory containing one of these strings is classified
  internal.
- `scan.checkout_roots` ([]string, default `[/code/src/]`): GOPATH-style
  checkout root fragments. The path segment immediately after a matching
  root is read as the git host; a non-`github.com` host counts as internal.
  This split is derived from the path, never enumerated as a marker list.
- `scan.repo_path_markers` ([]string, default `[/code/, /workspace/]`): a
  working directory matching none of these reports no repo at all (for
  example, a session run from a temp directory).

### `remote`

Configures a git upstream for the store. With no `remote` section, or with
`upstreams` empty, the store stays a local-only git repo: the pre-remote
default.

- `remote.push` (bool, default `true`): whether a store write auto
  commit+pushes to the resolved upstream. Only takes effect once an upstream
  resolves for the current machine; omitting the key means `true`.
- `remote.upstreams` (map of profile label to git URL, default `{}`): maps a
  machine profile label to a git remote URL. Profile labels come from
  ralph's `~/.config/ralph/config.local.toml` (see Environment variables).
  The machine's first matching profile wins, so one config file shared
  across a fleet can route each machine's store to its own private repo (for
  example, a personal machine to a personal store repo and a work machine to
  a separate one). worklog keeps `origin` pointed at the resolved URL,
  clones it when the store directory is missing, and `worklog sync`
  pulls then pushes on demand.

## Environment variables

- `WORKLOG_CONFIG` (default `~/.config/worklog/config.yaml`): overrides the
  config file path itself.
- `WORKLOG_DIR` (default `~/code/worklog`): overrides the store root, the
  directory holding one subdirectory per work item. Read directly by the
  store, not by the config file.
- `WORKLOG_RALPH_CONFIG` (default `~/.config/ralph/config.local.toml`):
  overrides the path to ralph's per-machine profile file, the source of the
  profile labels `remote.upstreams` keys against. A missing or malformed
  file yields no profiles, which resolves to no upstream. Intended for
  tests; not a setting to change in normal use.
- `CLAUDE_PROJECTS_DIR` (default `~/.claude/projects`): overrides the root
  `worklog scan` reads Claude Code session transcripts from. Intended for
  tests; not a setting to change in normal use.

## Example

```yaml
# ~/.config/worklog/config.yaml — every key optional.
# A set list REPLACES the default shown beside it; it does not extend it.

scan:
  linear_prefixes:
    - MAD
  personal_path_markers:
    - github.com/mad01/
  internal_path_markers:
    - /workspace/
  checkout_roots:
    - /code/src/
  repo_path_markers:
    - /code/
    - /workspace/

remote:
  push: true
  upstreams:
    personal: git@github.com:you/worklog-personal.git
    work: git@github.com:you/worklog-work.git
```
