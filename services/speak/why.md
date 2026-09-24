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
adds CORS and answers preflight, the reachability probes (`/healthz` for
serve, `/enginez` for the engine), and the error events when synthesis fails,
so every consumer talks to a stable local origin instead of the engine
directly. It cannot be a feature of present or webkit because speech is a
machine-wide capability: any page and any agent should reach it at
`speak.this`. And the engine itself cannot be the service: it is
machine-private wiring, installed and sandboxed by a recipe in the consuming
repo, while the Go component is what this repo can build, test, and release.

## Why this shape

The gateway boundary comes first: `speak serve` answers `/v1/audio/speech`
by synthesizing through the configured provider and supplies everything the
provider lacks (CORS on every response, preflight handling, probes, error
events, the audio cache), leaving the engine untouched. The release artifact is the Go program only; the
Kokoro venv, model prefetch, and no-egress sandbox are recipe-managed wiring
in the consuming repo (docs/adr/0006), so CI never ships a Python
environment. And `speak mcp` is an independent surface with playback
in-process: serve plays audio in the browser, mcp plays it on the speakers via
afplay, and the two share only the engine. Unlike reminder's MCP, a thin
client to its serve, speak's MCP does not funnel through serve, because the
shared resource here is the audio device rather than a store; an flock on a
playback lock file serializes it across processes instead.

Remote providers pushed serve to prepare audio ahead. Gemini 3.1 Flash TTS
through OpenRouter took 3.2 seconds for a 7-word sentence and 21.6 for 120
words, and it answers with the whole clip at once, so there is no streaming
to start playback early. Asking for one sentence at a time on play meant a
wait on every press and a gap whenever a short sentence ended before the
next, longer one arrived. So a page registers its text with serve, which
synthesizes the parts into a disk cache when the page asks or plays, and a
play press replays them: a 4-part document took 18 seconds to prepare, then
each part came back in about a millisecond. What is not registered, text
selections and the MCP tools, groups sentences into parts that start at one
sentence and keeps two parts synthesizing ahead of the one playing.

speak has no reading page of its own. It served one, an upload form that
rendered a markdown file and played it, until the reading controls moved into
webkit's `<wk-read-aloud>` and present mounted them: present already renders
every briefing and note an agent writes, so a second place to read was a
second UI to keep in step for the same audio. speak keeps the audio API and a
landing page that shows the engine state; a file an agent wants read goes
through the MCP tools or onto a present page.

## Non-goals

speak does not ship, install, or supervise the TTS engine; without that
sidecar the landing page loads and the speech endpoints return errors, by
design. It stores no documents: text is never written to disk, and serve
holds the 32 most recently registered documents in memory only; the page
that registered them registers again after a restart. What speak writes is
synthesized audio and the playback lock; serve's cache deletes a clip after
30 days unused and stays under 2 GiB. There is no cloud fallback; the engine
runs offline with zero network egress. MCP registration also stays out of
this repo: the `mcp` subcommand ships, but wiring it into a client is
machine-private (docs/adr/0006).
