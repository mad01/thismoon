# speak, read markdown aloud over localhost

Go CLI serving a local page where you upload a markdown file, see it rendered
inline, and play it section by section with the shared `<wk-read-aloud>` webkit
component. The same process serves an OpenAI-style `/v1/audio/speech` behind a
CORS allowlist, so pages on this machine's other local origins (present.this,
localhost) can fetch speech from it too, and pages from anywhere else cannot.
Speech comes from the provider `~/.config/speak/config.yaml` selects: the local
Kokoro engine (mlx-audio) by default, or OpenRouter, OpenAI, a LiteLLM proxy
or the Gemini Developer API.

## Module layout

```
services/speak/
  cmd/speak/           # entrypoint (delegates to internal/cli)
  internal/
    cli/               # cobra: root flags (--config/--provider/--tts-url/--port/
                        # --state-dir), serve, mcp, config (config/active/env),
                        # doctor, docs, version (build metadata from the shared
                        # buildinfo package); provider.go builds the provider
                        # every subcommand uses
    web/               # server.go (mux, HTTP API), speech.go (speech handler +
                        # /enginez health), cors.go (the
                        # cross-origin allowlist), markdown.go
                        # (goldmark render + section split), assets/shell.html
                        # (chrome-only shell) + assets/app.js (client render)
    config/            # the provider config file: a block per provider, the
                        # active one selected; resolves type defaults and keys
                        # from env, marks unusable blocks with a Problem
    provider/          # builds the active provider from config: its client, its
                        # voices (curated, discovered, or catalog), or a broken
                        # provider that fails every synthesis with the reason
    tts/               # shared by every surface: tts.Error (classified failure:
                        # auth/quota/model/network/config/upstream), tts.Health
                        # (ok/degraded/down + reason), tts.Audio + Normalize
                        # (WAV/MP3 pass through, raw PCM gets a WAV header)
    ttsclient/         # HTTP client for OpenAI-compatible speech endpoints (local
                        # engine, OpenRouter, OpenAI, LiteLLM); every failure
                        # comes back as a *tts.Error
    gemini/            # client for the Gemini API's generateContent speech (key
                        # in x-goog-api-key); same tts.Request/Audio/Error contract
    playback/          # server-side afplay engine: sessions, pause/resume via
                        # SIGSTOP/SIGCONT, flock, sentence split, md text extract
    mcpserver/         # go-sdk MCP server: 8 speak_* tools over the playback engine
  Makefile             # part of module github.com/mad01/thismoon (no own go.mod)
```

## How it works

- `speak serve --port 7425`. `--config`, `--provider`, `--tts-url`, `--port`
  and `--state-dir` (each with a `SPEAK_*` env twin) are persistent root
  flags, so serve, mcp, and doctor cannot resolve different providers, ports
  or state directories. `cli.activeProvider` builds the provider for all of
  them. Full surface, including the config file, in `config.md`.
- **Providers.** One config file holds a block per provider side by side; the
  `provider:` line (or `--provider`/`SPEAK_PROVIDER`) picks one. Types:
  `local`, `openrouter`, `openai` and `litellm` share `POST
  {base}/v1/audio/speech` and go through `ttsclient`; `gemini` goes through
  `internal/gemini`. `provider.Provider` holds either behind a one-method
  `synthesizer` interface. Keys come only from env vars the block names;
  `speak config env` lists them and `recipes/speak/speak-env.sh` pulls
  exactly those from the secrets file for speak-web and MCP hosts. No
  automatic fallback: a broken config or unusable active block becomes a
  provider that fails every synthesis with a `config` reason, so every
  surface shows it.
- `POST /read` (multipart field `doc`, max 5MB) renders markdown via goldmark
  (GFM) and splits it into `<section class="doc-section">` blocks at every
  h1/h2; those blocks are the per-button play units of
  `<wk-read-aloud targets=".doc-section" endpoint="">` (empty endpoint = same
  origin). No storage; render per request.
