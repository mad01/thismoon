# speak configuration

speak's settings come from flags, environment variables and one optional
config file that picks the text-to-speech (TTS) provider. Everything is
resolved once at process start.

## Where config lives

The config file is `~/.config/speak/config.yaml` (`$XDG_CONFIG_HOME/speak/`
when that variable holds an absolute path). `--config` or `SPEAK_CONFIG`
points elsewhere. Without a file, speak uses one implicit `local` provider:
the Kokoro engine on this machine, which is how speak behaved before the
file existed.

Every setting is a root flag, so `speak serve`, `speak mcp` and `speak
doctor` resolve the same provider, port and state directory. Precedence is
flag, then environment variable, then config file, then built-in default.
Both long-running processes log the resolved provider to stderr at startup.

`speak config` prints what was resolved: the file read, the active provider,
and every configured block with the problem that keeps it from being used.
`speak config active -o json` prints the active block alone. API keys are
never printed.

## The config file

The file holds a block per provider, all configured side by side, and a
top-level `provider` line naming the one in use. Switching providers is a
one-line edit, or `--provider` / `SPEAK_PROVIDER` for a single run.

```yaml
provider: openrouter-gemini

providers:
  local: {}

  openrouter:
    voice: af_heart
    voices: [af_heart, af_bella, am_michael]

  openrouter-gemini:
    type: openrouter
    model: google/gemini-3.1-flash-tts-preview
    voice: Kore

  proxy:
    type: litellm
    model: tts-1

  gemini:
    voice: Puck
```

Block fields, all optional:

- `type`: the provider type (below). Defaults to the block's name, so a
  block called `openrouter` needs no `type`. Name blocks freely to keep two
  setups of one type, such as two OpenRouter models.
- `base_url`: the endpoint root, without `/v1` (a trailing `/v1` is trimmed).
- `base_url_env`: an environment variable holding the base URL instead.
- `api_key_env`: the environment variable holding the API key.
- `model`: the model id.
- `voice`: the default voice.
- `voices`: the voices to offer. When set, the list is exactly these, and
  `voice` joins it if missing; without `voice`, the first entry is the
  default. Omit it to have speak discover the provider's voices.

A provider named after a type needs no block at all: `--provider openrouter`
works with nothing but `OPENROUTER_API_KEY` set.

## Provider types

- `local`: the Kokoro engine on this machine (mlx-audio, the speak-tts
  agent). Defaults: base URL `http://127.0.0.1:8765`, model
  `mlx-community/Kokoro-82M-bf16`, voice `af_heart`, no key. `--tts-url`
  or `SPEAK_TTS_URL` overrides the base URL of every local block.
- `openrouter`: OpenRouter's speech endpoint. Defaults: base URL
  `https://openrouter.ai/api`, key from `OPENROUTER_API_KEY` (the same
  variable humanizer reads), model `hexgrad/kokoro-82m`, voice `af_heart`.
  Audio is requested as pcm, the only format besides mp3 it offers.
- `openai`: OpenAI's speech endpoint. Defaults: base URL
  `https://api.openai.com`, key from `OPENAI_API_KEY`, model
  `gpt-4o-mini-tts`, voice `alloy`.
- `litellm`: a LiteLLM proxy, routing to whatever backend it is configured
  for. Base URL from `LITELLM_BASE_URL` and an optional key from
  `LITELLM_API_KEY`, the variables humanizer reads; `model` is required
  (the name the proxy routes speech to); voice defaults to `alloy`.
- `gemini`: the Gemini Developer API, called directly rather than through
  OpenRouter. Defaults: base URL `https://generativelanguage.googleapis.com`,
  key from `GEMINI_API_KEY`, else `GOOGLE_API_KEY` (the order Google's own
  SDKs use), model `gemini-3.1-flash-tts-preview`, voice `Kore`. The key
  goes in the `x-goog-api-key` header, never the URL. The API has no speed
  setting, so a requested speed is ignored. A server error, or an answer
  without audio, is retried once: Google notes its speech models sometimes
  return text instead of audio, at random, and says to retry.

Every type except `gemini` speaks the OpenAI-style `POST
{base}/v1/audio/speech`; `gemini` uses the Gemini API's `generateContent`
with audio output. Whatever a
provider returns (WAV, MP3, or raw PCM) is normalized to WAV or MP3 before
it reaches the browser or afplay. A provider whose base URL is not loopback
is remote: the text being read leaves this machine. There is no automatic
fallback from one provider to another.

A request to a remote provider may take up to 2 minutes, since remote speech
models answer with the whole clip at once and can take tens of seconds for a
long part; the local engine gets 30 seconds. Neither limit is configurable.

## Voices

Each provider offers its own voices, resolved when serve or mcp starts:

1. The block's `voices` list, when set.
2. Otherwise discovered: the local engine's English voice packs in the
   Hugging Face cache (`HF_HUB_CACHE`, else `$HF_HOME/hub`, else
   `~/.cache/huggingface/hub`), or OpenRouter's public models list for the
   configured model. Only American and British English Kokoro voices count
   locally: the engine has English G2P only, and its sandbox blocks fetching
   another language's.
3. Otherwise a built-in catalog: Kokoro's English voices for a Kokoro model,
   OpenAI's voice set for `openai`, and the 30 Gemini voices for `gemini`
   (the Gemini API does not list them).
