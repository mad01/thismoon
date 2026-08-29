# speak architecture

## Overview

speak is two independent surfaces built from one component. `speak serve`, a
t-man launchd agent on port 7425 behind `http://speak.this/`, serves the
read-aloud web page and reverse-proxies `/v1/audio/speech` to the local Kokoro
TTS engine (mlx-audio on `127.0.0.1:8765`); audio plays in the browser.
`speak mcp` is a stdio MCP server that plays audio on the machine's speakers
via `afplay`; it needs the engine but not serve. The two share only the
engine, which is not part of this component: it is a recipe-managed sidecar in
the consuming repo (docs/adr/0006), and the release artifact here is the Go
program alone.

## Structure

```
cmd/speak/           entrypoint, delegates to internal/cli
internal/cli/        cobra: root flags (root.go), serve, mcp (mcp.go),
                     doctor (doctor.go), docs (docs.go), version (version.go)
internal/web/        server.go (mux, TTS proxy, HTTP API), cors.go (the
                     cross-origin allowlist), markdown.go (goldmark render +
                     section split), assets/shell.html + assets/app.js
internal/ttsclient/  HTTP client for the engine (WAV over /v1/audio/speech)
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

Proxy path: `internal/web/server.go` reverse-proxies `/v1/audio/speech` to the
engine and answers `OPTIONS` preflight locally (the engine does no CORS).
`cors.go` decides who may fetch: an `Origin` on loopback or under `.this` is
reflected back with `Vary: Origin`, and anything else gets no CORS headers,
so pages on other local origins such as present briefings keep working while
a page from the internet cannot reach the engine. A failed or 4xx/5xx proxied
request emits an `error` event through `kit/notify`; successful synthesis is
not logged. `GET /enginez` probes the engine behind the proxy, `GET /healthz`
proves only that the page is up.

MCP path: `speak_text`/`speak_file` extract plain text from the input,
split it into sentences (`playback.SplitSentences`, hand-rolled because Go
`regexp` has no lookbehind), and start a worker goroutine that fetches each
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

CLI: `speak serve`, `speak mcp`, `speak doctor`, `speak docs`,
`speak version [-o json]`.

MCP tools: `speak_text`, `speak_file`, `speak_pause`, `speak_resume`,
`speak_stop`, `speak_voices`, `speak_status`, `speak_doctor`. Registering the
MCP server with a client is machine-private wiring and stays in the consuming
repo; `speak mcp --help` prints the snippet.

Config: `--port`/`SPEAK_PORT` (default 7425), `--tts-url`/`SPEAK_TTS_URL`
(default `http://127.0.0.1:8765`), and `--state-dir`/`SPEAK_STATE_DIR`. All
three are root flags, so serve, mcp, and doctor resolve identically; see
`config.md`.
