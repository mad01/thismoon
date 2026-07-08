# speak

A Go CLI that serves a local read-aloud page at `http://speak.this/` and
reverse-proxies speech synthesis requests to a local Kokoro TTS engine.

Upload a markdown file, read it rendered in the browser, and play it section
by section. The same service also handles `/v1/audio/speech` so other local
tools (present briefings, for example) can request speech without touching the
engine directly.

## Install

```bash
make install   # builds and installs ~/code/bin/speak (adhoc codesigned on macOS)
```

The TTS engine (mlx-audio Kokoro-82M) installs separately via the consuming
repo's `speak-tts` recipe. To set it up from scratch:

```bash
ralph up   # builds speak, creates the venv, pre-fetches the model, registers both agents
```

**The release artifact is the Go binary only.** It serves the page and
proxies speech requests, but synthesis needs the recipe-managed Kokoro
sidecar running on `:8765`; without it the page loads and the speech
endpoints return errors. CI builds and releases never ship the engine.

### TTS engine dependencies

The `speak-tts` recipe's install script creates the venv with pinned
versions:

- `mlx-audio==0.4.3` (0.4.4 ships a broken Kokoro vocoder)
- `mlx==0.31.1`
- `mlx-audio[server]` + `misaki[en]`, both required; the base package lacks
  uvicorn/fastapi and Kokoro imports misaki at request time

Always request `response_format: "wav"`. The default MP3 path calls out to
ffmpeg; if ffmpeg is absent, the engine returns a 200 with an empty body
rather than an error.

## Usage

Two cooperating processes:

| Process | Role | Port |
|---------|------|------|
| `speak serve` | web front-end and TTS proxy | 7425 |
| `python -m mlx_audio.server` | Kokoro TTS engine | 8765 |

Both run as background launchd agents (via t-man). To start them manually:

```bash
speak serve --port 7425 --tts-url http://127.0.0.1:8765
```

```bash
t-man status speak-web     # web front-end agent
t-man status speak-tts     # Kokoro engine agent
t-man restart speak-web    # restart after a binary rebuild
t-man restart speak-tts    # restart after venv changes
t-man logs speak-web       # web server logs
t-man logs speak-tts       # engine logs
```

Open `http://speak.this/` (or `http://localhost:7425/` on hosts without
d-man) to upload a markdown file and play it back section by section.

To verify the full stack:

```bash
curl -i http://127.0.0.1:7425/healthz

curl -sS -X POST http://speak.this/v1/audio/speech \
  -H 'Content-Type: application/json' \
  -d '{"model":"mlx-community/Kokoro-82M-bf16","input":"Hello.","voice":"af_heart","response_format":"wav"}' \
  -o /tmp/t.wav && afplay /tmp/t.wav
```

## Endpoints

| Path | Description |
|------|-------------|
| `GET /` | Upload form |
| `GET /app.js` | Client-side renderer |
| `POST /read` | Render and split a markdown file for playback |
| `POST /v1/audio/speech` | Proxy to the Kokoro engine (adds CORS) |
| `GET /healthz` | 204, CORS'd; reachability probe for this page |
| `GET /enginez` | 204/502; reachability probe for the TTS engine behind the proxy |
| `GET /version` | `{"version":"<sha>"}` build sha |
| `GET /webkit/` | Shared chrome from the in-repo `webkit` package |

The `<wk-read-aloud>` webkit component on the rendered page calls
`POST /v1/audio/speech` locally; present briefings call it cross-origin via
`http://speak.this`.

## Configuration

| Flag | Env | Default |
|------|-----|---------|
| `--port` | `SPEAK_PORT` | `7425` |
| `--tts-url` | `SPEAK_TTS_URL` | `http://127.0.0.1:8765` |

## Where things live

- TTS engine venv: `~/.local/share/speak/venv`, created by the `speak-tts`
  recipe's install script. Rebuild it with:

  ```bash
  # Remove and recreate via the speak-tts recipe hook (runs sandboxed, network open)
  rm -rf ~/.local/share/speak/venv
  ralph up
  ```

- Model cache: `~/.cache/huggingface` (~165 MB Kokoro weights plus ~30 MB
  voice packs, cached at install time). The runtime engine runs offline with
  no network egress; if the model cache is missing or incomplete, synthesis
  fails mid-request with `LocalEntryNotFoundError`.
- No document storage on the server: markdown is rendered per request and
  never written to disk. Recently-read docs are remembered client-side only,
  in the browser's `localStorage`.

## Develop

```bash
make test    # go test ./...
make build   # ./speak
```

See [recipes/speak/CLAUDE.md](../../recipes/speak/CLAUDE.md) for the recipe
and sandbox-watch details.
