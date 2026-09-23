# operating speak

speak turns text and markdown into audio through a TTS provider: the local
Kokoro engine (mlx-audio) by default, or OpenRouter, OpenAI, a LiteLLM
proxy or the Gemini API. It is two independent surfaces in one binary: `speak serve` is a web
page plus a CORS-guarded `/v1/audio/speech`, with audio playing in the
browser; `speak mcp` is a stdio MCP server that plays audio on the machine's
speakers with afplay. Both need the provider; neither needs the other.

## how it runs

`speak serve` listens on localhost (default {{.BaseURL}}) and synthesizes
through the provider ~/.config/speak/config.yaml selects (`speak config`
shows which, and why a block cannot be used). Without that file it is the
local engine at http://127.0.0.1:8765, a separate process supervised as the
t-man agent speak-tts. Remote providers read their key from an environment
variable; speak-web and MCP hosts start through the recipe's speak-env.sh,
which pulls it from the secrets file. Machines
usually also route http://speak.this to serve via the local domain front door;
if the localhost port answers but the .this host does not, the router is the
problem, not this service.

`speak mcp` does not go through serve. Playback lives in the mcp process
itself: each tool call splits text into sentences, fetches one clip per
sentence from the provider, and plays it with afplay. The only dependency the
two surfaces share is the provider.

## where state lives

No documents persist: serve renders markdown per request and writes nothing.
Playback state on disk lives under the state directory (--state-dir,
SPEAK_STATE_DIR): {{.StorePath}} on an install that already has it,
otherwise ~/.local/state/speak. It holds per-sentence WAV files in `audio/`
(reaped after 24 hours on start) and `playback.lock` plus a `.owner` sidecar
naming the current lock holder. The sentence queue, position, and pause
state live only in the memory of the mcp process that started playback.

## failure modes

Start with `speak doctor`, one line per check, FAIL lines naming the cause.
config parses the file; one provider line per block fails for the active
block's problem (an unset key, say) and only notes an inactive one's;
tts-engine-reachable pings a local engine; tts-synthesis speaks a short test
phrase, since a running provider can still fail to speak; voices checks the
default voice is one the provider offers; store-readable opens the state
dir; service-reachable and version-skew probe the optional web surface at
{{.BaseURL}}. Only those last two failing leaves the MCP tools able to
speak. The `speak_doctor` tool returns the same checks as JSON.

Every surface reports one health state: ok, degraded (a failure that may pass
on its own, fewer than three in a row) or down, with the reason. `GET
{{.BaseURL}}/enginez` returns it as JSON, from the last real request when that
is under a minute old and from a test synthesis otherwise; `/healthz` proves
only that serve is up. The web page shows it as a banner, and a failed play
turns its button red and raises a toast. speak_text and speak_file synthesize
the first sentence before replying, so a dead engine comes back as an error
reply starting UNAVAILABLE; speak_resume retries that session once fixed.

FAIL config or the active provider: speak starts anyway and fails every
synthesis with that reason, never falling back to another provider. Fix the
file or set the variable the problem names; for a key, the secrets file
speak-env.sh reads. `speak config` lists every block and its problem.

FAIL tts-engine-reachable: the local engine sidecar is down, and nothing can
synthesize. `t-man status speak-tts`, then `t-man restart speak-tts`.

FAIL tts-synthesis: the provider answers but cannot speak; the detail carries
its own reason, and an auth failure names the key variable to check. "broke off the audio stream" or "empty audio" means it failed
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

UNREADABLE reply: speak_file found the file but may not read it (its mode,
or macOS privacy protection on the MCP host). Read it yourself and pass the
text to speak_text.

## version skew

`speak version -o json` reports the build of the binary on PATH. `GET
{{.BaseURL}}/version` reports the build the running serve came from. When the
`commit` values differ, an old process survived an upgrade: `t-man restart
speak-web` and compare again. `speak doctor` runs this comparison as its
version-skew check; the mcp shim always runs the PATH binary, so it never
skews on its own.

## first moves

1. `speak doctor`: config, providers, synthesis, voices, state dir, web surface, version skew
2. FAIL config or the active provider: `speak config`, then fix the file or the key it names
3. FAIL tts-engine-reachable or tts-synthesis on local: `t-man restart speak-tts`, `t-man logs speak-tts`
4. FAIL service-reachable or version-skew only: `t-man restart speak-web`; tools keep working meanwhile
5. `speak_status` for tts_health, playback state, the lock holder, and the last worker error
