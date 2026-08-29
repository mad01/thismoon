# events configuration

## Where config lives

There is no config file. events is configured entirely by command-line flags,
each with an environment variable fallback for the same value. Flag beats
environment variable when both are set.

## Flags

Persistent flags, accepted by every subcommand:

- `--workdir` (string, default `~/.local/share/events`): directory holding
  the `sources/` JSONL store. A leading `~` is expanded at startup. Falls
  back to `EVENTS_WORKDIR` when unset.
- `--port` (int, default `7430`): port `events serve` listens on, and the
  port the CLI and MCP server talk to. Falls back to `EVENTS_PORT` when
  unset.
- `--base-url` (string, default empty): base URL the MCP tools link to in
  their responses. When empty, it resolves at runtime to
  `http://localhost:<port>`. Falls back to `EVENTS_BASE_URL` when unset.

Flags accepted only by `events serve`:

- `--per-source-cap` (int, default `500`): newest events kept in memory (and
  on disk before compaction) per source.
- `--global-cap` (int, default `1500`): max events returned across all
  sources in a single query.

## Environment variables

- `EVENTS_WORKDIR`: fallback for `--workdir`.
- `EVENTS_PORT`: fallback for `--port`. A value that does not parse as an
  integer warns once on stderr and falls back to the compiled-in default.
- `EVENTS_BASE_URL`: fallback for `--base-url`.

A `~` that cannot be expanded — no resolvable home directory, which happens
under a launchd agent with a stripped environment — is an error at startup.
Earlier releases stripped the `~` and used a path relative to the working
directory instead, which quietly served an empty store.

To see what a given environment actually resolved to, run `events doctor`; an
agent with no shell gets the same report from the `events_doctor` MCP tool.

## Example

Run `events serve` on a non-default port with a scratch store, then point the
CLI at the same port:

```bash
EVENTS_PORT=7999 EVENTS_WORKDIR=~/tmp/events-scratch events serve

events --port 7999 list --source deps
```
