# brain, local markdown knowledge base (CLI + MCP)

Go CLI and MCP stdio server over an Obsidian-style markdown vault. It indexes the vault with zoekt for literal search and chromem-go for semantic search on Ollama embeddings, and fuses the two for hybrid search. brain is the tool only: the vault content lives in your own vault repo, checked out wherever `vault:` in the config points. Terms (vault, tier, note, stem, wikilink, backlink, MOC, evidence tier, promotion) are defined in `CONTEXT.md` beside this file.

## Module layout

```
brain/
  cmd/brain/          - entrypoint (delegates to internal/cli)
  internal/
    cli/              - cobra: index, search, semantic-search, hybrid-search, count, query, read, peek, backlinks, write, edit, promote, list, docs, doctor, mcp, version
    config/           - ~/.config/brain/config.yaml (vault path, semantic settings)
    vault/            - note read, write, edit, promote, peek, backlinks; the logic CLI and MCP share
    markdown/         - frontmatter parse and render (unknown keys kept), wikilink parsing
    search/           - zoekt index build, staleness fingerprint, query
    chunk/            - splits notes into chunks for embedding
    embedding/        - Ollama embeddings client
    semantic/         - chromem-go index and indexer
    hybrid/           - reciprocal rank fusion of literal and semantic results
    rerank/           - optional LLM rerank of fused results
    mcpserver/        - MCP tool handlers, thin adapters over the packages above
  docs/cli.md         - agent CLI guide, embedded and printed by `brain docs`
  docs/mcp.md         - full MCP tool reference (inputs, outputs, evidence scheme)
```

## Build, test, install

```bash
make build      # ./brain, linked with $(BUILDINFO_LDFLAGS)
make test       # go test ./...
make lint       # golangci-lint, using the repo-root .golangci.yml
make install    # copies to ~/code/bin/brain and re-signs on macOS
```

`brain version` prints the bare version token; `brain version -o json` prints the shared four-key build object (`version`, `commit`, `tag`, `build_time`) from the `buildinfo` package.

Tests never read or write `~/.config/brain`: tests that load the config point `HOME` at a temp dir with `t.Setenv`, and everything else takes explicit temp paths. Keep it that way; a test that falls through to the real config indexes the real vault.

## CLI contract

Agents drive brain from the shell, so the CLI keeps a stable contract:

- Every command takes `-o text|json`. Under `-o json` stdout is one JSON document with the same shape the matching MCP tool returns, and a failure prints `{"error":{"code","message"}}` there.
- Errors print once on stderr as `brain <cmd>: <message>` and exit with a documented code:

| Code | JSON `code` | Meaning |
|---|---|---|
| 0 | | success |
| 1 | `error` | anything else (config, I/O, Ollama) |
| 2 | `usage` | bad flag, argument, path or tier |
| 3 | `not_found` | note or title does not exist |
| 4 | `already_exists` | write or promote target exists |
| 5 | `invalid_query` | zoekt query does not parse |

- `docs/cli.md` is the agent guide `brain docs` prints. Keep it under 200 lines and in step with `internal/cli/errors.go`: a new exit code or error class changes both.
- Frontmatter keys brain does not model survive write, edit and promote in source order, nested values included (`internal/markdown/fields.go`). `--set key=<yaml>` and `--unset key` reach any key; typed keys are shape-checked. Rendering writes 2-space YAML, so an edit to one key leaves the other lines of the frontmatter byte-identical.

## Tier model

| Tier | Directory | Default `type` | Default `status` | Purpose |
|---|---|---|---|---|
| inbox | `inbox/` | `note` | `raw` | Raw captures, unprocessed |
| notes | `notes/` | `note` | `validated` | First-class knowledge |
| outputs | `outputs/` | `output` | `validated` | Finished artifacts (date-prefixed filenames) |
| MOCs | `MOCs/` | `moc` | `validated` | Maps of Content, curated indexes |

The research loop: capture to inbox, verify empirical claims, promote to notes with an evidence tier in frontmatter, then write finished artifacts to outputs. `docs/mcp.md` (Evidence) has the A/B/C/D/mixed scheme.

## Indexing

```bash
brain index                 # refresh if stale (literal + semantic)
brain index --force         # unconditional rebuild
brain index --literal-only  # skip semantic (no Ollama needed)
```

The literal index lives at `~/.config/brain/index/` and the semantic index at `~/.config/brain/semantic/`. The MCP handlers call `ensureIndex` before every search: a vault mtime fingerprint check, then a re-index when stale. A manual `brain index` is only needed after a config change or to force a rebuild.

Semantic indexing needs Ollama running locally with `nomic-embed-text` pulled; the rerank pass uses `llama3.2:3b`. When Ollama isn't reachable, `brain index` falls back to the literal index alone.

## MCP tools

Twelve tools: `brain_search`, `brain_semantic_search`, `brain_hybrid_search`, `brain_count`, `brain_query_validate`, `brain_read`, `brain_peek`, `brain_list_tier`, `brain_backlinks`, `brain_write`, `brain_edit`, `brain_promote`. Schemas: `docs/mcp.md`.

- `brain_search` and `brain_count` take zoekt query syntax; `brain_query_validate` checks a query before running it.
- A `tier` input on search, list and count expands to `file:^<tier>/`, so don't add a `file:` prefix yourself when passing `tier`.
- `brain_promote` writes the note under the target tier, sets `status: validated`, rewrites every `[[oldstem]]` wikilink across the vault, then deletes the inbox file.
- `brain_edit` takes `body` (replace) or `append`, which are mutually exclusive, plus a `frontmatter_patch`.

Smoke test:

```bash
( printf '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}\n{"jsonrpc":"2.0","method":"notifications/initialized"}\n{"jsonrpc":"2.0","id":2,"method":"tools/list"}\n'; sleep 1 ) | brain mcp
```

Expect the `initialize` response, then `tools/list` naming all twelve `brain_*` tools.

## Not yet on the repo conventions

brain predates its import, and these gaps against docs/adr/0011 (config conventions) and docs/adr/0009 (operating docs) stay open until the code around them is touched:

- State lives under `~/.config/brain` (the `index/` and `semantic/` directories) rather than `~/.local/state/brain`. Moving it needs the legacy-directory probe ADR-0011 describes, so an existing index is found rather than rebuilt.
- No `--config` flag or `BRAIN_CONFIG` variable; the config path is fixed.
- The MCP server has no `brain_doctor` tool, and its instructions are not generated from `agentdoc.Facts`.

## Serve, recipe, sandbox

- Port 7427 is reserved for a future `brain serve` web UI. When it lands, declare the port as a `facts.go` constant (ADR-0011) and add brain to the CI list of serve-bearing components that must ship a doctor.
- A ralph recipe under `recipes/brain/` is coming with `brain serve`.
- No srt sandbox yet. brain reads the vault and writes its indexes outside any single workdir. The recipe needs a `spec.access` block in `service-info.yaml` (docs/SANDBOXING.md), since `make sandbox-verify` fails a recipe that launches a process without one.
