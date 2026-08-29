# present configuration

present has no config file. Everything is set with flags or environment
variables, resolved once at process start by `present serve` and
`present mcp` independently.

## Where config lives

There is no config file to locate or parse. `present serve` and
`present mcp` each resolve `--workdir`/`PRESENT_WORKDIR` and
`--port`/`PRESENT_PORT` on their own; because they must agree to work
together (the MCP tools write pages into the workdir, and the URLs they
return point at the port `serve` listens on), a divergence between the two
processes is a configuration error to check for, not a bug. Both processes
log their resolved `workdir=… port=… base-url=…` to stderr at startup.

A leading `~` in `--workdir` is expanded before use. Flags take precedence
over environment variables, which take precedence over the built-in
defaults. A `~` that cannot be expanded — no resolvable home directory,
which happens under a launchd agent with a stripped environment — is an
error at startup. Earlier releases stripped the `~` and used a path relative
to the working directory instead, which quietly served an empty store.

## Flags

These are persistent flags shared by `present serve` and `present mcp`:

- `--workdir` (string; env `PRESENT_WORKDIR`): directory holding `pages/`,
  the page store both processes read and write. The default is
  `~/.config/present` while that directory exists, otherwise
  `$XDG_STATE_HOME/present` (or `~/.local/state/present` when that variable
  is unset or not absolute).
- `--port` (int, default `7423`; env `PRESENT_PORT`): port `present serve`
  listens on. Also used to compute the default `--base-url`, and by
  `present mcp` to build the page URLs it returns.
- `--base-url` (string, default `http://localhost:<port>`; env
  `PRESENT_BASE_URL`): base URL used when building page links, e.g. in
  `present_create`'s returned URL. Only needs overriding when pages are
  reached through something other than plain `localhost:<port>` (for example
  the local domain front door's `present.this`).

## Where the page store lives

The store moved out of `~/.config` — pages are state, not configuration —
but **existing pages are never migrated**. The resolution is deliberately
sticky: a machine that already has `~/.config/present` keeps using it for as
long as that directory exists, so upgrading never orphans published pages or
breaks the URLs handed out for them. Only a machine without that directory
lands in the XDG state directory.

Nothing changes for a supervised install: the fleet recipe passes
`--workdir` to `present serve` explicitly, and the MCP's seatbelt wrapper
sets `PRESENT_WORKDIR`, so both processes agree regardless of which branch
the default would take. To move an existing store, move the directory and
set `PRESENT_WORKDIR` (or `--workdir`) to the new path on both processes;
there is no automatic migration to undo.

## Environment variables

- `PRESENT_WORKDIR`: default for `--workdir`.
- `PRESENT_PORT`: default for `--port`. An unparseable value warns once on
  stderr, and the built-in default (`7423`) is used instead.
- `PRESENT_BASE_URL`: default for `--base-url`.

To see what a given environment actually resolved to, run `present doctor`;
an agent with no shell gets the same report from the `present_doctor` MCP
tool. It checks the page store first, since the MCP tools write it directly
and keep working while `serve` is down.

## Example

```bash
present serve --workdir ~/.config/present --port 7423
present mcp --workdir ~/.config/present --port 7423
```

Equivalent via environment variables:

```bash
export PRESENT_WORKDIR=~/.config/present
export PRESENT_PORT=7423
present serve
```
