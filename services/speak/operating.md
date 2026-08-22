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
sentence from the engine, and plays it with afplay. The shim being up says
nothing about serve, and serve being up says nothing about the shim; the only
dependency the two share is the engine.

## where state lives

No documents persist: serve renders markdown per request and writes nothing.
Playback state on disk lives under {{.StorePath}}: per-sentence WAV files in
`audio/` (reaped after 24 hours on start) and `playback.lock` plus a `.owner`
sidecar naming the current lock holder. The sentence queue, position, and
pause state live only in the memory of the mcp process that started playback.

## failure modes

Connection refused on {{.BaseURL}}: serve is not running. t-man supervises it
as speak-web. Run `t-man list`, then `t-man restart speak-web`.

"TTS engine not reachable": the engine sidecar is down, not speak. Check
`t-man status speak-tts` (restart with `t-man restart speak-tts`), or `GET
{{.BaseURL}}/enginez`, which answers 204 when the engine responds and 502 when
it does not. `/healthz` proves only that serve is up, never the engine.

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
speak-web` and compare again. The mcp shim always runs the PATH binary, so it
never skews on its own.

## first moves

1. `curl -s -o /dev/null -w '%{http_code}' {{.BaseURL}}/healthz` (204 means serve is up)
2. Same host, `/enginez` (204 means the engine answers, 502 means it is down)
3. If either fails: `t-man list`, then `t-man restart speak-web` or `t-man restart speak-tts`
4. `speak_status` for playback state, the lock holder, and the last worker error
5. Compare `speak version -o json` with `GET /version` for skew