- `POST /v1/audio/speech` synthesizes through the active provider (the
  request's `model` and `response_format` are ignored; a voice the provider
  doesn't offer becomes its default) and answers `OPTIONS` preflight
  locally. `internal/web/cors.go`
  is the allowlist: an `Origin` that is an http/https URL on loopback or under
  `.this` is reflected back with `Vary: Origin`; anything else gets no CORS
  headers. The wrapper handler applies it to every response, so the
  component's cross-origin `GET /` reachability probe works from the sibling
  `.this` pages. **Never widen this to `*`** — the endpoint drives the
  machine's TTS engine, so `*` lets any page the user is browsing use it.
- **Failures carry a reason; nothing fails silently.** A failed synthesis is
  answered with `{"error": {"message", "type", "provider", "model",
  "health"}}` (the OpenAI error shape plus provider and health): 502 for a
  provider failure (speak is the gateway), 429 for a rate limit, 503 for a
  config problem. `<wk-read-aloud>` shows the message as a toast. Every
  outcome is recorded in one `tts.Health` per process, and `/enginez` reports
  it. A 200 with no audio, or a stream broken off mid-body (mlx-audio does
  both when it fails after answering), counts as an upstream failure.
  Failures also emit an `error` event to events.this via `kit/notify`
  (fire-and-forget); success is recorded but never emitted, since read-aloud
  fans out one request per sentence and would flood the event log.
- Chrome comes from the in-module webkit package (`GET /webkit/`). The
  service compiles against the webkit committed beside it; there is no version
  to pin or bump.

### TTS engine (the other half)

mlx-audio runs as the separate t-man agent `speak-tts` from the venv at
`~/.local/share/speak/venv`, created by the consuming repo's `speak-tts`
recipe (machine wiring that stays out of this repo; see `docs/adr/0006`).
Gotchas that cost time once:

- **Pin `mlx-audio==0.4.3` + `mlx==0.31.1`.** 0.4.4's Kokoro vocoder is broken
  (`[broadcast_shapes]` ValueError in istftnet on every input).
- **`mlx-audio[server]` + `misaki[en]` are both required**: the bare package
  is missing uvicorn/fastapi, and Kokoro imports misaki at request time.
- **Send `model`.** mlx-audio answers 422 "Field required" without it;
  `ttsclient` sends `speak.DefaultModel` (the same id the read-aloud
  component sends). Before speak's health reporting this 422 was invisible:
  the MCP tools replied "Playing" and stayed silent.
- **Request `response_format: "wav"`.** The default mp3 path shells out to
  ffmpeg, which may not be installed; failures surface as a 200 with an empty
  streamed body, not an error status. Any engine error after the status line
  (a G2P crash, say) looks the same: 200, then an empty or broken-off body.
  speak reports those as upstream failures pointing at `t-man logs speak-tts`.
- **misaki pulls `en_core_web_sm` via `uv pip install` on first G2P**: that
  subprocess needs `VIRTUAL_ENV` set or it dies with "No virtual environment
  found" and the request hangs. The recipe warms G2P at install time and the
  t-man agent sets `--env VIRTUAL_ENV=...` as a belt-and-suspenders.
- **The engine runs SANDBOXED with zero network egress.** The consuming
  repo's `speak-tts` recipe registers it with a seatbelt profile: no outbound
  network (offline mode `HF_HUB_OFFLINE=1`; the Kokoro model is pre-fetched at
  install time), `$HOME` reads denied outside the venv + caches, writes
  confined. If synthesis breaks after a change, check denials first:
  `recipes/speak/sandbox-audit.sh stream` (this repo). Triage docs:
  `recipes/speak/CLAUDE.md`.
- The model must be in `~/.cache/huggingface` BEFORE the engine can serve (the
  sandbox blocks the lazy download path); the recipe's install script handles
  this. Warm latency is ~100–200ms per sentence on M-series.

## Build / install / test

```bash
make build    # ./speak binary (build metadata via ldflags, from ../../buildinfo.mk)
make install  # build + cp to ~/code/bin/speak + adhoc codesign
make test     # go test ./...
```

## HTTP API

| Path | Description |
|------|-------------|
| `GET /` | Upload form (embedded `shell.html`; body built client-side by `app.js`) |
| `GET /app.js` | Client renderer; `Cache-Control: no-cache` so a rebuild is picked up on next load |
| `POST /read` | Render and split a markdown file for playback; returns `{name, content}` JSON |
| `POST /v1/audio/speech` | OpenAI-style speech through the active provider; answers WAV or MP3 (reflects an allowlisted origin). A failure answers JSON `{"error": {"message", "type", "provider", "model", "health"}}`: 502 for a provider failure, 429 rate limit, 503 config problem, 400 for a request without `input` |
| `GET /healthz` | 204; the `<wk-read-aloud>` component's cross-origin reachability probe against `GET /` gets its CORS header from the wrapper handler, which applies the allowlist to every response |
| `GET /enginez` | TTS health as JSON (`status` ok/degraded/down/unknown, `provider`, `model`, `kind`, `reason`, `checked_at`); 200 when ok, 503 otherwise. Answers from the last recorded outcome when under a minute old, else runs a test synthesis first (a ping would call a running engine with a missing model healthy). `app.js` renders it as the banner; distinct from `/healthz`, which only proves this page is up |
| `GET /version` | The four-key build metadata object (`version`, `commit`, `tag`, `build_time`), the HTTP twin of `speak version -o json`, which ralph uses for update detection |
| `GET /webkit/` | Shared chrome from the in-module `webkit` package |

## Shared UI: webkit

The chrome (`<wk-header>` + theme/font/size/fixation controls) comes from the
in-module package **`github.com/mad01/thismoon/webkit`**, mounted at
`GET /webkit/` via `webkit.Mount(mux)` and loaded by `internal/web/assets/shell.html`
(which pulls the FOUC guard from `/webkit/boot.js`). Don't re-add
palette/topbar/theme CSS locally; it lives in webkit only.

### Header markup

`shell.html` uses:

```html
<wk-header brand="speak·aloud" controls="cmdk,font,fixation,size,speed,reload,theme"
  fixation-targets="[data-fixation], .doc-section p, .doc-section li"></wk-header>
```

### Per-repo changes

- Page chrome lives in `internal/web/assets/shell.html`; only speak-specific
  styles (upload form, drop overlay, `.doc-section` rendering) live in its
  inline `<style>` block.
- webkit components speak uses: `<wk-page-header>` + `<wk-title>` +
  `<wk-subtitle>` for the hero block, `<wk-callout variant="warn">` for the
  engine-down banner, and `<wk-read-aloud targets=".doc-section" endpoint="">`
  (see `webkit/COMPONENTS.md`) mounted fresh after every upload since it reads
  its targets once on connect.
- Recent-docs chips and drag/drop/paste handling are speak-local logic in
  `app.js`, not webkit components.

### Version check

`GET /webkit/version` confirms which embedded webkit assets the running
`speak` server serves.

## MCP tools

`speak mcp` is a second, independent surface from `speak serve`. serve plays
audio **in the browser** (the `<wk-read-aloud>` component fetches per-sentence
WAV and plays it there); mcp plays audio **on the machine's speakers** via
`afplay`, so an agent can make the host talk. They share only the TTS engine.

- `speak_text`: speak a text string on the host speakers.
- `speak_file`: read a file (text or markdown) aloud.
- `speak_pause` / `speak_resume` / `speak_stop`: control the running playback
  session; stop saves the sentence index so a later resume restarts there.
- `speak_voices`: list the active provider's voices with the default marked,
  and where the list came from (config, discovered, catalog, default).
- `speak_status`: report the playback state, current session, and
  `tts_health` (the health recorded from this process's syntheses).
- `speak_doctor`: run the same checks as `speak doctor` and return the report
  as JSON, for a client that can call a tool but has no shell.

Names and behaviour are ported from the Python `speak_mcp.py` server so agent
muscle memory carries over. Implementation notes:

- **Playback lives in the mcp process**, not in serve. `speak mcp` does not
  require `speak serve` to be running — it only needs the `speak-tts` engine
  reachable on `--tts-url`. This is deliberately unlike reminder's MCP (a thin
  client to its serve): the shared resource here is the audio device,
  serialised by an `flock`, not a JSON store, so there is no single-writer
  file to funnel through.
- `internal/playback` runs a worker goroutine over the sentence list: fetch WAV
  from `internal/ttsclient`, write it under the state directory's `audio/`,
  `afplay` it, `Wait`. Pause = `SIGSTOP` the afplay child + release the lock;
  resume = re-acquire the lock + `SIGCONT`.
- **One session at a time, cross-process.** An `flock` on `playback.lock` in
  the state directory (with a `.owner` sidecar naming the holder) means a
  second `speak mcp` gets a `BUSY | …` reply. Old WAVs are reaped after 24h
  on start. The engine takes the directory from `--state-dir`, so two
  processes only serialize against each other when they resolve the same one
  — which is why the flag is a persistent root flag, not a per-command one.
- **Go `regexp` has no lookbehind** — the sentence splitter
  (`playback.SplitSentences`) is hand-rolled, not a translation of the Python
  `re.split(r'(?<=[.!?])\s+')`.
- **The `voice` parameter is the provider's voice list.** `registerTools`
  builds the speak_text/speak_file input schemas from the struct, then pins
  `voice` to an enum of the provider's voices (resolved once at startup:
  discovery runs then, with a 3s budget for OpenRouter). With no known list
  it stays free text. `OpenWorldHint` is true when the provider is remote,
  since text then leaves the machine.
