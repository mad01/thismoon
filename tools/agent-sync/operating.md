# operating agent-sync

{{.Bin}} brings coding agent config from a data repo onto this machine.
The config has one section per kind of data. `{{.Bin}} settings` merges a
template settings.json into Claude Code's settings, `{{.Bin}} mods`
reconciles Claude Code mods through `claude plugin`, and `{{.Bin}} mcp`
registers MCP servers through `claude mcp`.

## how it runs

There is no serve process and no store. Each run reads the config, works
out the change, and exits. Every section has the same three modes:

- `sync` applies the change.
- `plan` prints what sync would change and writes nothing.
- `check` writes nothing and exits 1 when sync would change anything. Its
  first stdout line is a one-line summary, the line `ralph doctor` shows.

Exit codes: 0 ok, 1 drift or an error, 2 a failed `claude` command. An
error prints once on stderr as `error: ...`, a warning as `warning: ...`.

## config

The config file is `~/.config/agent-sync/config.yaml`, or the same path under
`$XDG_CONFIG_HOME`. `--config` or `$AGENT_SYNC_CONFIG` moves it. Paths must
be absolute or start with `~/`. A key the tool does not know is an error.

```yaml
settings:
  template: ~/code/dotfiles/claude/settings.json  # required
  prune:
    - [includeCoAuthoredBy]   # retired: superseded by attribution
mods:
  file: ~/code/dotfiles/claude/mods.json          # required
  # dropins: ~/.config/claude-mods/mods.d (default)
mcp:
  servers: ~/code/dotfiles/claude/servers.json    # required
  # dropins: ~/.config/claude-mcp/servers.d (default)
  prune: false
```

## settings

The template wins over live: objects merge key by key, arrays and scalars
are replaced, and keys only live has are kept. Each `prune` entry is a key
path deleted from live before the merge. The overlay merges last; its
permission lists add to the template's. Sync writes a temp file beside the
live file and renames it over, so a failed write leaves the old file in
place. A symlinked live file becomes a regular file; a new one is 0600.

## mods

The mods file, then each drop-in in name order, names one directory
marketplace and its mods. Sync runs `claude plugin ... --scope user` for
the difference and never edits Claude Code's plugin files itself. An
enabled mod whose install record version differs from its folder version
is updated. `"installed": false` uninstalls a mod, even one the marketplace
no longer lists. A mod gated off this machine by `hosts` or `profiles` is
left alone. `$CLAUDE_CONFIG_DIR` moves the plugin state it reads.

## mcp

The servers file, with each drop-in merged over it by name, is the desired
set; `hosts` and `profiles` gate entries to machines. Missing servers are
added at user scope, and a server whose `claude mcp get` spec differs is
removed and added again. Remote servers compare by url only. With `prune`,
registered servers outside the set go, in any scope. A stamp of the
desired set lets sync and check skip the per-server get calls; plan always
makes them, and `--force` makes sync and check make them too.

## failure modes

`config: file not found`: no config at the path shown. Write one, or point
`--config` at it.

`section missing`: the command's section is not in the config. Add it, or
stop calling the command.

`settings.prune retires keys the template still sets`: a key is in both the
template and `prune`. Drop it from one of them. Nothing was written.

`warning: skipping .../settings.local.json`: the overlay does not parse.
Sync carries on without it and doctor fails on it.

`live settings: open .../.settings.json.tmp-...: permission denied`: the
live file's directory is not writable. Doctor's `settings-live-writable`
check names it.

A permission a session added is gone after a settings sync: the template's
lists replace live's. Put machine-only permissions in the overlay.

`` `claude` CLI not found on PATH ``: mods needs claude only to sync; plan
prints the commands anyway. mcp needs it in every mode, to list what is
registered. Exit 1.

mods `... failed: <detail>`, exit 2: a `claude plugin` call failed and sync
stopped there. The calls before it are applied; run sync again after the
fix.

`marketplace '<name>' is not on disk`: ralph has not pulled that source
yet. The file is skipped with this note, which is not an error.

`warn: <id> is not in the marketplace listing yet`: the mods file enables a
mod its marketplace does not list. Merge the marketplace change first.

`sets 'installed' false and 'enabled' true`: a retired mod cannot be
enabled. Drop `enabled`.

`is already declared by`: a mod or marketplace name appears in two files.
To move one between files, drop it from the old file, sync, then add it to
the new one.

mcp `` `claude mcp list` failed ``, `add 'x' failed` or `remove 'x'
failed`, exit 2 with claude's stderr: the servers before it are already
changed and the stamp is not written. An http or sse server with
`headers` and no `headersHelper` always fails to add: the flags go where
claude rejects them, a script bug kept for parity. Register that one by
hand.

`has no 'url'` or `has no 'command'`, exit 1: the entry cannot be added. On
an update the remove has already run, so the server is gone until the
entry is fixed.

`<file>: server 'x': <field>: want ...`: a servers.json field has the wrong
type. Nothing was changed.

`warning: could not write sync stamp`: exit 0, but every run checks every
spec until the stamp path is writable.

mcp plan shows `~ <name>` on every run for a server with env, or with an
argument that holds a space: claude prints those differently from
servers.json. This matches the script and is harmless; the re-add gives
the same server.

## first moves

1. `{{.Bin}} doctor` (config valid, data readable, live writable, claude on PATH)
2. `{{.Bin}} <section> check`, then `{{.Bin}} <section> plan` to see the change
3. `{{.Bin}} version -o json` to see which build is installed
