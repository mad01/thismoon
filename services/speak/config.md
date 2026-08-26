# speak configuration

speak has no config file. Everything is set with flags or environment
variables, resolved once at process start.

## Where config lives

There is no config file to locate or parse. `speak serve` and `speak mcp`
each read their own flags and environment variables independently; the two
processes do not share a config source, so a mismatched `--tts-url` between
them is a configuration error to check for, not a bug.

Flags take precedence over environment variables, which take precedence over
the built-in defaults. Both processes log their resolved values to stderr at
startup.

## Flags

### `speak serve`

- `--port` (int, default `7425`; env `SPEAK_PORT`): port the HTTP server
  listens on.
- `--tts-url` (string, default `http://127.0.0.1:8765`; env `SPEAK_TTS_URL`):
  base URL of the local Kokoro TTS engine (mlx-audio) that `/v1/audio/speech`
  proxies to.

### `speak mcp`

- `--tts-url` (string, default `http://127.0.0.1:8765`; env `SPEAK_TTS_URL`):
  same meaning as above. `speak mcp` talks to the engine directly and does
  not require `speak serve` to be running.

## Environment variables

- `SPEAK_PORT`: default for `--port` on `speak serve`. An unparseable value
  is ignored and the built-in default (`7425`) is used instead.
- `SPEAK_TTS_URL`: default for `--tts-url` on both `speak serve` and
  `speak mcp`.

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
