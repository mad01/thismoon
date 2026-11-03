# clipboard configuration

## Where config lives

clipboard has no configuration file. Every command reads its input from an
argument or stdin and nothing else; there is no environment variable it
checks and no on-disk settings to override.

## Flags

- `--version` (bool, default `false`): print the build version and exit,
  the standard cobra root flag.

Each subcommand (`copy`, `paste`, `mcp`, `version`, `docs`) takes only
positional arguments or stdin, documented in the
[README](README.md#usage) and [CLAUDE.md](CLAUDE.md#commands). The one
subcommand flag is `version -o json`, which prints the build metadata
object instead of the bare version token.

## Environment variables

None. clipboard execs `/usr/bin/pbcopy` and `/usr/bin/pbpaste` by absolute
path specifically so it never depends on `PATH` or any other environment
state.
