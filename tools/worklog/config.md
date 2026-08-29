# worklog configuration

## Where config lives

worklog reads an optional YAML config file. Its location resolves in the
usual order: the `--config` flag, then `$WORKLOG_CONFIG`, then
`config.yaml` in worklog's directory under `$XDG_CONFIG_HOME` (when that
variable holds an absolute path) or `~/.config/worklog/`. A home directory
that cannot be resolved is an error rather than a path relative to the
working directory.

The file is optional and so is every key in it. With no file at all worklog
runs on its built-in defaults. A file that **is** there and cannot be read or
parsed stops the command instead: it carries this machine's ticket-firewall
strings and the store's push remote, and dropping those silently looks
exactly like working software. `worklog config` is the one exception — it
prints the problem and the defaults, since explaining a broken config is
what it is for. It also prints the resolved file path, whether it loaded, and
the settings in effect once worklog has applied its defaults; the command's
`--help` carries the full annotated reference shown under Example below.

A handful of other settings are environment-variable-only, with no config
file equivalent; see Environment variables.

## Keys

### `scan`

Ticket-firewall strings for `worklog scan`, which classifies a session as
personal or internal from its working directory and keeps ticket ids from
the two worlds from co-mingling. A key set in the file replaces its default
outright; it does not extend it.

The three classification keys have **no built-in values**. They describe one
person's machine layout — which ticket prefixes and which directories are
personal — so a compiled-in guess would misfile another machine's sessions
rather than admit it does not know. With none of them set the firewall has
nothing to route by: every session comes back with `context: "unknown"` and
all of its ticket references surfaced together for human review, which is
the same handling a genuinely mixed session already gets. Nothing is
misfiled, and nothing is dropped.

- `scan.linear_prefixes` ([]string, no default): `TEAM-NN` key prefixes
  routed to the personal (Linear) ticket world. Any other prefix is treated
  as internal (Jira). Unset means no key is personal.
- `scan.personal_path_markers` ([]string, no default): a session working
  directory containing one of these strings is classified personal.
- `scan.internal_path_markers` ([]string, no default): a session working
  directory containing one of these strings is classified internal.
- `scan.checkout_roots` ([]string, default `[/code/src/]`): GOPATH-style
  checkout root fragments. The path segment immediately after a matching
  root is read as the git host; a non-`github.com` host counts as internal.
  This split is derived from the path, never enumerated as a marker list,
  which is why it can ship with a default when the marker lists cannot.
- `scan.repo_path_markers` ([]string, default `[/code/, /workspace/]`): a
  working directory matching none of these reports no repo at all (for
  example, a session run from a temp directory). Repo detection is not part
  of the firewall, so it works out of the box.

### `remote`

Configures a git upstream for the store. With no `remote` section, or with
`url` empty, the store stays a local-only git repo: the pre-remote default.

- `remote.url` (string, default `""`): the git remote `origin` points at.
  worklog keeps `origin` on this URL, clones it when the store directory is
  missing, and `worklog sync` pulls then pushes on demand.
- `remote.push` (bool, default `true`): whether a store write auto
  commit+pushes to the upstream. Only takes effect once `url` is set;
  omitting the key means `true`.

**Retired: `remote.upstreams`.** The upstream used to be a map from machine
profile label to URL, resolved at runtime by reading ralph's
`config.local.toml` for the machine's profiles. That read is gone.
Provisioning writes a machine's class once and nothing changes it between
runs, so picking the upstream is a provisioning concern, not
a runtime one — the same conclusion ADR-0010 reached for the guard tools.
The layer that installs this file writes the URL for the machine it installs
on. A config still carrying `upstreams` with no `url` resolves to no remote,
so worklog warns on stderr rather than going quietly local-only; replace the
key with the machine's own `url`.

## Environment variables

- `WORKLOG_CONFIG` (default: the XDG path above): the config file location.
  `--config` overrides it.
- `XDG_CONFIG_HOME` (default `~/.config`): the root worklog's config
  directory sits under. Ignored when it is not an absolute path, per the XDG
  spec.
- `WORKLOG_DIR` (default `~/code/worklog`): overrides the store root, the
  directory holding one subdirectory per work item. Read directly by the
  store, not by the config file.
- `CLAUDE_PROJECTS_DIR` (default `~/.claude/projects`): overrides the root
  `worklog scan` reads Claude Code session transcripts from. Intended for
  tests; not a setting to change in normal use.

## Example

A full config, with the classification lists filled in the way a machine's
provisioning layer would ship them:

```yaml
# ~/.config/worklog/config.yaml — every key optional.
# A set list REPLACES the default shown beside it; it does not extend it.

scan:
  linear_prefixes:
    - MAD
  personal_path_markers:
    - github.com/you/
  internal_path_markers:
    - /workspace/
  checkout_roots:
    - /code/src/
  repo_path_markers:
    - /code/
    - /workspace/

remote:
  url: git@github.com:you/worklog-private.git
  push: true
```