4. Otherwise nothing beyond the default voice. LiteLLM does not list voices,
   so set `voices` on a LiteLLM block to offer more than one.

`speak_voices` lists the result with the default marked and says which of
these it came from. The MCP tools' `voice` parameter offers exactly that
list. A voice the list does not contain (the read-aloud component sends the
Kokoro default to every provider) becomes the provider's default.

## When the config is broken

A file that exists but cannot be parsed, has an unknown field (a key pasted
in as `api_key`, say), or selects a provider with no block is not ignored.
speak starts anyway, logs the problem to stderr, and fails every synthesis
with that reason as a `config` error: the web page's banner, the speech
endpoint, the MCP tools and `speak doctor` all show it. The same goes for a
block that cannot be used, such as one whose key variable is unset. speak
never falls back to another provider quietly.

## Flags

Every flag is a persistent flag on the root command and applies to all
subcommands.

- `--config` (string; env `SPEAK_CONFIG`): the config file. Default above.
- `--provider` (string; env `SPEAK_PROVIDER`): the block to use, overriding
  the file's `provider` line.
- `--tts-url` (string; env `SPEAK_TTS_URL`): base URL of the local engine,
  overriding every local block's `base_url` (default
  `http://127.0.0.1:8765`).
- `--port` (int, default `7425`; env `SPEAK_PORT`): port `speak serve`
  listens on, and the port `speak doctor` and the `speak_doctor` MCP tool
  probe for the web surface.
- `--state-dir` (string; env `SPEAK_STATE_DIR`): directory holding speak's
  state. `speak mcp` writes per-part audio files under `audio/` and the
  cross-process `playback.lock`; `speak serve` keeps its audio cache under
  `cache/`, deleting clips unused for 30 days and then the least recently
  used while the cache is over 2 GiB.

### Where `--state-dir` defaults to

1. `SPEAK_STATE_DIR`, when set.
2. `~/.local/share/speak`, when it holds an `audio/` directory, so an install
   that has actually played something keeps its audio and lock across
   upgrades. `audio/` is what settles it, not the parent directory:
   the mlx-audio engine installs its virtualenv and logs under the same path,
   so on a machine with the TTS engine but no playback history the directory
   exists while the state does not, and that machine lands in (3).
3. `$XDG_STATE_HOME/speak` when `XDG_STATE_HOME` holds an absolute path,
   otherwise `~/.local/state/speak`. This is where a fresh install lands.

A leading `~` in `--state-dir` or `--config` is expanded before any
subcommand runs, so a value that reaches the process unexpanded (from launchd,
say) still names the right path. A home directory that cannot be resolved is
an error, never a path relative to the working directory.

## Environment variables

- `SPEAK_CONFIG`, `SPEAK_PROVIDER`, `SPEAK_TTS_URL`, `SPEAK_PORT`,
  `SPEAK_STATE_DIR`: defaults for the flags above. An unparseable
  `SPEAK_PORT` warns on stderr and the built-in default is used.
- `OPENROUTER_API_KEY`, `OPENAI_API_KEY`, `LITELLM_BASE_URL`,
  `LITELLM_API_KEY`, `GEMINI_API_KEY`, `GOOGLE_API_KEY`: what the provider
  types read by default. A block's
  `api_key_env` and `base_url_env` name other variables. Keys are read only
  from the environment, never from the file or a flag.
- `HF_HUB_CACHE`, `HF_HOME`: where local voice discovery looks.
- `EVENTS_BASE_URL`: where synthesis failures are archived (default
  `http://127.0.0.1:7430`). Emitting is best effort: an unreachable events
  service drops the event and never affects a response.

## Keys for agents and MCP hosts

Neither the speak-web launchd agent nor an MCP host carries a login shell's
environment. The speak recipe's `speak-env.sh` wrapper bridges that: it runs
`speak config env` (the names of the variables the active provider reads,
nothing for the local engine), pulls exactly those from the ralph-managed
secrets file (`~/.config/ralph/secrets.sh`, legacy `~/.secrets.sh`), and
execs speak with its arguments. speak-web starts through it; point an MCP
registration at it as well. Pick the provider with the file or
`SPEAK_PROVIDER`, not a `--provider` flag after the subcommand, which the
wrapper's query would not see.

## Cross-origin access

`speak serve` answers cross-origin requests only from this machine's own
pages, so an arbitrary page the browser happens to have open cannot start a
synthesis, which on a remote provider costs money. An `Origin` header is
allowed when it is an `http`/`https` URL whose host is loopback
(`localhost`, `127.0.0.0/8`, `::1`, on any port) or ends in `.this`. An
allowed origin is reflected back in `Access-Control-Allow-Origin` alongside
`Vary: Origin`; any other origin gets 403 with no CORS headers, on every
route including the `OPTIONS` preflight. A request with no `Origin` header
is refused the same way when the browser marks it cross-site
(`Sec-Fetch-Site: cross-site`) and it is not a top-level load of the page
(`GET /`), such as a foreign page's `<audio>` or `<iframe>` pointing at an
audio URL. Same-origin fetches, following a
link to the page, curl and the CLI are unaffected.

The allowlist is fixed: no flag or environment variable widens it.

## Example

```bash
speak config                                   # what is resolved, and why not
SPEAK_PROVIDER=openrouter speak doctor         # try another block for one run
speak serve --port 7425                        # the provider the file selects
~/.config/ralph/sources/thismoon/recipes/speak/speak-env.sh mcp
```
