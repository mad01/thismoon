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
exists but does not parse, names no vault, or describes its vaults
inconsistently (see Vaults below) fails every command that reads it with
exit code 2. The message names the file, and under `-o json`
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
an absolute path, otherwise `~/.local/state/brain`. Each vault has its own
indexes there, named after the vault:

- `index/<vault>/`: the literal (zoekt) index shards and `state.json`, the
  fingerprint the literal index was built against.
- `semantic/<vault>/`: the chromem-go vector store, unless
  `semantic.index_dir` moves the parent, and `manifest.json`: the embed
  model and chunk sizes the store was made with, the fingerprint it was
  synced against, and a content hash per note. A sync re-embeds only the
  notes whose hash changed and drops the chunks of deleted notes. A store
  made with another model or chunk size, or one without a manifest, is
  rebuilt.
- `locks/`: lock files. A build of one vault's literal index holds
  `<vault>.literal.lock` and a semantic sync holds `<vault>.semantic.lock`,
  so two brain processes (the CLI, the MCP server) never build the same
  index at once; the one that waited finds the index fresh and skips the
  build.

Because every vault keeps its own fingerprint, a pull in one vault
re-indexes only that vault. Every file there is rebuilt from the vaults, so
deleting the directory costs a re-index (and a re-embed when the semantic
layer is on) and loses nothing.

Installs from before the split kept state in `~/.config/brain`. When
`~/.config/brain/index/state.json` exists and `XDG_STATE_HOME` is unset, that
directory stays the state directory, so an upgrade moves no files and
re-embeds nothing. The state file settles it, not the directory: provisioning
links `config.yaml` into `~/.config/brain` on every machine, and only a
finished `brain index` writes `state.json`. An absolute `XDG_STATE_HOME`
always wins, which is what lets an eval or a test isolate brain from a
machine's real indexes. `brain doctor` names the index directory in effect.
Indexes from before per-vault state sit at the root of `index/` and
`semantic/`; brain no longer reads them and builds `index/default/` and
`semantic/default/` afresh, which costs one re-embed. The old
`index/state.json` stays, because it is the legacy probe. A semantic store
from before manifests is rebuilt once, on its first sync.

## Vaults

A config names one vault with `vault: <path>`, or several with a `vaults`
list; setting both is invalid. The single-path form loads as one vault named
`default`. Each list entry has:

- `name` (string, required): a lowercase slug (`a-z`, `0-9`, single
  hyphens), unique across the list. It prefixes a qualified note ref
  (`team:notes/x.md`) and names the vault's index directories. `id`, `tag`
  and `dangling` are reserved.
- `path` (string, required): the vault root, unique across the list. A
  leading `~` is expanded.
- `default` (bool): the vault `brain write` targets without `--vault`. At
  most one vault sets it; when none does, the first non-shared vault is the
  default.
- `shared` (bool): a team vault, kept in its own repo. It is searched,
  listed and read like any vault, but `write`, `edit` and `promote` refuse
  it with exit code 6 (`read_only`): shared notes change through a pull
  request, which a later brain release opens for you. A shared vault cannot
  be the default.

List order matters: search lists vaults in it, and a wikilink that is not
found in its own vault resolves in the shared vaults in this order.

## Keys

- `vault` (string) or `vaults` (list, above): the vaults. One is required.
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
  the parent of each vault's vector store (`<index_dir>/<vault>/`). A
  leading `~` is expanded.
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
vaults:
  - name: personal
    path: ~/notes/vault
  - name: team
    path: ~/code/team-notes
    shared: true
semantic:
  enabled: true
  ollama_url: http://localhost:11434
  embed_model: nomic-embed-text
```
