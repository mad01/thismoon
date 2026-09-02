# humanizer configuration

## Where config lives

humanizer has no config file. It's a short-lived CLI/MCP process that reads
no persisted settings: configuration is command-line flags, scoped one
subcommand at a time, plus a handful of environment variables. The only
on-disk state is the extracted vale style pack cache, whose location is the
one setting that isn't a per-command flag.

The `vale` binary itself is resolved from `$PATH` (Go's `exec.LookPath`).
The underlying `rules.DetectOptions` struct has a `ValeBinary` field that
would override this, but neither the CLI nor the MCP server exposes it:
there is no flag or environment variable to point humanizer at a `vale`
binary that isn't on `PATH`.

## Cache directory

Where the embedded Humanizer vale style pack extracts to, resolved in this
order:

- `HUMANIZER_CACHE_DIR` (path expanded if it starts with `~`), used
  directly as the cache root.
- `XDG_CACHE_HOME` (path expanded), joined as `<value>/humanizer/vale`.
- Default: `~/.cache/humanizer/vale`.

The directory is a regenerable cache: a run rewrites any file that drifted
from the embedded copy, so deleting it is always safe.

## Flags

### `detect`

- `--min-severity` (string, default `""`): filter findings below this level
  (`suggestion`, `warning`, `error`).
- `--rule` (string list, default none, repeatable): restrict detection to
  these rule IDs, e.g. `--rule Humanizer.AIVocabulary`.
- `--statistical` (bool, default `false`): run the statistical detector
  (sentence uniformity, contraction rate, TTR, anaphora) instead of the vale
  span rules.
- `--json` (bool, default `false`): emit findings as JSON, the same shape
  `humanizer_detect` returns over MCP.

### `judge`

Each flag falls back to an environment variable when unset, listed with it
below. `judge` is the one networked command: it needs a provider configured
through the environment (see Environment variables) and fails without one.

- `--backend` (string, default `""`, env `HUMANIZER_BACKEND`): force an LLM
  backend, `litellm` or `openrouter`. Unset auto-detects: a set
  `LITELLM_BASE_URL` selects `litellm`, else a set `OPENROUTER_API_KEY`
  selects `openrouter`.
- `--model` (string, default `""`, env `HUMANIZER_MODEL`): override the
  backend's default model id (`claude-haiku-4-5-20251001` for `litellm`,
  `anthropic/claude-haiku-4.5` for `openrouter`).
- `--json` (bool, default `false`): emit the verdict as JSON, the same shape
  `humanizer_judge` returns over MCP.

### `scan`

- `--go` (bool, default `false`): extract from Go source. Required today:
  it selects the only extractor there is.
- `--detect` (bool, default `false`): run the vale span rules over the
  extracted prose and report findings against their Go source lines.
- `--holistic` (bool, default `false`): send each file's prose to the LLM
  judge. With no backend configured it prints a note on stderr and the scan
  continues.
- `--kind` (string list, default none, repeatable): restrict extraction to
  these kinds (`doc`, `cobra`, `mcp`, `error`, `schema`).

### `profile`

- `--diff` (string, default `""`): path to a sample file; also prints a
  metric-by-metric delta against it.
- `--json` (bool, default `false`): emit the profile (and diff, if
  requested) as JSON.

### `rules list`

- `--category` (string, default `""`): filter by category (`content`,
  `language`, `style`, `communication`).
- `--json` (bool, default `false`): emit the rule list as JSON.

### `lint`

- `--aggressive` (bool, default `false`): also flag Cyrillic/fullwidth Latin
  confusable lookalikes.
- `--strip-emoji-glue` (bool, default `false`): paranoid mode, also flags
  load-bearing invisibles (emoji glue, script joiners, flag tags,
  orthographic Cf) that are normally preserved.
- `--json` (bool, default `false`): emit the report as JSON.

### `fix`

- `--nfkc` (bool, default `false`): apply Unicode NFKC normalization after
  the scrub. Risky — alters visible characters.
- `--aggressive-homoglyphs` (bool, default `false`): map Cyrillic/fullwidth
  Latin confusables to ASCII. Risky.
- `--no-normalize-spaces` (bool, default `false`): skip rewriting exotic
  spaces to `U+0020` (the one default-on transform).
- `--strip-emoji-glue` (bool, default `false`): also strip load-bearing
  invisibles. Risky.
- `-o`, `--output` (string, default `""`): write cleaned text here instead
  of stdout.
