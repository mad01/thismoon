# wire configuration

## Where config lives

wire has no config file. Configuration is four persistent command-line
flags, shared across `wire serve`, `wire mcp`, and every CLI command, each
with an environment-variable fallback. Precedence is flag, then environment
variable, then built-in default.

## Flags

These are persistent flags on the `wire` root command, so they apply to
`serve`, `mcp`, and every management subcommand (`open`, `join`, `leave`,
`post`, `read`, `follow`, `close`, `list`, `connect`).

- `--workdir` (string, default `~/.local/share/wire`, env `WIRE_WORKDIR`):
  directory holding `channels.jsonl` and `messages/<channel-id>.jsonl`. Only
  `wire serve` writes here; the CLI and MCP read it indirectly through
  serve's HTTP API.
- `--port` (int, default `7432`, env `WIRE_PORT`): port `wire serve` listens
  on, and the port the CLI and `wire mcp` connect to as clients. serve binds
  loopback only (`127.0.0.1:<port>`): nothing authenticates requests, so it
  must stay unreachable from outside the machine.
- `--base-url` (string, default `http://localhost:<port>`, env
  `WIRE_BASE_URL`): the human-facing URL the MCP tools return alongside a
  channel's connection string (e.g. `http://wire.this/?channel=<name>`).
  Purely cosmetic: it doesn't change what host or port the client actually
  connects to, which is always `--port`.
- `--from` (string, default `WIRE_FROM` env var, else the OS username): the
  name CLI-posted messages are signed with. The username default is
  deliberate: at the CLI the sender is usually the human at the terminal, and
  the OS username names them distinctively. The MCP tools take `from` as an
  explicit call argument instead and require a short distinctive agent name;
  an agent driving the CLI should hold itself to the same rule and pass
  `--from` explicitly.

A leading `~` in `--workdir` is expanded at runtime, since `WIRE_WORKDIR`
reaches Go without shell expansion. A `~` that cannot be expanded — no
resolvable home directory, which happens under a launchd agent with a
stripped environment — is an error at startup. Earlier releases stripped the
`~` and used a path relative to the working directory instead, which quietly
served an empty store. An unparseable `WIRE_PORT` warns once on stderr and
falls back to the compiled-in default.

To see what a given environment actually resolved to, run `wire doctor`; an
agent with no shell gets the same report from the `wire_doctor` MCP tool.

## Environment variables

- `WIRE_WORKDIR`: fallback for `--workdir`.
- `WIRE_PORT`: fallback for `--port`.
- `WIRE_BASE_URL`: fallback for `--base-url`.
- `WIRE_FROM`: fallback for `--from`.

## Example

```bash
export WIRE_PORT=7432
export WIRE_WORKDIR=~/.local/share/wire
export WIRE_BASE_URL=http://wire.this
export WIRE_FROM=planner

wire serve --port "$WIRE_PORT" --workdir "$WIRE_WORKDIR"
```

Everything else, including which channels exist, who's on the roster, and
what's been said, lives in the append-only JSONL store under `--workdir`,
not in any config.
