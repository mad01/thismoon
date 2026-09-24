# speak, read markdown aloud over localhost

Go CLI serving a local page where you upload a markdown file, see it rendered
inline, and play it section by section with the shared `<wk-read-aloud>` webkit
component, from audio serve synthesizes ahead into a disk cache. The same
process serves an OpenAI-style `/v1/audio/speech` behind a CORS allowlist, so
pages on this machine's other local origins (present.this, localhost) can
fetch speech from it too, and pages from anywhere else are refused.
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
    web/               # server.go (mux, web.Config, cache reaper, the served
                        # wrapper: CORS, cross-site guard, preflight), docs.go
                        # (both forms of POST /read, kept docs, DocStatus,
                        # the /doc and /audio routes), speech.go
                        # (read-through cached speech handler + /enginez
                        # health), cors.go (the cross-origin allowlist),
                        # markdown.go (planBlocks: the part plan both forms
                        # of /read share; PlanSections: goldmark render and
                        # section split around it), assets/shell.html
                        # (chrome-only shell) + assets/app.js (client render,
                        # audio badges)
    chunk/             # text into parts: SplitSentences, Speakable, the Live
                        # and Prepared size ramps; mirrored by
                        # webkit/src/sentences.ts, change both together
    audiocache/        # serve's clip cache: Key, Store (mtime = last use;
                        # Reap: 30 days unused, then a 2 GiB cap), Preparer
                        # (3 lazy workers, urgent queue, 3 attempts with
                        # backoff), Join (one WAV or MP3 from many)
    config/            # the provider config file: a block per provider, the
                        # active one selected; resolves type defaults and keys
                        # from env, marks unusable blocks with a Problem
    provider/          # builds the active provider from config: its client, its
                        # voices (curated, discovered, or catalog), or a broken
                        # provider that fails every synthesis with the reason
    tts/               # shared by every surface: tts.Error (classified failure:
                        # auth/quota/model/network/config/upstream), tts.Health
                        # (ok/degraded/down + reason), tts.Audio + Normalize
                        # (WAV/MP3 pass through, raw PCM gets a WAV header),
                        # WithTimeout/TimedOut (a timeout is upstream, not
                        # network), WaitToRetry/Retried (the one client
                        # retry)
    ttsclient/         # HTTP client for OpenAI-compatible speech endpoints (local
                        # engine, OpenRouter, OpenAI, LiteLLM); every failure
                        # comes back as a *tts.Error
    gemini/            # client for the Gemini API's generateContent speech (key
                        # in x-goog-api-key); same tts.Request/Audio/Error contract
    playback/          # server-side afplay engine: sessions, pause/resume via
                        # SIGSTOP/SIGCONT, flock, 2-part prefetch, md text
                        # extract
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
  (GFM) and splits it into `<section class="doc-section" data-section="N">`
  blocks at every h1/h2; those blocks are the per-button play units of
  `<wk-read-aloud targets=".doc-section" endpoint="">` (empty endpoint = same
  origin). `web.PlanSections` also plans each section's parts with
  `chunk.Prepared` (the first up to 250 characters, then up to 600). Each
  section carries `data-ra-parts` with its part keys in play order, and every
  read block (p, h1-h6, li) carries `data-ra-chunk="<key> ..."`, the parts
  that read it, for highlighting; code, tables, raw HTML and images carry no
  tag and are not read. The component plays the keys from `GET /audio/{key}`.
- **`POST /read` has a JSON form** for a page that renders itself (present):
  `Content-Type: application/json` with `{"name", "sections": [{"blocks":
  ["text", ...]}, ...]}`, the page's text already split into the sections it
  plays by and the blocks that show them. It answers `{"name", "doc",
  "sections": [{"parts": [keys in play order], "blocks": [[keys per block],
  ...]}]}`, the arrays mirroring the request one to one (`[]` for an empty
  section or a block with nothing speakable). Both forms plan through
  `planBlocks`, so a block posted as text gets the key the same block
  rendered from markdown does, and the same texts make the same document
  id. The JSON form keeps the document but queues nothing: a page registers
  on every view and a remote provider bills every part, so playback and
  `POST /doc/{id}/prepare` start synthesis. Its errors (400 for malformed
  JSON, no sections, or a body over 5MB) use the document routes' JSON
  shape; the multipart form's stay plain text.
