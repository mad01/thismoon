# speak

A Go CLI that turns text into speech for this machine through a configurable
text-to-speech (TTS) provider: a local Kokoro engine by default, or
OpenRouter, OpenAI, a LiteLLM proxy or the Gemini Developer API. `speak
serve` at `http://speak.this/` is the audio service present pages read aloud
through; `speak mcp` reads on the speakers for agents.

Documents are read on present. A present page registers its text with speak
on load; speak synthesizes the parts into a disk cache when the page asks
(Prepare all) or plays them, section by section, retrying a part the
provider fails on, and a replay starts from ready audio; once every part is
ready the whole page can be downloaded as one audio file. The same service handles `/v1/audio/speech` so other local
tools can request speech without touching the provider directly. speak's own
page is a landing page: the engine state and the routes.

## Providers

`~/.config/speak/config.yaml` holds a block per provider, all configured side
by side, and a `provider:` line picking the one in use. Without the file speak
uses the local engine. Keys come only from environment variables; `speak
config` shows what is resolved and why a block cannot be used. For example,
to read aloud with Gemini voices through OpenRouter:

```yaml
provider: openrouter-gemini
providers:
  local: {}
  openrouter-gemini:
    type: openrouter          # key from OPENROUTER_API_KEY
    model: google/gemini-3.1-flash-tts-preview
    voice: Kore
```

Full format, voices and precedence: [config.md](config.md).

## Install

```bash
make install   # builds and installs ~/code/bin/speak (adhoc codesigned on macOS)
```

The TTS engine (mlx-audio Kokoro-82M) installs separately via the consuming
repo's `speak-tts` recipe. To set it up from scratch:

```bash
ralph up   # builds speak, creates the venv, pre-fetches the model, registers both agents
```

**The release artifact is the Go binary only.** With the default local
provider, synthesis needs the recipe-managed Kokoro sidecar running on
`:8765`; without it the page loads and the speech endpoints return errors. CI
builds and releases never ship the engine. A machine on a remote provider
needs no engine.

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
| `speak serve` | document audio API and TTS proxy | 7425 |
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
d-man) to see the engine state and the routes; open a page on
`http://present.this/` to read it aloud.

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
| `GET /` | Landing page: the engine state (fetched from `/enginez`) and the routes; documents are read on present |
| `POST /read` | Register a page's text, split into sections and blocks, as a document; answers the part keys the page plays by and synthesizes nothing until play or prepare. JSON only, 415 otherwise |
| `GET /doc/{id}` | Audio state of a registered document: parts ready, in progress, retrying, not prepared, failed |
| `POST /doc/{id}/prepare[?section=N][&failed=1]` | Synthesize every part not ready yet, in the document or in section N; `failed=1` retries only the failed ones |
| `GET /doc/{id}/audio[?section=N]` | The document, or one section, as one audio file once every part is ready |
| `GET /audio/{key}` | One part's audio, synthesized first if it is not ready |
| `POST /v1/audio/speech` | OpenAI-style speech through the active provider, cached on disk (CORS allowlist: loopback and `.this` origins); failures answer JSON naming the reason |
| `GET /healthz` | 204; reachability probe for serve |
| `GET /enginez` | TTS health as JSON (ok, degraded or down, with the reason); 200 when ok, 503 otherwise |
| `GET /version` | Build metadata: `version`, `commit`, `tag`, `build_time` |
| `GET /webkit/` | Shared chrome from the in-repo `webkit` package |

The `<wk-read-aloud>` webkit component on a present page registers the page
through `POST /read` and plays the prepared parts from `GET /audio/{key}`,
cross-origin via `http://speak.this`; text selections and pages without
prepared parts call `POST /v1/audio/speech`.

## MCP server

`speak serve` plays audio in the browser: the person at the page hears it.
`speak mcp` plays audio on the machine's speakers with `afplay`, so an agent
can make the machine talk. The two are independent: they share only the TTS
provider, and the MCP server needs that provider reachable but does **not**
need `speak serve` running. The `voice` parameter of `speak_text` and
`speak_file` offers the provider's voices.

```bash
speak mcp   # stdio MCP server for Claude Code, on the configured provider
```

A remote provider's key has to reach the server's environment, which an MCP
host does not carry from your shell. The speak recipe's `speak-env.sh` pulls
exactly the variables the active provider reads from the ralph secrets file
and execs speak; register that script with the argument `mcp` instead of the
bare binary.

On a standalone install, register it once:

```bash
claude mcp add --scope user speak -- speak mcp
```

On a ralph-managed machine, skip the manual command above. Registering the server with a client is machine-private wiring that ships from the consuming repo's companion recipe (see [docs/adr/0006](../../docs/adr/0006-recipe-layering-and-platform-deps.md)).

Only one server-side session plays at a time; playback is serialised across
processes by an `flock` on `playback.lock` in the state directory, so a second
caller gets a `BUSY | …` reply instead of talking over the first.

| Tool | What it does |
|------|--------------|
| `speak_text` | Speak given text aloud; returns a session id. |
| `speak_file` | Read a markdown file aloud, section by section (`sections` = comma-separated 1-based indices, empty = all). |
| `speak_pause` | Pause playback (`SIGSTOP` the `afplay` child) and release the lock. |
| `speak_resume` | Resume after pause, or restart from the saved position after stop. |
| `speak_stop` | Stop playback; the position is saved for `speak_resume`. |
| `speak_voices` | List the active provider's voices, default marked. |
| `speak_status` | Report provider health and playback state (session, playing/paused/stopped/idle, position, lock holder). |
| `speak_doctor` | Run the same checks as `speak doctor` and return the report as JSON. |

Confirm the registration with `claude mcp list`, and run `speak doctor` to check the config, the providers, synthesis, the store, the web surface, and version skew in one pass.

## Configuration

All three are root flags: `serve`, `mcp`, and `doctor` resolve them the same way.

| Flag | Env | Default |
|------|-----|---------|
| `--port` | `SPEAK_PORT` | `7425` |
| `--tts-url` | `SPEAK_TTS_URL` | `http://127.0.0.1:8765` |
| `--state-dir` | `SPEAK_STATE_DIR` | `~/.local/share/speak` where it holds `audio/`, else `~/.local/state/speak` |

`speak serve` answers cross-origin requests only from this machine's own
pages: an `Origin` on loopback or under `.this` is reflected back, and any
other origin, or a cross-site request that sends none, gets 403, so a
foreign page cannot start a synthesis. curl and the CLI are unaffected. See
[config.md](config.md) for the full surface.

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
- Audio cache: `<state-dir>/cache/`, one file per synthesized part, capped
  at 2 GiB. When `speak serve` starts and once a day after, it deletes files
  unused for 30 days, then the least recently used until the cache fits.
- No text on disk: `speak serve` keeps the 32 most recently registered
  documents in memory; a present page registers again after a restart.

## Develop

```bash
make test    # go test ./...
make build   # ./speak
```

See [recipes/speak/CLAUDE.md](../../recipes/speak/CLAUDE.md) for the recipe
and sandbox-watch details.

## Docs

- [architecture](architecture.md): internal structure and data flow
- [operating](operating.md): runtime behavior, failure modes, first moves
- [why](why.md): why this component exists
- [config](config.md): configuration reference