- **speak_text and speak_file synthesize the first sentence before
  replying.** A backend that cannot speak comes back as `Failed` with an
  `UNAVAILABLE | TTS <health>` message, flagged `isError` over MCP, instead
  of a "Playing" reply that stays silent. The queue is kept as a stopped
  session, so `speak_resume` retries it. `startMu` serializes starts and
  stops: a start holds the playback lock while it synthesizes, before
  `running` is set, so an unserialized second start would see "not running"
  and release that lock mid-synthesis.
- **stdout is the MCP protocol channel** — `runMCP` logs the resolved tts-url to
  stderr only.
- **MCP registration stays host-gated in the consuming repo** (docs/adr/0006) —
  the binary ships the `mcp` subcommand, but wiring it into a client is
  machine-private and is NOT added to `recipes/speak/` here.

## Gotchas

- **Runtime debugging lives in `operating.md`** (embedded in the binary,
  printed by `speak docs`): serve/engine reachability, playback lock and
  pause/resume semantics, no-audio triage, version-skew checks. Keep those
  facts there, not here.
- **Version probe convention.** `GET /version` and `speak version -o json` both
  return the shared four-key build metadata object (`version`, `commit`, `tag`,
  `build_time`, every key present and `""` when unknown) from
  `github.com/mad01/thismoon/buildinfo`, so ralph can check which build is live.
  Plain `speak version` stays a bare token — status parses it as one.
