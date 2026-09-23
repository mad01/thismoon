# speak architecture

## Overview

speak is two independent surfaces built from one component. `speak serve`, a
t-man launchd agent on port 7425 behind `http://speak.this/`, serves the
read-aloud web page and an OpenAI-style `/v1/audio/speech`; audio plays in the
browser. `speak mcp` is a stdio MCP server that plays audio on the machine's
speakers via `afplay`; it needs the provider but not serve. Both synthesize
through the TTS provider a config file selects: the local Kokoro engine
(mlx-audio on `127.0.0.1:8765`, a recipe-managed sidecar in the consuming repo,
docs/adr/0006) by default, or OpenRouter, OpenAI or a LiteLLM proxy. The
release artifact here is the Go program alone.

## Structure

```
cmd/speak/           entrypoint, delegates to internal/cli
internal/cli/        cobra: root flags (root.go), serve, mcp (mcp.go), config
                     (config.go), doctor (doctor.go), docs, version; provider.go
                     builds the provider every subcommand uses
internal/web/        server.go (mux, HTTP API), speech.go (speech handler,
                     /enginez), cors.go (the
                     cross-origin allowlist), markdown.go (goldmark render +
                     section split), assets/shell.html + assets/app.js
internal/config/     the provider config file: a block per provider, one active
internal/provider/   the active provider built from config: client and voices
internal/tts/        tts.Error (classified failure), tts.Health (ok, degraded or
                     down, with the reason), tts.Audio (WAV/MP3 normalization)
internal/ttsclient/  HTTP client for OpenAI-compatible speech endpoints
internal/playback/   afplay engine: sessions, flock, sentence split,
                     pause/resume via SIGSTOP/SIGCONT
internal/mcpserver/  go-sdk MCP server: the 8 speak_* tools over playback
```

`internal/web` mounts the in-module webkit Go package with
`webkit.Mount(mux)`; `shell.html` carries the `<wk-header>` and speak-local
styling, and the rendered document is played through webkit's
`<wk-read-aloud>` component.

## Data flow

Web path: `GET /` serves the embedded chrome-only `shell.html` and `app.js`
builds the page client-side. `POST /read` (multipart field `doc`, max 5 MB)
renders the markdown with goldmark (GFM), splits it into
`<section class="doc-section">` blocks at every h1/h2, and returns
`{name, content}` JSON; `app.js` mounts it and re-mounts `<wk-read-aloud
targets=".doc-section">`, whose play buttons fetch one WAV per sentence from
`POST /v1/audio/speech` on the same origin.

Speech path: `internal/web/speech.go` decodes the OpenAI-style request and
synthesizes through the active provider (`internal/provider`, built once at
startup from `internal/config`), which maps a voice it does not offer to its
default and calls `internal/ttsclient` against the provider's
`/v1/audio/speech`. Raw PCM answers get a WAV header (`tts.Normalize`).
`OPTIONS` preflight is answered locally. `cors.go` decides who may fetch: an
`Origin` on loopback or under `.this` is reflected back with `Vary: Origin`,
and anything else gets no CORS headers, so pages on other local origins such
as present briefings keep working while a page from the internet cannot use
the endpoint. Every outcome feeds one `tts.Health`: a failure is answered
with a JSON error body naming the reason and emits an `error` event through
`kit/notify`. `GET /enginez` reports that health, running a test synthesis
when nothing fresh is recorded; `GET /healthz` proves only that the page is
up.

MCP path: `speak_text`/`speak_file` extract plain text from the input,
split it into sentences (`playback.SplitSentences`, hand-rolled because Go
`regexp` has no lookbehind), synthesize the first sentence before replying (so
a backend that cannot speak comes back as an `UNAVAILABLE` error reply), and
start a worker goroutine that fetches each
sentence's WAV through `internal/ttsclient`, writes it under the state
directory's `audio/`, and plays it with `afplay`. Pause sends
`SIGSTOP` to the afplay child and releases the lock; resume re-acquires it and
sends `SIGCONT`; stop saves the sentence index so resume restarts the worker
from there. An `flock` on the playback lock serializes sessions across
processes — a second caller gets a `BUSY` reply.

## Storage

No documents: markdown is rendered per request and never written to disk
(recently-read docs live only in the browser's localStorage). The MCP playback
engine writes per-sentence WAV files under `<state-dir>/audio/` (reaped after
24 hours on start) and holds `<state-dir>/playback.lock` plus an `.owner`
sidecar naming the lock holder. The state directory is `--state-dir` /
`SPEAK_STATE_DIR`, defaulting to `~/.local/share/speak` where that already
exists and `~/.local/state/speak` on a fresh install. The consuming repo's recipe creates the engine's venv and model
cache (`~/.local/share/speak/venv`, `~/.cache/huggingface`); speak never
writes them.

## Interfaces

Web: `GET /` (upload page), `GET /app.js`, `POST /read`,
`POST /v1/audio/speech` (engine proxy), `GET /healthz`, `GET /enginez`,
`GET /version`, `GET /webkit/` from the webkit Go package.

CLI: `speak serve`, `speak mcp`, `speak config [active|env] [-o json]`,
`speak doctor`, `speak docs`, `speak version [-o json]`.

MCP tools: `speak_text`, `speak_file`, `speak_pause`, `speak_resume`,
`speak_stop`, `speak_voices`, `speak_status`, `speak_doctor`. Registering the
MCP server with a client is machine-private wiring and stays in the consuming
repo; `speak mcp --help` prints the snippet.

Config: `~/.config/speak/config.yaml` holds a block per provider and a
`provider:` line picking one (`--config`/`SPEAK_CONFIG`,
`--provider`/`SPEAK_PROVIDER`); keys come from env vars the block names.
`--tts-url`/`SPEAK_TTS_URL` overrides the local engine's address,
`--port`/`SPEAK_PORT` (default 7425) and `--state-dir`/`SPEAK_STATE_DIR` are
as before. All are root flags, so serve, mcp, and doctor resolve identically;
see `config.md`.
