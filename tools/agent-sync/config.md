# agent-sync configuration

## Where config lives

agent-sync reads one YAML file. Its location resolves in order: the
`--config` flag, then `$AGENT_SYNC_CONFIG`, then `config.yaml` in
agent-sync's directory under `$XDG_CONFIG_HOME` (when that variable holds an
absolute path) or `~/.config/agent-sync/`. A home directory that cannot be
resolved is an error, never a path relative to the working directory.

The file is required: every section names data that lives elsewhere, so
there is nothing to sync without it. A section the file leaves out has
nothing to sync: its command fails with `section missing`, and doctor skips
it. Parsing is strict
and fails closed. An unknown section or key stops the command, and so does
a value of the wrong type or a relative path. The error names the file, the
line and the key. `agent-sync doctor` runs the same checks without syncing
anything.

## Keys

Paths must be absolute or start with `~/`. Keys must be strings: quote one
that YAML would read as a number or a bool. A bool must be a bare `true` or
`false`.

### `settings`

- `settings.template` (path, required): the template settings.json merged
  into the live file.
- `settings.live` (path, default `$CLAUDE_CONFIG_DIR/settings.json`, or
  `~/.claude/settings.json` when that variable is unset): the settings file
  Claude Code reads.
- `settings.overlay` (path, default `settings.local.json` beside `live`):
  the machine-local file merged on top. It need not exist.
- `settings.prune` (list of key paths, default empty): keys to delete from
  the live file before the merge. Each entry is a list of keys from the
  root, such as `[hooks, Notification]`, so a key with a dot in it stays
  one key. Give each entry a comment naming what retired it. A path the
  template still sets is an error, because the merge would add it straight
  back. The overlay is not checked: an overlay that sets a pruned path
  merges it back in after the prune, with no error, so the key stays in
  the live file.

### `mods`

- `mods.file` (path, required): the base mods file. It names one directory
  marketplace (`"marketplace": {"name", "path"}`) and its mods (`"mods":
  {name: {"enabled", "installed", "hosts", "profiles"}}`).
- `mods.dropins` (path, default `~/.config/claude-mods/mods.d`): a
  directory whose `*.json` files have the same shape and are read after
  `file`, in name order. Each names its own marketplace. It need not exist.
- `mods.profiles_file` (path, default `~/.config/ralph/config.local.toml`):
  the TOML file whose top-level `profiles` list holds this machine's
  profile tags, for the `profiles` gates. It need not exist.

### `mcp`

- `mcp.servers` (path, required): the base servers.json,
  `{"servers": {name: spec}}`. A spec's `hosts` and `profiles` lists gate
  it to machines, as in the mods file; each `hosts` entry must be a
  string.
- `mcp.dropins` (path, default `~/.config/claude-mcp/servers.d`): a
  directory whose `*.json` files have the same shape and are merged over
  `servers` by name, in name order. It need not exist.
- `mcp.profiles_file` (path, default `~/.config/ralph/config.local.toml`):
  the TOML file whose top-level `profiles` list holds this machine's
  profile tags, for the `profiles` gates. It need not exist.
- `mcp.stamp` (path, default `~/.cache/claude-mcp/sync.hash`): where sync
  records a hash of the desired set. While it matches, sync and check skip
  the per-server spec check.
- `mcp.prune` (bool, default `false`): remove registered servers the
  desired set does not name, including servers added by hand. It removes
  at user scope only. Some names it can't remove there: a claude.ai
  connector, a server a plugin provides, or a server in another scope.
  Each fails every sync with exit 2 and keeps check at exit 1, until you
  remove the server where it lives or turn prune off. It also makes every
  sync and check compare each server's spec, as `--force` does.

## Environment variables

- `AGENT_SYNC_CONFIG` (default: the XDG path above): the config file
  location. `--config` overrides it.
- `XDG_CONFIG_HOME` (default `~/.config`): the root agent-sync's config
  directory sits under. Ignored when it is not an absolute path.
- `CLAUDE_CONFIG_DIR` (default `~/.claude`): Claude Code's config
  directory. It holds the default live and overlay settings files, which
  need it absolute, and the plugin state mods reads, where a leading `~`
  is expanded.
- `PATH`: mods and mcp run the first `claude` CLI found on it. One found
  through a relative entry (`.`, or an empty entry) counts as missing.

## Example

```yaml
# ~/.config/agent-sync/config.yaml
settings:
  template: ~/code/dotfiles/claude/settings.json
  # live: ~/.claude/settings.json
  # overlay: ~/.claude/settings.local.json
  prune:
    - [includeCoAuthoredBy]            # superseded by attribution
    - [hooks, Notification]            # moved to the belt hooks
    - [modelSettings, "us.old-model"]  # model retired
mods:
  file: ~/code/dotfiles/claude/mods.json
  # dropins: ~/.config/claude-mods/mods.d
  # profiles_file: ~/.config/ralph/config.local.toml
mcp:
  servers: ~/code/dotfiles/claude/servers.json
  # dropins: ~/.config/claude-mcp/servers.d
  # profiles_file: ~/.config/ralph/config.local.toml
  # stamp: ~/.cache/claude-mcp/sync.hash
  prune: false
```