- `--in-place` (bool, default `false`): overwrite the input file (writes a
  `.bak` backup first). Requires a file argument.
- `--json` (bool, default `false`): emit the stats summary as JSON on
  stderr.

### `rewrite`

Each flag falls back to an environment variable when unset, listed with it
below.

- `--backend` (string, default `print-prompt`, env
  `WATERMARKS_REWRITE_BACKEND`): `print-prompt` (offline, builds the prompt
  only), `ollama`, or `openai-compatible`.
- `--model` (string, default `""`, env `WATERMARKS_REWRITE_MODEL`): model
  name, required for the `ollama` and `openai-compatible` backends.
- `--base-url` (string, default `http://127.0.0.1:11434`, env
  `WATERMARKS_REWRITE_BASE_URL`): backend base URL.
- `--allow-remote` (bool, default `false`, env
  `WATERMARKS_REWRITE_ALLOW_REMOTE`): allow a non-loopback backend host.
  Denied by default, since content would otherwise leave the machine. The
  flag and env var are checked independently: passing `--allow-remote`
  explicitly always wins; otherwise the env var is consulted.
- `--strength` (string, default `paraphrase`): `paraphrase`, `humanize`,
  `code`, `backtranslate`, or `structural`.
- `--lang` (string, default `French`): pivot language for `backtranslate`.
- `--original-lang` (string, default `English`): original language for
  `backtranslate`.
- `--timeout` (float, default `120.0`): per-request timeout in seconds.
- `--temperature` (float, default `0.9`): sampling temperature for the
  backend.
- `--candidates` (int, default `1`): number of rewrite candidates to
  generate and score, keeping the most lexically diverged.
- `--no-layer-a-after` (bool, default `false`): skip the Layer A scrub on
  model output.
- `-o`, `--output` (string, default `""`): write the result here instead of
  stdout.
- `--json-stats` (bool, default `false`): emit the info block as JSON on
  stderr.

There is deliberately no `--api-key` flag: a key on argv leaks through `ps`
and shell history. Set `WATERMARKS_REWRITE_API_KEY` instead.

## Environment variables

- `HUMANIZER_CACHE_DIR`: overrides the vale style pack cache root (see
  Cache directory above).
- `XDG_CACHE_HOME`: cache root fallback when `HUMANIZER_CACHE_DIR` is unset.
- `HUMANIZER_BACKEND`: fallback for `judge --backend` (`litellm` or
  `openrouter`); also what `scan --holistic` and the `humanizer_judge` MCP
  tool use, since neither takes a backend flag.
- `HUMANIZER_MODEL`: fallback for `judge --model`, applied to whichever
  backend is selected.
- `LITELLM_BASE_URL`: base URL of a LiteLLM proxy, without the `/v1` suffix
  (a trailing `/v1` is trimmed). Setting it selects the `litellm` backend
  during auto-detection.
- `LITELLM_API_KEY`: virtual key for the LiteLLM proxy, sent as a bearer
  token. Optional: a proxy without a master key needs none. Never accepted
  as a flag.
- `OPENROUTER_API_KEY`: API key for OpenRouter. Setting it selects the
  `openrouter` backend when no LiteLLM base URL is set. Never accepted as a
  flag.
- `WATERMARKS_REWRITE_BACKEND`: fallback for `rewrite --backend`.
- `WATERMARKS_REWRITE_MODEL`: fallback for `rewrite --model`.
- `WATERMARKS_REWRITE_BASE_URL`: fallback for `rewrite --base-url`.
- `WATERMARKS_REWRITE_ALLOW_REMOTE`: fallback for `rewrite --allow-remote`
  (truthy values: `1`, `true`, `yes`, `on`).
- `WATERMARKS_REWRITE_API_KEY`: API key for the `openai-compatible` backend.
  Read only from this variable, never accepted as a flag.

## Example

```bash
# Point the style pack cache at a custom location.
export HUMANIZER_CACHE_DIR=~/.cache/humanizer-dev

# Configure a local Ollama rewrite backend without passing flags each time.
export WATERMARKS_REWRITE_BACKEND=ollama
export WATERMARKS_REWRITE_MODEL=llama3.2
export WATERMARKS_REWRITE_BASE_URL=http://127.0.0.1:11434

humanizer detect draft.md --min-severity warning
humanizer rewrite draft.md --strength humanize -o draft.rewritten.md
```
