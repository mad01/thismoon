# Semantic search

How csl's meaning-based search works: what lexical and semantic search each
are, how the semantic index gets built, what happens on a query, and how to
run a different embedding model per machine.

## Lexical vs semantic, in general

The two backends answer different questions.

**Lexical search** matches the literal text you typed. An indexer (zoekt
here) builds an inverted index, a map from trigrams to the files and
positions where they occur, so a query like `Fingerprint` or `f:.*\.go$
retry` becomes a handful of index lookups instead of a scan. Lexical search
is exact, fast, and cheap; its results are easy to trust because a match
*is* the string you asked for. Its weakness is vocabulary: it only finds
what you can name. If the code calls it `RepoState` and you search for
"checkout status", lexical search returns nothing.

**Semantic search** matches meaning. An embedding model — a neural network
trained so that texts with similar meaning land near each other — maps each
piece of code to a vector (a list of a few hundred to a few thousand
floats). At query time your question goes through the same model, and search
becomes geometry: find the stored vectors closest to the query vector,
usually by cosine similarity. This is why "compute a hash of a repo's git
state" can find a function named `Fingerprint` that never contains the word
"hash". The trade-offs are the mirror image of lexical: the model ranks
results by a learned notion of similarity rather than an exact match,
quality depends on what the model was trained on (a model that never saw
code ranks code poorly), and building the index costs real compute because
every chunk must pass through the model once.

Neither wins outright, which is why csl also has **hybrid search**: run
both backends, then fuse the two ranked lists with Reciprocal Rank Fusion
(RRF), where each result scores `1/(k + rank)` summed across the lists it
appears in. Exact-term queries ride the lexical list; paraphrased queries
ride the semantic list; results both backends agree on rise to the top. See
[architecture.md](architecture.md) for the fusion details.

Rule of thumb: reach for lexical when you know the identifier, semantic
when you know the behavior, hybrid when you're not sure which you have.

## How indexing works

`csl index --semantic-all` (or the per-repo `--semantic --repo <name>`)
builds the vector index in three steps per repo:

Before any of them, csl decides which files are worth embedding at all
(`internal/semantic/index.go`). The candidate list is `git ls-files` — only
tracked files, so anything gitignored never reaches the chunker; non-git
directories fall back to a filesystem walk. A tracked file is then dropped
when it sits in a build or vendored tree (`.build`, `DerivedData`, `Pods`,
`target`, `node_modules`, `vendor`, asset catalogs, Xcode project bundles),
when it is a lockfile or ML-model metadata file (`package-lock.json`,
`go.sum`, `tokenizer.json`, `vocab.json`), when its extension marks binary
or media content, when it exceeds 1MB, or when a content sniff finds null
bytes (compiled binary) or kilobyte-long lines (serialized data). The rules
run on *tracked* files deliberately: a committed Swift `.build/` directory
or a bundled tokenizer vocabulary is tracked, text, and under the size cap,
yet embedding it would bury real source in retrieval noise.

A repo can extend these rules with a `.cslignore` file at its root: one
glob per line, `#` comments allowed. Patterns match repo-relative paths —
`*` stops at path separators, `**` crosses them, a trailing `/` covers the
whole tree under a directory, and a leading `/` anchors the pattern to the
repo root (unanchored patterns match at any depth). So `models/` drops
every file under any `models` directory, and `/docs/generated/` only the
root-level one. Unlike the built-in rules above, `.cslignore` applies to
both indexes — matched files are excluded from lexical (zoekt) search and
from semantic embedding.

`csl semantic files [path]` prints the decision for every tracked file
under a path; `--skipped` shows what was filtered and why, including
`.cslignore` matches.

1. **Chunking.** Each source file is split into chunks
   (`internal/semantic/chunk.go`). For languages with a tree-sitter
   grammar, chunks follow declarations — a function, method, or type with
   its body — so a vector corresponds to a unit a human would recognize.
   Oversized declarations are split, and files without a grammar fall back
   to fixed windows (120 lines, 20 overlap). Each chunk carries a
   breadcrumb (repo, path, symbol) plus its body, capped at 6000
   characters.
2. **Embedding.** csl sends the chunks in batches to an Ollama server's
   `/api/embed` endpoint (`internal/semantic/ollama.go`). csl bundles no
   model; Ollama owns model loading, GPU use, and lifetime. Every request
   pins `num_ctx` and `num_batch` to 8192 tokens so large chunks are
   neither truncated nor crash the runner, and bulk runs unload the model
   when they finish so it doesn't squat in memory.
