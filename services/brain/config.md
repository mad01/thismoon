# brain configuration

## Where config lives

`brain` reads exactly one config file per run, resolved in this order:

1. `--config <path>`, a persistent flag on every subcommand.
2. `$BRAIN_CONFIG`.
3. `config.yaml` in the XDG config directory: `$XDG_CONFIG_HOME/brain` when
   that variable holds an absolute path, otherwise `~/.config/brain`.

A leading `~` is expanded in the flag and the environment variable, so both
work where no shell expands them first. `brain mcp` resolves the file once at
startup and every tool loads that file.

## Parse failure

brain is a CLI that agents depend on, so it fails loudly. A config file that
exists but does not parse, or that names no `vault`, fails every command that
reads it with exit code 2. The message names the file, and under `-o json`
the error code is `invalid_config`. brain never falls back to defaults over a
broken file: whoever wrote that file meant to configure brain, and running
on defaults would hide the mistake.

A missing file is exit 1 (`error`) with a pointer to `brain doctor`, whose
`config` check names the path it looked for and how to create it. Commands
that read no config (`version`, `docs`, `query`) run either way, and
`brain doctor` reports either case as a failed `config` check and exits 1,
as it does for any failed check.

## Where state lives

Nothing but the config file belongs in the config directory. brain's state
lives in the state directory: `$XDG_STATE_HOME/brain` when that variable holds
an absolute path, otherwise `~/.local/state/brain`. It holds:

- `index/`: the literal (zoekt) index shards and `state.json`, the
  fingerprints of both indexes.
- `semantic/`: the chromem-go vector store, unless `semantic.index_dir` moves
  it.

Every file there is rebuilt from the vault, so deleting the directory costs a
re-index (and a re-embed when the semantic layer is on) and loses nothing.

Installs from before the split kept state in `~/.config/brain`. When
`~/.config/brain/index/state.json` exists and `XDG_STATE_HOME` is unset, that
directory stays the state directory, so an upgrade moves no files and
re-embeds nothing. The state file settles it, not the directory: provisioning
links `config.yaml` into `~/.config/brain` on every machine, and only a
finished `brain index` writes `state.json`. An absolute `XDG_STATE_HOME`
always wins, which is what lets an eval or a test isolate brain from a
machine's real indexes. `brain doctor` names the index directory in effect.

## Keys

- `vault` (string, required): the vault root. A leading `~` is expanded.
- `semantic.enabled` (bool, default `false`): turns on the semantic layer.
  When off, `semantic-search`, `hybrid-search` and their MCP tools return a
  "semantic disabled" error and `brain index` builds the literal index only.
- `semantic.ollama_url` (string, default `http://localhost:11434`): the local
  Ollama base URL.
- `semantic.embed_model` (string, default `nomic-embed-text`): the embedding
  model.
- `semantic.chat_model` (string, default `llama3.2:3b`): the model the
  optional reranker scores with.
- `semantic.index_dir` (string, default `semantic/` in the state directory):
  where the vector store lives. A leading `~` is expanded.
- `semantic.chunk_size` (int, default `800`): target tokens per chunk.
- `semantic.chunk_overlap` (int, default `100`): tokens adjacent chunks
  share.
- `semantic.rerank_default` (bool, default `false`): rerank hybrid results
  when a call does not say.

The defaults are the constants in `facts.go`. Unknown keys are ignored.

## Environment variables

- `BRAIN_CONFIG`: path to the config file. `--config` beats it.
- `XDG_CONFIG_HOME` / `XDG_STATE_HOME`: roots of the config and state
  directories when set to an absolute path. Relative values are ignored, per
  the XDG spec.
- `HOME`: root of both directories when the XDG variables are unset. An
  unresolvable home is an error, never a path relative to the working
  directory.

To run brain against a scratch setup without touching a machine's real
config or indexes, set exactly `BRAIN_CONFIG=<tmp>/config.yaml` and
`XDG_STATE_HOME=<tmp>/state`.

## Example

```yaml
# ~/.config/brain/config.yaml
vault: ~/notes/vault
semantic:
  enabled: true
  ollama_url: http://localhost:11434
  embed_model: nomic-embed-text
```
