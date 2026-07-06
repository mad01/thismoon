# speak — read markdown aloud over localhost

Go CLI serving a local page where you upload a markdown file, see it rendered
inline, and play it section by section with the shared `<wk-read-aloud>` webkit
component. The same process reverse-proxies `/v1/audio/speech` to the local
Kokoro TTS engine (mlx-audio) with CORS headers, so pages on other local
origins (present.this) can fetch speech from `http://speak.this` too.

## Module layout

```
services/speak/
  cmd/speak/           — entrypoint (delegates to internal/cli)
  internal/
    cli/               — cobra: serve, version (Version via ldflags)
    web/               — server.go (mux, CORS, TTS proxy), markdown.go
                         (goldmark render + section split), assets/shell.html
                         (chrome-only shell) + assets/app.js (client render)
  Makefile
```

Part of the `github.com/mad01/thismoon` module — no nested go.mod.

## How it works

- `speak serve --port 7425 --tts-url http://127.0.0.1:8765` (envs `SPEAK_PORT`,
  `SPEAK_TTS_URL`).
- `GET /` — upload form. `POST /read` (multipart field `doc`, max 5MB) renders
  markdown via goldmark (GFM) and splits it into
  `<section class="doc-section">` blocks at every h1/h2 — those are the
  per-button play units of `<wk-read-aloud targets=".doc-section" endpoint="">`
  (empty endpoint = same origin). No storage; render per request.
- `POST /v1/audio/speech` — reverse proxy to the mlx-audio engine, adding
  `Access-Control-Allow-Origin: *` and answering `OPTIONS` preflight locally
  (the engine doesn't do CORS). Every response carries the CORS header so the
  component's cross-origin `GET /` reachability probe works.
- Chrome comes from the in-module webkit package (`GET /webkit/`) — the
  service compiles against the webkit committed beside it, no pin to bump.

## TTS engine (the other half)

mlx-audio runs as the separate t-man agent `speak-tts` from the venv at
`~/.local/share/speak/venv`, created by the consuming repo's `speak-tts`
recipe (machine wiring — it stays out of this repo, see `docs/adr/0006`).
Gotchas that cost time once:

- **Pin `mlx-audio==0.4.3` + `mlx==0.31.1`.** 0.4.4's Kokoro vocoder is broken
  (`[broadcast_shapes]` ValueError in istftnet on every input).
- **`mlx-audio[server]` + `misaki[en]` are both required** — the bare package
  is missing uvicorn/fastapi, and Kokoro imports misaki at request time.
- **Request `response_format: "wav"`.** The default mp3 path shells out to
  ffmpeg, which may not be installed; failures surface as a 200 with an empty
  streamed body, not an error status.
- **misaki pulls `en_core_web_sm` via `uv pip install` on first G2P** — that
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
make build    # ./speak binary
make install  # build + cp to ~/code/bin/speak + adhoc codesign
make test     # go test ./...
```

## See also

- Recipe: `recipes/speak/recipe.toml` (+ `recipes/speak/CLAUDE.md`) — builds
  the binary, registers the `speak-web` and `sandbox-watch` agents
- Component: `webkit/src/read-aloud.ts` (`<wk-read-aloud>`, COMPONENTS.md)
- Route: `speak` → 7425 in the consuming repo's d-man routes overlay
- Import provenance: `docs/MIGRATED-FROM.md`
