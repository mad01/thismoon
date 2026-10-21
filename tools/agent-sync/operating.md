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
no longer lists, but only while the marketplace directory is on disk: a
file whose `.claude-plugin/marketplace.json` is missing is skipped whole,
retirements included. A mod gated off this machine by `hosts` or
`profiles` is left alone. `$CLAUDE_CONFIG_DIR` moves the plugin state it
reads.

## mcp

The servers file, with each drop-in merged over it by name, is the desired
set; `hosts` and `profiles` gate entries to machines. A server name
`claude mcp add` would reject is a config error. Missing servers are
added at user scope, and a server whose `claude mcp get` spec differs is
removed and added again. Remote servers compare by url only. claude runs
in an empty temporary directory, so list and get never show the local or
project scope servers of the directory the sync started in. A server get
still shows at another scope, such as an enterprise one, is not compared.
With `prune`, sync removes registered servers outside the set at user
scope. It skips a claude.ai connector, a plugin's server, a server in
another scope and a name `claude mcp add` would reject. Each run names the
servers it left alone in `note:` lines. A stamp of the desired set
lets sync and check skip the per-server get calls; plan always makes them,
and `--force` or `prune` makes sync and check make them too.

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

A key on the `prune` list is still in the live file after a sync: the
overlay sets it. The overlay merges after the prune and is not checked
against the list, so it puts the key back with no error and check reports
no drift. Drop the key from the overlay.

`` `claude` CLI not found on PATH ``: mods needs claude only to sync; plan
prints the commands anyway. mcp needs it in every mode, to list what is
registered. Exit 1. A claude found first through a relative PATH entry,
such as `.`, counts as missing too.

mods `... failed: <detail>`, exit 2: a `claude plugin` call failed and sync
stopped there. The calls before it are applied; run sync again after the
fix.

`marketplace '<name>' is not on disk`: ralph has not pulled that source
yet. The file is skipped with this note, which is not an error. Its
`"installed": false` entries are skipped too, so a retired mod stays
installed until the marketplace directory is back.

`warn: <id> is not in the marketplace listing yet`: the mods file enables a
mod its marketplace does not list. Merge the marketplace change first.

`sets 'installed' false and 'enabled' true`: a retired mod cannot be
enabled. Drop `enabled`.

`is already declared by`: a mod or marketplace name appears in two files.
To move one between files, drop it from the old file, sync, then add it to
the new one.

mcp `` `claude mcp list` failed ``, `add 'x' failed` or `remove 'x'
failed`, exit 2 with claude's stderr: the servers before it are already
changed and the stamp is not written.

mcp `note: prune skips servers outside user scope: ...`, with `prune:
true`: the names listed are a claude.ai connector, a plugin's server, or
a server whose `claude mcp get` shows another scope, such as an
enterprise one. A user-scope remove cannot reach them, so prune leaves
them and check does not count them. Remove one where it lives if it
should go.

mcp `note: not managed, name not valid for claude mcp add: ...`, with
`prune: true`: list shows a name with a character other than letters,
numbers, hyphens, and underscores, usually a user-scope entry edited in
by hand.
`claude mcp add` could not add it back, so prune leaves it and check does
not count it. Remove it by hand if it should go.

mcp `note: spec not compared, get shows another scope for: ...`: a
servers.json name is also defined at a scope agent-sync does not write,
such as an enterprise one. `claude mcp get` shows that copy, so the
user-scope one cannot be compared, and a server with no user-scope copy
is not added there. Both count as in sync. A local or project scope copy
never causes this note: claude runs in an empty directory, where neither
exists. Drop the name from servers.json if the other copy should win.

`has no 'url'` or `has no 'command'`, exit 1: the entry cannot be added. On
an update the remove has already run, so the server is gone until the
entry is fixed.

`<file>: server 'x': <field>: want ...`: a servers.json field has the wrong
type. Nothing was changed. A `hosts` or `profiles` gate may be a list, an
object (its keys count) or a string (each character counts), as the script
read it. A number other than 0, `true`, or a hosts entry that is not a
string is this error.

`<file>: server 'x': name not valid for claude mcp add`, exit 1: a
servers.json name holds a character other than letters, numbers, hyphens,
and underscores, such as a dot. Every machine checks every name, even in
an entry gated off it. Nothing was changed and claude did not run. Rename
the server.

`warning: could not write sync stamp`: exit 0, but every run checks every
spec until the stamp path is writable.

mcp plan shows `~ <name>` on every run: `claude mcp get <name>` prints
that server differently from its servers.json entry, so every re-add gives
the same server and it still drifts. An `env` value with leading or
trailing whitespace does this, because get's lines are read stripped.
Otherwise compare the get output with the entry; a new claude release may
have changed the format. A copy at local or project scope in the
directory you ran from is not the cause: claude runs in an empty
directory, where get shows the user-scope copy.

mcp check reports in sync after an `args` edit that only moves an argument
boundary (`["--name two", "words"]` to `["--name", "two words"]`): get
prints the arguments joined by spaces, so both read the same. Run
`claude mcp remove --scope user <name>`, then sync to add it again.

## first moves

1. `{{.Bin}} doctor` (config valid, data readable, live writable, claude on PATH)
2. `{{.Bin}} <section> check`, then `{{.Bin}} <section> plan` to see the change
3. `{{.Bin}} version -o json` to see which build is installed