- **serve pre-synthesizes uploads.** Remote models answer slowly and with the
  whole clip at once (see Gotchas), so serve keeps each upload in memory (the
  32 most recently used docs) and synthesizes its parts in reading order into
  a disk cache, up to 25,000 characters (about 30 minutes of speech); the rest
  waits for Prepare all (`POST /doc/{id}/prepare`). `audiocache.Preparer` runs 3
  workers, started when work is queued, and a part someone is waiting to play
  jumps to an urgent queue. A newer upload's parts go before older queued
  ones, and a doc that falls out of the 32 stops preparing its queued parts.
  The one that falls out is the least recently used with nothing queued,
  generating or retrying, so a page registering text on every view (the JSON
  form of `/read`) never costs an upload its queued parts; only when every
  document has work in flight does the least recently used of them go.
  Parts are synthesized in the provider's default voice at its default speed;
  the browser applies the speed control through `playbackRate`. A failure of
  kind auth, quota, network, config or model, from a background part or a
  played one, returns every background-queued part to idle, so a bad key or a
  rate limit does not cost a request per part. Those halting kinds are never
  retried, and a halt also cancels pending retries. An upstream (or
  unclassified) failure leaves the queue running and gets up to 3 attempts in
  total, 15s then 45s apart; while it waits the part is `retrying` (the
  section's reason is its last failure), then it goes back to the front of
  the background queue. A part whose last attempt also timed out halts the
  background the same way, so a provider that stalls on everything cannot
  cost 3 attempts per part; played parts still run. Playing a retrying part
  runs its next attempt at once; playing a failed part, or a manual re-queue
  (Retry failed, a section's Retry, Prepare all), starts it over with a fresh
  count. The reason reads `failed after N attempts: <reason>`. Each attempt has a 4-minute
  backstop in case the client never returns; the client's own two 90s tries
  end first. After a restart serve has no docs; the page gets a 404 from
  `GET /doc/{id}` and re-posts its markdown, and `GET /audio/{key}` serves a
  key already on disk even when no loaded doc holds it.
- **The cache is `<state-dir>/cache/<key>.wav|.mp3`**, named by
  `audiocache.Key`: 32 hex characters of a sha256 over `provider.ClipID`
  (provider name, model, resolved voice), the speed and the text, so a
  provider, model or voice switch never replays an old clip. A clip's mtime is
  its last use, bumped on every read. Serve reaps at start and daily: first
  clips unused for 30 days, and `.tmp` files a crash left behind by the same
  age, logging `removed N clips unused for 30 days from the audio cache`;
  then, while the cache is over its 2 GiB cap, the least recently used
  clips. Its startup line names the cache directory.
- `POST /v1/audio/speech` synthesizes through the active provider (the
  request's `model` and `response_format` are ignored; a voice the provider
  doesn't offer becomes its default), read-through cached under the same key
  scheme. `internal/web/cors.go` is the allowlist: an `Origin` that is an
  http/https URL on loopback or under `.this` is reflected back with `Vary:
  Origin`. The wrapper handler applies it to every request, so the
  component's cross-origin `GET /` reachability probe works from the sibling
  `.this` pages, and answers every `OPTIONS` preflight itself (204 carrying
  the permission, so a sibling page can post JSON to any route; no route
  handles OPTIONS on its own). It answers 403 to any other `Origin`,
  preflight included, and to a cross-site request without one (`Sec-Fetch-Site:
  cross-site` on anything but a top-level load of `GET /`, such as an `<audio src>` or `<iframe>` on a
  foreign page), so no foreign page can start a paid synthesis. curl and the
  CLI send no `Origin` and pass. **Never widen this to `*`**: the endpoint
  drives the machine's TTS provider, so `*` lets any page the user is
  browsing use it.
- **Failures carry a reason; nothing fails silently.** A failed synthesis is
  answered with `{"error": {"message", "type", "provider", "model",
  "health"}}` (the OpenAI error shape plus provider and health): 502 for a
  provider failure (speak is the gateway), 429 for a rate limit, 503 for a
  config problem. `<wk-read-aloud>` shows the message as a toast. Every
  outcome is recorded in one `tts.Health` per process, and `/enginez` reports
  it. A 200 with no audio, or a stream broken off mid-body (mlx-audio does
  both when it fails after answering), counts as an upstream failure.
  Failures also emit an `error` event to events.this via `kit/notify`
  (fire-and-forget); background preparation failures emit at most one a
  minute, while the page badge and health still show every one. Success is
  recorded but never emitted, since read-aloud fans out one request per part
  and would flood the event log. The document routes and the JSON form of
  `/read` answer errors as `{"error": {"message"}}`; the multipart form's
  `/read` errors stay plain text.
- **Timeouts and the client retry.** One attempt at a remote provider gets
  90s, the local engine 30s. A remote attempt that times out or answers 5xx
  (for Gemini also an answer without audio) is retried once after 500ms; the
  local engine is never retried. Running out of time is kind `upstream` with
  `<provider> did not answer within <limit>` (the limit is the caller's own
  deadline when that is shorter), plus ` (2 attempts)` when both attempts
  timed out: `did not answer within 1m30s (2 attempts)`. `not reachable`
  (kind `network`) means a failed connection. `tts.WaitToRetry` skips the
  retry when the caller's context is done or its deadline cannot fit the
  500ms pause plus another whole attempt, so the 10s doctor and `/enginez`
  probes never retry, even on a fast 5xx.
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
| `POST /read` | Keep a document for the audio routes. Multipart field `doc` (the speak page): render and split a markdown file, start synthesizing its parts, return `{name, content, doc}` (`doc` is a `DocStatus`); errors stay plain text. `application/json` body `{name, sections: [{blocks: [text, ...]}]}` (a page that renders itself, such as present): plan the same keys without synthesizing anything, return `{name, doc, sections: [{parts: [...], blocks: [[...], ...]}]}` aligned with the request; 400 as `{"error": {"message"}}` for malformed JSON, no sections, or a body over 5MB |
| `GET /doc/{id}` | `DocStatus`: part counts (`parts`, `ready`, `generating`, `queued`, `retrying`, `idle`, `failed`) for the doc and per section, plus the latest failure `reason` with its attempt count (`failed after 3 attempts: ...`). 404 when serve no longer keeps the doc |
| `POST /doc/{id}/prepare[?section=N][&failed=1]` | Queue every idle or failed part of the doc, or of section N (1-based; 400 for a bad number), failed ones with a fresh attempt count; `failed=1` queues only the failed parts. Answers `DocStatus` |
| `GET /doc/{id}/audio[?section=N]` | The doc, or section N (the page links only the whole doc; the section form is for scripts), as one file (WAV parts joined, MP3 appended) with `Content-Disposition: attachment` (`notes.wav`, `notes-section-2.wav`). 409 naming how many parts are ready, 404 when nothing there is read aloud, 400 for a bad section |
| `GET /audio/{key}` | One part's clip. A part not ready yet moves to the front of the queue and the request waits for it (tens of seconds on a remote provider); a synthesis failure answers like `/v1/audio/speech`. A key already on disk is served even when no loaded doc holds it. 400 malformed key, 404 unknown key |
| `POST /v1/audio/speech` | OpenAI-style speech through the active provider, read-through cached (key: provider, model, resolved voice, speed, text); answers WAV or MP3 (reflects an allowlisted origin; like every route, 403 for any other origin or a cross-site request without one). A failure answers JSON `{"error": {"message", "type", "provider", "model", "health"}}`: 502 for a provider failure, 429 rate limit, 503 config problem, 400 for a request without `input` |
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
- Recent-docs chips, drag/drop/paste handling, and the audio state are
  speak-local logic in `app.js`, not webkit components. The audio state is a
  `<wk-badge>` per section (amber `retrying, n of m ready` while an automatic
  retry waits, its title naming the last failure) with a Retry button beside
  it when the section has failed parts (`POST
  /doc/{id}/prepare?section=N&failed=1`); the page's `Audio: n of m parts
  ready` line with its retrying and failed counts, a `Retry failed (n)`
  button (`?failed=1`) when anything failed or `Prepare all` (idle and
  failed parts) when parts are only unprepared, and `Download page audio`
  once every part is ready; and a 2s
  `GET /doc/{id}` poll while anything is queued, generating or retrying.
  There is no per-section download link.

### Version check

`GET /webkit/version` confirms which embedded webkit assets the running
`speak` server serves.

## MCP tools

`speak mcp` is a second, independent surface from `speak serve`. serve plays
audio **in the browser** (the `<wk-read-aloud>` component fetches each part's
clip and plays it there); mcp plays audio **on the machine's speakers** via
`afplay`, so an agent can make the host talk. They share only the TTS engine;
mcp does not use serve's cache.

- `speak_text`: speak a text string on the host speakers.
- `speak_file`: read a file (text or markdown) aloud.
- `speak_pause` / `speak_resume` / `speak_stop`: control the running playback
  session; stop saves the part index so a later resume restarts there.
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
- `internal/playback` runs a worker goroutine over a list of parts:
  `chunk.Live` groups the sentences (one sentence, then up to 250
  characters, then up to 600), so the first sound waits on one short request.
  speak_file ramps only the first section with content, later sections start
  at 600, and no part spans two sections. The worker keeps the next 2 parts
  synthesizing through the provider while one plays, writes each clip under
  the state directory's `audio/`, and `afplay`s it. A prefetched part that
  failed ends the session when playback reaches it. Pause = `SIGSTOP` the
  afplay child + release the lock; resume = re-acquire the lock + `SIGCONT`.
  Stop cancels the session's syntheses in flight instead of waiting for
  them.
- **One session at a time, cross-process.** An `flock` on `playback.lock` in
  the state directory (with a `.owner` sidecar naming the holder) means a
  second `speak mcp` gets a `BUSY | …` reply. Old WAVs are reaped after 24h
  on start. The engine takes the directory from `--state-dir`, so two
  processes only serialize against each other when they resolve the same one
  — which is why the flag is a persistent root flag, not a per-command one.
- **Go `regexp` has no lookbehind** — the sentence splitter
  (`chunk.SplitSentences`) is hand-rolled, not a translation of the Python
  `re.split(r'(?<=[.!?])\s+')`.
- **The `voice` parameter is the provider's voice list.** `registerTools`
  builds the speak_text/speak_file input schemas from the struct, then pins
  `voice` to an enum of the provider's voices (resolved once at startup:
  discovery runs then, with a 3s budget for OpenRouter). With no known list
  it stays free text. `OpenWorldHint` is true when the provider is remote,
  since text then leaves the machine.
- **speak_text and speak_file synthesize the first part before
  replying.** A backend that cannot speak comes back as `Failed` with an
  `UNAVAILABLE | TTS <health>` message, flagged `isError` over MCP, instead
  of a "Playing" reply that stays silent. The queue is kept as a stopped
  session, so `speak_resume` retries it. A `speak_stop` during that first
  synthesis cancels it and replies `Stopped before the first part was
  ready.` `startMu` serializes starts and stops: a start holds the playback
  lock while it synthesizes, before `running` is set, so an unserialized
  second start would see "not running" and release that lock mid-synthesis.
  Stop cancels the session's context before it takes `startMu`, so it never
  waits out a slow first synthesis.
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
- **Remote speech models do not stream: time to first sound is the whole
  synthesis.** Gemini 3.1 Flash TTS (preview) through OpenRouter took 3.2s for
  a 7-word sentence, 8.6 to 14.3s for 34 words and 21.6s for 120 words, each
  answered as one clip; three parallel requests ran without slowing down.
  That is why serve prepares uploads ahead (a 4-part document took 18s, then
  each part came from the cache in about a millisecond), why live playback
  starts on a one-sentence part and keeps 2 parts in flight. Healthy parts
  of up to about 700 characters take 15 to 22s, with tails near 45s, but a
  request also stalls now and then at random: one 323-character part ran
  past 2 minutes in production, then took 14.0, 15.2, 15.7 and 44.8s in four
  re-timings. So a remote attempt gets 90s, about twice the slow tail, plus
  one retry, rather than waiting a stall out. `chunk.MaxChars` (600) keeps a
  part well inside that limit.
- **The Gemini API answers a bad key with 400, not 401.** Its error body
  carries `INVALID_ARGUMENT` with reason `API_KEY_INVALID`; `internal/gemini`
  reclassifies that as `auth` so the reason names the key variable. Its audio
  arrives as base64 `audio/L16;codec=pcm;rate=24000`, little-endian despite
  the L16 name (Google's examples write it straight into a WAV). A 5xx, an
  answer without audio, or a timeout is retried once: Google documents that
  the model sometimes returns text instead of audio, at random, failing the
  request with a 500. There is no speed control: `speed` is ignored.
- **Local voice discovery filters to English.** It lists the Kokoro voice
  packs in the Hugging Face cache, but only `af_`/`am_`/`bf_`/`bm_`: the
  engine has English G2P only, and its zero-egress sandbox blocks fetching
  another language's, so the other packs are present but cannot speak.
- **The venv shares the legacy state path.** `~/.local/share/speak` holds
  both the engine's venv/logs (recipe-created, on every machine with the
  engine) and, on pre-XDG installs, the playback `audio/` cache and lock
  (and serve's `cache/`).
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
