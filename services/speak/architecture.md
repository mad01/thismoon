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
internal/cli/        cobra: serve and version (root.go), mcp (mcp.go)
internal/web/        server.go (mux, CORS wrapper, TTS proxy, HTTP API),
                     markdown.go (goldmark render + section split),
                     assets/shell.html + assets/app.js (client render)
internal/ttsclient/  HTTP client for the engine (WAV over /v1/audio/speech)
internal/playback/   afplay engine: sessions, flock, sentence split,
                     pause/resume via SIGSTOP/SIGCONT
internal/mcpserver/  go-sdk MCP server: the 7 speak_* tools over playback
internal/notify/     best-effort event emit to events.this on proxy failures
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
engine, answers `OPTIONS` preflight locally, and sets
`Access-Control-Allow-Origin: *` on every response (the engine does no CORS),
which is what lets pages on other local origins, such as present briefings,
fetch speech from `http://speak.this`. A failed or 4xx/5xx proxied request
emits an `error` event through `internal/notify`; successful synthesis is not
logged. `GET /enginez` probes the engine behind the proxy, `GET /healthz`
proves only that the page is up.

MCP path: `speak_text`/`speak_file` extract plain text from the input,
split it into sentences (`playback.SplitSentences`, hand-rolled because Go
`regexp` has no lookbehind), and start a worker goroutine that fetches each
sentence's WAV through `internal/ttsclient`, writes it under
`~/.local/share/speak/audio/`, and plays it with `afplay`. Pause sends
`SIGSTOP` to the afplay child and releases the lock; resume re-acquires it and
sends `SIGCONT`; stop saves the sentence index so resume restarts the worker
from there. An `flock` on the playback lock serializes sessions across
processes — a second caller gets a `BUSY` reply.

## Storage

No documents: markdown is rendered per request and never written to disk
(recently-read docs live only in the browser's localStorage). The MCP playback
engine writes per-sentence WAV files under `~/.local/share/speak/audio/`
(reaped after 24 hours on start) and holds
`~/.local/share/speak/playback.lock` plus an `.owner` sidecar naming the lock
holder. The consuming repo's recipe creates the engine's venv and model
cache (`~/.local/share/speak/venv`, `~/.cache/huggingface`); speak never
writes them.

## Interfaces

Web: `GET /` (upload page), `GET /app.js`, `POST /read`,
`POST /v1/audio/speech` (engine proxy), `GET /healthz`, `GET /enginez`,
`GET /version`, `GET /webkit/` from the webkit Go package.

CLI: `speak serve`, `speak mcp`, `speak version [-o json]`.

MCP tools: `speak_text`, `speak_file`, `speak_pause`, `speak_resume`,
`speak_stop`, `speak_voices`, `speak_status`. Registering the MCP server with
a client is machine-private wiring and stays in the consuming repo.

Config: `--port`/`SPEAK_PORT` (default 7425) and `--tts-url`/`SPEAK_TTS_URL`
(default `http://127.0.0.1:8765`), shared by serve and mcp.
