# speak architecture

## Overview

speak is two independent surfaces built from one component. `speak serve`, a
t-man launchd agent on port 7425 behind `http://speak.this/`, serves the
read-aloud web page and an OpenAI-style `/v1/audio/speech`; audio plays in the
browser, mostly from a disk cache serve fills ahead of playback. `speak mcp`
is a stdio MCP server that plays audio on the machine's speakers via
`afplay`; it needs the provider but not serve. Both synthesize
through the TTS provider a config file selects: the local Kokoro engine
(mlx-audio on `127.0.0.1:8765`, a recipe-managed sidecar in the consuming repo,
docs/adr/0006) by default, or OpenRouter, OpenAI, a LiteLLM proxy or the
Gemini Developer API. The release artifact here is the Go program alone.

## Structure

```
cmd/speak/           entrypoint, delegates to internal/cli
internal/cli/        cobra: root flags (root.go), serve, mcp (mcp.go), config
                     (config.go), doctor (doctor.go), docs, version; provider.go
                     builds the provider every subcommand uses
internal/web/        server.go (mux, cache reaper), docs.go (uploaded docs and
                     their audio routes), speech.go (speech handler,
                     /enginez), cors.go (the cross-origin allowlist),
                     markdown.go (goldmark render, section split, part
                     plan), assets/shell.html + assets/app.js
internal/chunk/      sentences grouped into parts, sized by a ramp
internal/audiocache/ serve's clip cache on disk and the background Preparer
internal/config/     the provider config file: a block per provider, one active
internal/provider/   the active provider built from config: client and voices
internal/tts/        tts.Error (classified failure), tts.Health (ok, degraded or
                     down, with the reason), tts.Audio (WAV/MP3 normalization)
internal/ttsclient/  HTTP client for OpenAI-compatible speech endpoints
internal/gemini/     client for the Gemini API's generateContent speech
internal/playback/   afplay engine: sessions, flock, part prefetch,
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
`<section class="doc-section" data-section="N">` blocks at every h1/h2, and
plans each section's parts (`chunk.Prepared`: the first up to 250
characters, then up to 600). Each section carries `data-ra-parts`, its part
keys in play order, and every read block carries `data-ra-chunk`, the keys
of the parts that read it, for highlighting. serve keeps the document in
memory (the 32 most recent; an evicted one stops preparing), queues its
parts in reading order for background synthesis up to 25,000 characters,
ahead of older uploads' queued parts, and returns `{name, content, doc}` JSON, `doc`
being the audio state. `app.js` mounts the content, re-mounts
`<wk-read-aloud targets=".doc-section">`, and shows each section's audio
state, polling `GET /doc/{id}` while parts are queued or generating. A play
button fetches the section's parts from `GET /audio/{key}` with two in
flight; a part not ready yet jumps the queue and the request waits for it.
`GET /doc/{id}/audio` joins the ready parts into one download.

Why ahead of time: remote speech models answer with the whole clip at once
and take seconds to tens of seconds per part (Gemini 3.1 Flash TTS through
OpenRouter measured 21.6s for 120 words), so synthesizing on play means
waiting on play. From the cache a part answers in about a millisecond.

Speech path: `internal/web/speech.go` decodes the OpenAI-style request and
synthesizes through the active provider (`internal/provider`, built once at
startup from `internal/config`), which maps a voice it does not offer to its
default and calls its backend client: `internal/ttsclient` against an
OpenAI-style `/v1/audio/speech`, or `internal/gemini` against the Gemini
API's `generateContent`. Raw PCM answers get a WAV header (`tts.Normalize`).
The clip is stored in the audio cache, so the same request (provider, model,
resolved voice, speed, text) answers from disk next time; present pages and
text selections, which have no prepared parts, still gain from that on a
replay. `OPTIONS` preflight is answered locally. `cors.go` decides who may
fetch, for every route: an `Origin` on loopback or under `.this` is
reflected back with `Vary: Origin`, and any other `Origin`, or a cross-site
request without one that is not a top-level load of `GET /`
(`Sec-Fetch-Site`, `Sec-Fetch-Dest`), gets 403.
Pages on other local origins such as present briefings keep working while a
page from the internet cannot start a synthesis. Every outcome feeds one `tts.Health`: a failure is answered
with a JSON error body naming the reason and emits an `error` event through
`kit/notify`. `GET /enginez` reports that health, running a test synthesis
when nothing fresh is recorded; `GET /healthz` proves only that the page is
up.

MCP path: `speak_text`/`speak_file` extract plain text from the input,
split it into sentences (`chunk.SplitSentences`, hand-rolled because Go
`regexp` has no lookbehind) and group them into parts with `chunk.Live`: one
sentence, then up to 250 characters, then up to 600, never across a file
section. They synthesize the first part before replying (so a backend that
cannot speak comes back as an `UNAVAILABLE` error reply) and start a worker
goroutine that keeps the next two parts synthesizing while one plays, writes
each clip under the state directory's `audio/`, and plays it with `afplay`.
Pause sends `SIGSTOP` to the afplay child and releases the lock; resume
re-acquires it and sends `SIGCONT`; stop saves the part index so resume
restarts the worker from there, cancelling syntheses in flight (a stop
during the first part's synthesis replies `Stopped before the first part was
ready.`).
An `flock` on the playback lock serializes sessions across processes: a
second caller gets a `BUSY` reply.

## Storage

Markdown is never written to disk: serve keeps the 32 most recent uploads in
memory, and recently-read docs live in the browser's localStorage, which the
page re-posts after a serve restart. Synthesized audio does go to disk, in
serve's cache at `<state-dir>/cache/<key>.wav` or `.mp3`; the key is 32 hex
characters of a sha256 over provider, model, resolved voice, speed and
text. A clip's mtime is bumped on every use. At start and daily serve
deletes clips unused for 30 days, then the least recently used until the
cache is under 2 GiB. The MCP playback engine writes per-part audio
files under `<state-dir>/audio/` (reaped after 24 hours on start) and holds
`<state-dir>/playback.lock` plus an `.owner` sidecar naming the lock holder.
The state directory is `--state-dir` / `SPEAK_STATE_DIR`, defaulting to
`~/.local/share/speak` where that already exists and `~/.local/state/speak`
on a fresh install. The consuming repo's recipe creates the engine's venv and
model cache (`~/.local/share/speak/venv`, `~/.cache/huggingface`); speak
never writes them.

## Interfaces

Web: `GET /` (upload page), `GET /app.js`, `POST /read`, `GET /doc/{id}`,
`POST /doc/{id}/prepare`, `GET /doc/{id}/audio`, `GET /audio/{key}`,
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
`--port`/`SPEAK_PORT` (default 7425) and `--state-dir`/`SPEAK_STATE_DIR`
(playback state and serve's audio cache) are as before. All are root flags,
so serve, mcp, and doctor resolve identically; see `config.md`.
