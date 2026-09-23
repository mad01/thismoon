# operating speak

speak turns text and markdown into audio with a local Kokoro TTS engine
(mlx-audio). It is two independent surfaces in one binary: `speak serve` is a
web page plus a CORS-adding reverse proxy for `/v1/audio/speech`, with audio
playing in the browser; `speak mcp` is a stdio MCP server that plays audio on
the machine's speakers with afplay. Both need the engine; neither needs the
other.

## how it runs

`speak serve` listens on localhost (default {{.BaseURL}}) and reverse-proxies
synthesis requests to the engine at http://127.0.0.1:8765 (--tts-url,
SPEAK_TTS_URL). The engine is a separate process, supervised as the t-man
agent speak-tts; serve only fronts it and holds no playback state. Machines
usually also route http://speak.this to serve via the local domain front door;
if the localhost port answers but the .this host does not, the router is the
problem, not this service.

`speak mcp` does not go through serve. Playback lives in the mcp process
itself: each tool call splits text into sentences, fetches one WAV per
sentence from the engine, and plays it with afplay. The only dependency the
two surfaces share is the engine.

## where state lives

No documents persist: serve renders markdown per request and writes nothing.
Playback state on disk lives under the state directory (--state-dir,
SPEAK_STATE_DIR): {{.StorePath}} on an install that already has it,
otherwise ~/.local/state/speak. It holds per-sentence WAV files in `audio/`
(reaped after 24 hours on start) and `playback.lock` plus a `.owner` sidecar
naming the current lock holder. The sentence queue, position, and pause
state live only in the memory of the mcp process that started playback.

## failure modes

Start with `speak doctor`: five checks, one line each, FAIL lines naming the
cause. tts-engine-reachable pings the engine; tts-synthesis speaks a short
test phrase through it, since a running engine can still fail to speak;
store-readable opens the state dir; service-reachable and version-skew probe
the optional web surface at {{.BaseURL}}. Only those last two failing leaves
the MCP tools able to speak. The `speak_doctor` tool returns the same checks
as JSON.

Every surface reports one health state: ok, degraded (a failure that may pass
on its own, fewer than three in a row) or down, with the reason. `GET
{{.BaseURL}}/enginez` returns it as JSON, from the last real request when that
is under a minute old and from a test synthesis otherwise; `/healthz` proves
only that serve is up. The web page shows it as a banner, and a failed play
turns its button red and raises a toast. speak_text and speak_file synthesize
the first sentence before replying, so a dead engine comes back as an error
reply starting UNAVAILABLE; speak_resume retries that session once fixed.

FAIL tts-engine-reachable: the engine sidecar is down, and nothing can
synthesize. `t-man status speak-tts`, then `t-man restart speak-tts`.

FAIL tts-synthesis: the engine answers but cannot speak; the detail carries
its own reason. "broke off the audio stream" or "empty audio" means it failed
after answering 200, so the cause is only in `t-man logs speak-tts`. A spaCy
download error there means the G2P warm-up never ran and the sandbox blocked
the lazy fetch.

FAIL service-reachable: serve is not running, so the upload page and the
speech proxy are down. t-man supervises it as speak-web: `t-man restart
speak-web`. Playback tools are unaffected.

A page fetches speech and the browser blocks it as a CORS error: serve
answers cross-origin only for an Origin whose host is loopback or ends in
.this. Open the page through its .this host or its localhost port; the
allowlist has no override.

Calls succeed but nothing is audible: afplay plays on the system default
output device, so check the volume and output device, then `speak_status`
for tts_health and `last_result`, the error that ended the worker.

BUSY reply: one playback session at a time, serialized across processes by a
lock on `playback.lock`; a second caller gets "BUSY | ..." naming the holder.
Pause releases the lock and resume re-acquires it; stop saves the sentence
index for resume. Clear a stuck session by stopping it from the owning
process or ending that process.

## version skew

`speak version -o json` reports the build of the binary on PATH. `GET
{{.BaseURL}}/version` reports the build the running serve came from. When the
`commit` values differ, an old process survived an upgrade: `t-man restart
speak-web` and compare again. `speak doctor` runs this comparison as its
version-skew check; the mcp shim always runs the PATH binary, so it never
skews on its own.

## first moves

1. `speak doctor`: engine, synthesis, state dir, web surface, and version skew in one pass
2. FAIL tts-engine-reachable: `t-man restart speak-tts`, then `speak doctor` again
3. FAIL tts-synthesis: `t-man logs speak-tts` for the engine's own error
4. FAIL service-reachable or version-skew only: `t-man restart speak-web`; tools keep working meanwhile
5. `speak_status` for tts_health, playback state, the lock holder, and the last worker error
