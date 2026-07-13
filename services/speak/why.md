# why speak

## The problem

Long documents on the machine, briefing pages, research notes, anything an
agent writes, are often better listened to than read. Local TTS makes that
possible without sending text anywhere, but the engine providing it (Kokoro
via mlx-audio) is a raw Python server with sharp edges: no CORS, so browser
pages cannot call it cross-origin; failure modes like a 200 with an empty
body when ffmpeg is missing; and version pins that must be exactly right
(0.4.4 ships a broken vocoder). Nothing turned "read this markdown aloud" or
"say this" into audio reliably.

## Why its own service

speak is the front for the engine. One component owns the reverse proxy that
adds CORS and answers preflight, the reachability probes (`/healthz` for the
page, `/enginez` for the engine), and the error events when synthesis fails,
so every consumer talks to a stable local origin instead of the engine
directly. It cannot be a feature of present or webkit because speech is a
machine-wide capability: any page and any agent should reach it at
`speak.this`. And the engine itself cannot be the service: it is
machine-private wiring, installed and sandboxed by a recipe in the consuming
repo, while the Go component is what this repo can build, test, and release.

## Why this shape

The proxy boundary comes first: `speak serve` reverse-proxies
`/v1/audio/speech` to the engine and supplies everything the engine lacks
(CORS on every response, preflight handling, probes, error events), leaving
the engine untouched. The release artifact is the Go program only; the
Kokoro venv, model prefetch, and no-egress sandbox are recipe-managed wiring
in the consuming repo (docs/adr/0006), so CI never ships a Python
environment. And `speak mcp` is an independent surface with playback
in-process: serve plays audio in the browser, mcp plays it on the speakers via
afplay, and the two share only the engine. Unlike reminder's MCP, a thin
client to its serve, speak's MCP does not funnel through serve, because the
shared resource here is the audio device rather than a store; an flock on a
playback lock file serializes it across processes instead.

## Non-goals

speak does not ship, install, or supervise the TTS engine; without that
sidecar the page loads and the speech endpoints return errors, by design. It
stores no documents: markdown is rendered per request and never written to
disk, and recently-read docs are remembered only in the browser's
localStorage. There is no cloud fallback; the engine runs offline with zero
network egress. MCP registration also stays out of this repo: the `mcp`
subcommand ships, but wiring it into a client is machine-private
(docs/adr/0006).
