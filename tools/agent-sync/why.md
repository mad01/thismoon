# why agent-sync

## The problem

A coding agent's config is spread over files that each machine
edits on its own. Claude Code writes `~/.claude/settings.json` whenever a
session changes a setting, so the file cannot be a symlink into a repo. Mods
and MCP servers are installed through CLI commands, not files. Keeping
several machines alike meant small scripts in a private dotfiles repo, one
per kind of data. Each script carried its own merge rules, drift check and
output, and the data they read sat beside code nobody else could reuse.

## Why its own tool

The merge rules are generic: a template wins over the live file, keys a
machine adds survive, retired keys are pruned, a local overlay goes on top,
and a check mode reports drift in one line. Only the data is personal. With the
code here, the dotfiles repo keeps only data and one command per kind. The
example private-config layout can then use the same tool instead of copying
the scripts.

## Why this shape

One binary, one config file, one section per kind of data, and the same
three modes for every section, so ralph runs each the same way: `sync` on
apply, `check` as verify. Each port keeps its script's stdout,
exit codes and written bytes, which needs JSON that behaves like Python's:
ordered keys, Python's equality and `json.dumps` output. Go's
`encoding/json` reorders keys and escapes differently, so the tool carries
its own small JSON package.

## Non-goals

No data ships with the tool, and its recipe never runs a sync: wiring the
data in is the consuming repo's job (docs/adr/0006). No watching for
changes, no daemon, and no merge rules beyond the ones the scripts had.
