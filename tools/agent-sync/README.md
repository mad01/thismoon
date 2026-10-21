# agent-sync

Sync the coding agent config a data repo carries onto this machine.

Your Claude Code settings, mods and MCP servers live in a repo. On each
machine, `agent-sync settings sync` merges the settings template into
`~/.claude/settings.json`, `agent-sync mods sync` installs, enables, updates
and retires the mods the repo lists, and `agent-sync mcp sync` registers the
MCP servers it names. Each section's `check` tells you, in one line, whether
a machine has drifted.

## How it works

The config file names the data, one section per kind:

- `settings`: a template settings.json, merged into Claude Code's live
  settings file, then the local overlay (`settings.local.json`) on top.
- `mods`: a mods file, plus the drop-in files in its `dropins` directory,
  each naming one directory marketplace and the mods to enable, disable or
  retire from it.
  agent-sync runs `claude plugin` at user scope to match.
- `mcp`: a servers.json, plus drop-in files merged over it by name, naming
  the MCP servers to register. agent-sync runs `claude mcp` at user scope
  to match.

Each section runs in one of three modes. `sync` applies the change, `plan`
prints it and writes nothing, and `check` writes nothing and exits 1 when
sync would change anything.

The settings merge follows four rules:

- The template wins. Objects merge key by key, and arrays and scalars are
  replaced by the template's.
- Keys only the live file has are kept, unless the `prune` list retires
  them.
- The overlay merges last. Its permission lists (`permissions.allow`,
  `deny`, `ask`, `additionalDirectories`) add to the template's instead of
  replacing them.
- A symlinked live file becomes a regular file.

Mods and MCP servers can be gated to some machines: a `hosts` list matches
the host name, and a `profiles` list matches the profile tags in ralph's
`config.local.toml`. mods never edits Claude Code's plugin files itself,
and mcp compares a remote server by url alone, so an OAuth login survives a
sync.

The output, exit codes and written files match the Python scripts this tool
replaces, apart from a few listed deviations. A recipe can switch to it
without changing what it checks.

## Install

```bash
make install   # builds and installs to ~/code/bin/agent-sync
```

On a ralph-managed machine the `agent-sync` recipe builds and installs it.
The recipe never runs a sync: the repo that holds your data wires that up.

## Usage

```bash
agent-sync settings plan    # show what sync would change
agent-sync settings sync    # merge and write ~/.claude/settings.json
agent-sync mods check       # exit 1 on drift; first line is a summary
agent-sync mcp sync --force # register servers, checking every spec
agent-sync doctor           # config valid, data readable, claude on PATH
```

A ralph recipe in the data repo runs `sync` on every apply and uses `check`
as its verify step, so `ralph doctor` shows the drift line.

## Configuration

`~/.config/agent-sync/config.yaml`, moved by `--config` or
`$AGENT_SYNC_CONFIG`:

```yaml
settings:
  template: ~/code/dotfiles/claude/settings.json
  prune:
    - [includeCoAuthoredBy]      # superseded by attribution
    - [hooks, Notification]      # moved to the belt hooks
mods:
  file: ~/code/dotfiles/claude/mods.json
mcp:
  servers: ~/code/dotfiles/claude/servers.json
  prune: false
```

Every other key has a default, and each section's drop-in directory need
not exist. [config.md](config.md) lists every key.

## Develop

```bash
make build    # ./agent-sync
make test     # go test ./...
```

See [`CLAUDE.md`](CLAUDE.md) for the package layout, the shared
conventions the sections follow, and each section's deviations from its
script.

## Docs

- [architecture](architecture.md): internal structure and data flow
- [operating](operating.md): runtime behavior, failure modes, first moves
- [why](why.md): why this component exists
- [config](config.md): configuration reference
- [migrating](migrating.md): cut over from sync scripts, then retire a
  settings key, a mod or an MCP server
