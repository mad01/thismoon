# speak configuration

speak has no config file. Everything is set with flags or environment
variables, resolved once at process start.

## Where config lives

There is no config file to locate or parse. The settings below are root
flags: `speak serve`, `speak mcp`, and `speak doctor` all read the same
three, so the web surface, the playback tools, and the diagnostics resolve
to the same engine, port, and state directory.

Flags take precedence over environment variables, which take precedence over
the built-in defaults. Both long-running processes log their resolved values
to stderr at startup.

## Flags

Every flag is a persistent flag on the root command and applies to all
subcommands.

- `--port` (int, default `7425`; env `SPEAK_PORT`): port `speak serve`
  listens on, and the port `speak doctor` and the `speak_doctor` MCP tool
  probe for the web surface.
- `--tts-url` (string, default `http://127.0.0.1:8765`; env `SPEAK_TTS_URL`):
  base URL of the local Kokoro TTS engine (mlx-audio). `speak serve` proxies
  `/v1/audio/speech` to it; `speak mcp` synthesizes against it directly and
  does not require `speak serve` to be running.
- `--state-dir` (string; env `SPEAK_STATE_DIR`): directory holding playback
  state: per-sentence WAV files under `audio/` and the cross-process
  `playback.lock`. Only `speak mcp` writes there; `speak serve` holds no
  playback state. See the default below.

### Where `--state-dir` defaults to

1. `SPEAK_STATE_DIR`, when set.
2. `~/.local/share/speak`, when that directory already exists, so an install
   that has playback state keeps using it across upgrades.
3. `$XDG_STATE_HOME/speak` when `XDG_STATE_HOME` holds an absolute path,
   otherwise `~/.local/state/speak`. This is where a fresh install lands.

A leading `~` is expanded before any subcommand runs, so a value that
reaches the process unexpanded (from `SPEAK_STATE_DIR` under launchd, say)
still names the right directory. A home directory that cannot be resolved is
an error, never a directory relative to the working directory.

## Environment variables

- `SPEAK_PORT`: default for `--port`. An unparseable value warns on stderr
  and the built-in default (`7425`) is used instead.
- `SPEAK_TTS_URL`: default for `--tts-url`.
- `SPEAK_STATE_DIR`: default for `--state-dir`.
- `EVENTS_BASE_URL`: where TTS proxy failures are archived (default
  `http://127.0.0.1:7430`). Emitting is best effort: an unreachable events
  service drops the event and never affects a response.

## Cross-origin access

`speak serve` answers cross-origin requests only from this machine's own
pages, so the local TTS engine is not reachable from an arbitrary page the
browser happens to have open. An `Origin` header is allowed when it is an
`http`/`https` URL whose host is loopback (`localhost`, `127.0.0.0/8`,
`::1`, on any port) or ends in `.this`. An allowed origin is reflected back
in `Access-Control-Allow-Origin` alongside `Vary: Origin`; any other origin
gets no CORS headers at all, including on the `OPTIONS` preflight. Requests
with no `Origin` header (same-origin fetches, curl) are unaffected.

The allowlist is fixed: no flag or environment variable widens it.

## Example

```bash
speak serve --port 7425 --tts-url http://127.0.0.1:8765
speak mcp --tts-url http://127.0.0.1:8765
```

Equivalent via environment variables:

```bash
export SPEAK_PORT=7425
export SPEAK_TTS_URL=http://127.0.0.1:8765
speak serve
```
