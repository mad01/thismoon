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

serve synthesizes an uploaded document ahead of playback: its parts (up to
600 characters each, three at a time) go into a disk cache, up to about 30
minutes of speech, and the page's Prepare all queues the rest. The newest
upload goes first. Playing a part that is not ready moves it to the front and
waits for it. A remote model answers a part in seconds to tens of seconds,
all at once, so audio that was not prepared starts late.

`speak mcp` does not go through serve or its cache. Playback lives in the mcp
process itself: each tool call groups the text's sentences into parts (one
sentence first, then up to 250 and 600 characters), synthesizes the first
part before replying, and plays each with afplay while the next two
synthesize. The only dependency the two surfaces share is the provider.

## where state lives

Disk state lives under the state directory (--state-dir, SPEAK_STATE_DIR):
{{.StorePath}} on an install that already has it, otherwise
~/.local/state/speak. serve writes `cache/`, one audio file per part, named by
a hash of provider, model, voice, speed and text. At serve start and daily it
deletes files unused for 30 days, then the least recently used until the
cache is under 2 GiB; serve's startup log line names the directory. mcp
writes per-part audio in `audio/` (reaped after 24 hours on start) and
`playback.lock` plus a `.owner` sidecar naming the current lock holder.

serve keeps uploaded documents in memory only, the 32 most recent; an older
one stops preparing. After a restart the open page posts its markdown again
and prepared parts play from the cache. The mcp part queue, position, and
pause state live only in the memory of the mcp process that started
playback.

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
the first part before replying, so a dead engine comes back as an error
reply starting UNAVAILABLE; speak_resume retries that session once fixed.

serve retries a part that failed upstream by itself, up to 3 attempts, 15s
then 45s apart; meanwhile its section badge reads "retrying" and its title
names the last failure. A part out of attempts is failed, with a reason
starting "failed after 3 attempts", and its section gets a Retry button; the
page's "Retry failed (n)" re-queues only the failed parts, Prepare all the
failed and the not prepared ones, and playing a part also tries it again.
After an auth, quota, network, config or model failure, or a part whose
last attempt timed out too, serve stops preparing in the background (queued
and retrying parts go back to not prepared) rather than spend requests on
the same error; parts someone plays still run. Fix the cause, or for a
stalling provider wait a while, then Prepare all. events.this gets at most
one preparation-failure event a minute; the badge and health still show
every failure.

"did not answer within 1m30s" (30s on the local engine, less under a
caller's shorter deadline) means the provider was reached but was slow, not
that the network failed; "not reachable" is a failed connection. A remote
request that timed out is retried once, and " (2 attempts)" at the end means
it stalled twice in a row. Stalls are usually random: a background part
then gets up to 3 attempts of its own, and playing the part again or
pressing Retry usually works. A 10s probe (doctor, /enginez) never retries.

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

A page's speech request gets 403 or a CORS error: serve answers only an
Origin whose host is loopback or ends in .this, and refuses a cross-site
request that sends no Origin unless it opens the page itself. Open the page
through its .this host or its localhost port; the allowlist has no override.
curl and the CLI send no Origin and are unaffected.

Calls succeed but nothing is audible: afplay plays on the system default
output device, so check the volume and output device, then `speak_status`
for tts_health and `last_result`, the error that ended the worker.

BUSY reply: one playback session at a time, serialized across processes by a
lock on `playback.lock`; a second caller gets "BUSY | ..." naming the holder.
Pause releases the lock and resume re-acquires it; stop saves the part
index for resume and cancels syntheses in flight, and a stop during the
first part's synthesis replies "Stopped before the first part was ready."
Clear a stuck session by stopping it from the owning
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
