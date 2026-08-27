# speak, read markdown aloud over localhost

Go CLI serving a local page where you upload a markdown file, see it rendered
inline, and play it section by section with the shared `<wk-read-aloud>` webkit
component. The same process reverse-proxies `/v1/audio/speech` to the local
Kokoro TTS engine (mlx-audio) with CORS headers, so pages on other local
origins (present.this) can fetch speech from `http://speak.this` too.

## Module layout

```
services/speak/
  cmd/speak/           # entrypoint (delegates to internal/cli)
  internal/
    cli/               # cobra: serve, mcp, docs, version (build metadata from the shared buildinfo package)
    web/               # server.go (mux, CORS, TTS proxy, HTTP API), markdown.go
                        # (goldmark render + section split), assets/shell.html
                        # (chrome-only shell) + assets/app.js (client render)
    ttsclient/         # HTTP client for the Kokoro engine (WAV over /v1/audio/speech)
    playback/          # server-side afplay engine: sessions, pause/resume via
                        # SIGSTOP/SIGCONT, flock, sentence split, md text extract
    mcpserver/         # go-sdk MCP server: 7 speak_* tools over the playback engine
    notify/            # events.go (best-effort EmitEvent to events.this on
                        # TTS proxy failures)
  Makefile             # part of module github.com/mad01/thismoon (no own go.mod)
```

## How it works

- `speak serve --port 7425 --tts-url http://127.0.0.1:8765` (envs `SPEAK_PORT`,
  `SPEAK_TTS_URL`).
- `POST /read` (multipart field `doc`, max 5MB) renders markdown via goldmark
  (GFM) and splits it into `<section class="doc-section">` blocks at every
  h1/h2; those blocks are the per-button play units of
  `<wk-read-aloud targets=".doc-section" endpoint="">` (empty endpoint = same
  origin). No storage; render per request.
- `POST /v1/audio/speech` reverse-proxies to the mlx-audio engine, adding
  `Access-Control-Allow-Origin: *` and answering `OPTIONS` preflight locally
  (the engine doesn't do CORS). Every response carries the CORS header so the
  component's cross-origin `GET /` reachability probe works.
- When the proxied request fails or the engine answers with a 4xx/5xx, the
  server emits an `error` event to events.this via `internal/notify`: a
  fire-and-forget POST that never blocks the response. Successful synthesis is
  intentionally not logged, since read-aloud fans out one request per sentence
  and would flood the event log.
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
- **Request `response_format: "wav"`.** The default mp3 path shells out to
  ffmpeg, which may not be installed; failures surface as a 200 with an empty
  streamed body, not an error status.
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
| `POST /v1/audio/speech` | Reverse proxy to the Kokoro engine (adds CORS, strips the upstream's own CORS headers) |
| `GET /healthz` | CORS'd 204; the `<wk-read-aloud>` component's cross-origin reachability probe against `GET /` gets the same header from the wrapper handler, which sets CORS on every response |
| `GET /enginez` | Pings the TTS engine's `GET /` with a 1.5s timeout; 204 if reachable, 502 otherwise. `app.js` polls this to warn when play buttons won't work; distinct from `/healthz`, which only proves this page is up |
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
- `speak_voices`: list the engine's available voices.
- `speak_status`: report the playback state and current session.

Names and behaviour are ported from the Python `speak_mcp.py` server so agent
muscle memory carries over. Implementation notes:

- **Playback lives in the mcp process**, not in serve. `speak mcp` does not
  require `speak serve` to be running — it only needs the `speak-tts` engine
  reachable on `--tts-url`. This is deliberately unlike reminder's MCP (a thin
  client to its serve): the shared resource here is the audio device,
  serialised by an `flock`, not a JSON store, so there is no single-writer
  file to funnel through.
- `internal/playback` runs a worker goroutine over the sentence list: fetch WAV
  from `internal/ttsclient`, write it under `~/.local/share/speak/audio/`,
  `afplay` it, `Wait`. Pause = `SIGSTOP` the afplay child + release the lock;
  resume = re-acquire the lock + `SIGCONT`.
- **One session at a time, cross-process.** An `flock` on
  `~/.local/share/speak/playback.lock` (with a `.owner` sidecar naming the
  holder) means a second `speak mcp` gets a `BUSY | …` reply. Old WAVs are
  reaped after 24h on start.
- **Go `regexp` has no lookbehind** — the sentence splitter
  (`playback.SplitSentences`) is hand-rolled, not a translation of the Python
  `re.split(r'(?<=[.!?])\s+')`.
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
  binary only. It serves the page and proxies speech requests, but synthesis
  needs the recipe-managed Kokoro sidecar running on `:8765`; without it the
  page loads and the speech endpoints return errors. CI builds and releases
  never ship the engine.
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
- Import provenance: `docs/MIGRATED-FROM.md`
