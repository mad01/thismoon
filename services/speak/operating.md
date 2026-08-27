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
Playback state on disk lives under {{.StorePath}}: per-sentence WAV files in
`audio/` (reaped after 24 hours on start) and `playback.lock` plus a `.owner`
sidecar naming the current lock holder. The sentence queue, position, and
pause state live only in the memory of the mcp process that started playback.

## failure modes

Start with `speak doctor`: four checks, one line each, FAIL lines naming the
cause. In order: tts-engine-reachable pings the engine the way serve's
/enginez does, store-readable opens the state dir, and service-reachable plus
version-skew probe the optional web surface at {{.BaseURL}}. The last two
failing means only the web page is down; the MCP tools can still speak as
long as tts-engine-reachable passes.

FAIL tts-engine-reachable: the engine sidecar is down, and nothing can
synthesize — not the web page, not the tools. `t-man status speak-tts`, then
`t-man restart speak-tts`. From the web side the same fact shows as `GET
{{.BaseURL}}/enginez` answering 502; `/healthz` proves only that serve is up,
never the engine.

FAIL service-reachable: serve is not running, so the upload page and the
speech proxy are down. t-man supervises it as speak-web: `t-man list`, then
`t-man restart speak-web`. Playback tools are unaffected while the engine
answers.

Calls succeed but nothing is audible: afplay plays on the system default
output device, so check the volume and the selected output device first. Then
call `speak_status`: it reports engine reachability, the playback state, and
`last_result`, which records the TTS or playback error that ended the worker.
An "empty audio" error means the engine answered 200 with no body.

BUSY reply: one playback session at a time, serialized across processes by a
lock on `playback.lock`; a second caller gets "BUSY | ..." naming the holder
instead of talking over the first. Pause suspends afplay and releases the
lock; resume re-acquires it. Stop kills the current afplay and saves the
sentence index, so resume restarts from there. Clear a stuck session by
stopping it from the process that owns it, or by ending that process; either
frees the lock.

## version skew

`speak version -o json` reports the build of the binary on PATH. `GET
{{.BaseURL}}/version` reports the build the running serve came from. When the
`commit` values differ, an old process survived an upgrade: `t-man restart
speak-web` and compare again. `speak doctor` runs this comparison as its
version-skew check; the mcp shim always runs the PATH binary, so it never
skews on its own.

## first moves

1. `speak doctor`: engine, state dir, web surface, and version skew in one pass
2. FAIL tts-engine-reachable: `t-man restart speak-tts`, then `speak doctor` again
3. FAIL service-reachable or version-skew only: `t-man restart speak-web`; tools keep working meanwhile
4. `speak_status` for playback state, the lock holder, and the last worker error
5. All checks pass but nothing is audible: volume and output device, then `speak_status` last_result
