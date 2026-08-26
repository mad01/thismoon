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
defaults.

## Flags

These are persistent flags shared by `present serve` and `present mcp`:

- `--workdir` (string, default `~/.config/present`; env `PRESENT_WORKDIR`):
  directory holding `pages/`, the page store both processes read and write.
- `--port` (int, default `7423`; env `PRESENT_PORT`): port `present serve`
  listens on. Also used to compute the default `--base-url`, and by
  `present mcp` to build the page URLs it returns.
- `--base-url` (string, default `http://localhost:<port>`; env
  `PRESENT_BASE_URL`): base URL used when building page links, e.g. in
  `present_create`'s returned URL. Only needs overriding when pages are
  reached through something other than plain `localhost:<port>` (for example
  the local domain front door's `present.this`).

## Environment variables

- `PRESENT_WORKDIR`: default for `--workdir`.
- `PRESENT_PORT`: default for `--port`. An unparseable value is ignored and
  the built-in default (`7423`) is used instead.
- `PRESENT_BASE_URL`: default for `--base-url`.

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