3. **Storing.** Vectors land in one gob file per repo under
   `semantic-index/` in csl's state directory, alongside each file's content hash, the
   chunker version, and the vector dimensionality.

Re-runs are incremental: a file whose content hash is unchanged is skipped
entirely, so a rebuild after touching one file re-embeds one file. Two
recorded invariants force a wider rebuild automatically — if the store's
chunker version doesn't match the binary's, or its dimensionality doesn't
match the configured model's, the store is dropped and the repo re-embedded
from scratch. Stale vectors are never silently mixed with fresh ones.

When `semantic.sync: true` is set, `csl sync` runs the same per-repo
embedding pass over repos whose lexical index changed, best-effort, after
the pull.

## When local semantic search is worth it

Semantic and hybrid search are **off by default** (`semantic.enabled: false`),
and whether to turn them on is a per-machine decision that comes down to corpus
size. Building the index is the expensive part: every chunk of every tracked
file passes through the embedding model once, and `csl sync` re-embeds changed
repos on top of that. Lexical search has neither cost nor an Ollama dependency,
so it stays the right default at any scale.

As a rough guide, local semantic search stays comfortable up to **~250 repos**.
Below that, the initial `csl index --semantic-all` and the incremental refreshes
finish in reasonable time on a typical developer machine, so turning
`semantic.enabled: true` on pays off. Well above that, the one-time embed (and
every refresh after it) is too much compute to run locally: the build takes too
long and starves everything else on the machine.

For a corpus past that point, keep the machine lexical-only (the default), or
point `ollama_url` at a beefier machine and run the bulk build there (see
[Choosing and changing the model](#choosing-and-changing-the-model)) so the
heavy embedding happens off your laptop while queries stay local.

## How a query works

A semantic query (`csl semantic`, `csl_semantic_search`, the web UI, or the
semantic half of hybrid) does the inverse of indexing, once:

1. The query string is embedded through the same Ollama model. For
   instruction-tuned models (the qwen3-embedding family) csl prepends a
   retrieval instruction to the query — those models rank better when the
   query states its task — while documents are always embedded bare.
   Models without instruction tuning get the bare query; the prefix would
   be embedded as literal text and hurt ranking.
2. The query vector is compared against every stored chunk vector by
   cosine similarity, in-process — the model sees only the query, never
   your code, at search time.
3. The top-k chunks are expanded back to source snippets with their
   repo/path/line positions.

The daemon serves this from stores it holds in memory when
`semantic.enabled: true`; otherwise the caller loads the stores for that
one query. Either way the expensive part is the single query embed —
milliseconds once the model is warm. Ollama keeps the model resident for 20
minutes after a request (`keep_alive`), so the first query after idle pays
a model load of a second or two and the rest of the session doesn't.

Lexical queries never touch the model or Ollama; with `semantic.enabled`
off, csl has no Ollama dependency at all.

## Choosing and changing the model

The embedding model is per-machine configuration, not a build decision:

```yaml
semantic:
  enabled: true
  ollama_url: http://localhost:11434     # default
  embed_model: unclemusclez/jina-embeddings-v2-base-code:f16  # default
  dim: 1024                              # must match the model's output
```

Change `embed_model` and `dim`, pull the model (`ollama pull <model>`), and
run `csl index --semantic-all`. The dimensionality mismatch against the old
stores triggers the automatic full re-embed described above — no manual
cleanup. The only hard rule: query and index vectors must come from the
same model, which the dim check enforces for you (except between models
that share a dimensionality — after swapping between two 768-dim models,
force a rebuild yourself).

Models that work well here, all served by Ollama:

| Model | Dim | Context | Character |
|---|---|---|---|
| `qwen3-embedding:0.6b` | 1024 | 32k | Strongest code retrieval; too heavy for bulk indexing (MAD-235) |
| `unclemusclez/jina-embeddings-v2-base-code:f16` | 768 | 8k | Default. Code-trained, ~4x faster to index than qwen3 |
| `nomic-embed-text` | 768 | 8k | General-purpose; fine on prose, weaker on code |

Anything you point csl at needs a context window comfortably above the
chunk budget (6000 characters is roughly 1500–2000 tokens); a 512-token
model would silently truncate most chunks. Since `ollama_url` is also
config, the same mechanism reaches a model served on another machine —
useful for pushing a bulk index build off a laptop — as long as queries and
index builds keep hitting the same model.

## See also

- [Architecture](architecture.md): daemon lifecycle and hybrid fusion.
- [Configuration](configuration.md): every `semantic.*` key.
- [CLI reference](cli.md): `csl index`, `csl semantic`, `csl hybrid` flags.