- **Two processes, one release artifact.** The release artifact is the Go
  binary only. With the default local provider, synthesis needs the
  recipe-managed Kokoro sidecar running on `:8765`; without it the page loads
  and the speech endpoints return errors. CI builds and releases never ship
  the engine. A machine on a remote provider needs no engine at all.
- **OpenRouter offers mp3 or pcm, never wav.** The openrouter type requests
  pcm and `tts.Normalize` wraps it in a WAV header, taking the sample rate
  from the `Content-Type` parameters (24 kHz mono when absent).
- **The Gemini API answers a bad key with 400, not 401.** Its error body
  carries `INVALID_ARGUMENT` with reason `API_KEY_INVALID`; `internal/gemini`
  reclassifies that as `auth` so the reason names the key variable. Its audio
  arrives as base64 `audio/L16;codec=pcm;rate=24000`, little-endian despite
  the L16 name (Google's examples write it straight into a WAV). A 5xx or an
  answer without audio is retried once: Google documents that the model
  sometimes returns text instead of audio, at random, failing the request
  with a 500. There is no speed control: `speed` is ignored.
- **Local voice discovery filters to English.** It lists the Kokoro voice
  packs in the Hugging Face cache, but only `af_`/`am_`/`bf_`/`bm_`: the
  engine has English G2P only, and its zero-egress sandbox blocks fetching
  another language's, so the other packs are present but cannot speak.
- **The venv shares the legacy state path.** `~/.local/share/speak` holds
  both the engine's venv/logs (recipe-created, on every machine with the
  engine) and, on pre-XDG installs, the playback `audio/` cache and lock.
  That is why `defaultStateDir` probes for `audio/` rather than the
  directory: without the probe every machine with the engine installed would
  be pinned to the legacy path and none could adopt `~/.local/state/speak`.
- **The venv lives at `~/.local/share/speak/venv`,** created by the
  `speak-tts` recipe's install script (see TTS engine gotchas above for the
  pinned versions). Rebuild it with:

  ```bash
  rm -rf ~/.local/share/speak/venv
  ralph up   # recreates the venv, pre-fetches the model, registers both agents
  ```

  Model files (~165 MB Kokoro weights plus ~30 MB voice packs) cache in
  `~/.cache/huggingface` at install time; if that cache is missing or
  incomplete, synthesis fails mid-request with `LocalEntryNotFoundError`.

## See also

- Recipe: `recipes/speak/recipe.toml` (+ `recipes/speak/CLAUDE.md`), which
  builds the binary and registers the `speak-web` and `sandbox-watch` agents
- Component: `webkit/src/read-aloud.ts` (`<wk-read-aloud>`, `webkit/COMPONENTS.md`)
- Route: `speak` → 7425 in the consuming repo's d-man routes overlay (docs/adr/0006)
