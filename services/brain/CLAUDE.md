# brain, local markdown knowledge base (CLI + MCP + web)

Go CLI, MCP stdio server and `brain serve` web view over Obsidian-style markdown vaults. It indexes each vault with zoekt for literal search and chromem-go for semantic search on Ollama embeddings, and fuses the two for hybrid search. brain is the tool only: the content lives in vault repos checked out wherever the config points, typically one private vault plus a shared team vault that is read-only here. Terms (vault, private, local-only, shared and default vault, note ref, note id, tier, note, stem, wikilink, backlink, MOC, evidence tier, promotion, shared promotion, status, revoked note, tombstone, citation, classification, vault policy, check, similar note, autocommit, sync, stale queue, promotion queue, topic node, layout key) are defined in `CONTEXT.md` beside this file. The note lifecycle, git vaults and shared promotion are ADR-0030.

## Module layout

```
brain/
  facts.go, paths.go  - package brain: defaults, serve's port and its env var, agentdoc.Facts, config and state paths
  config.md           - config file, precedence, paths, env vars, parse-failure posture
  cmd/brain/          - entrypoint (delegates to internal/cli)
  internal/
    cli/              - cobra: index, search, semantic-search, hybrid-search, count, query, read, peek, backlinks, graph, write, capture, edit, promote, supersede, retract, withdraw, flag, inbox, drift, stale, check, similar, sync, list, vaults, migrate, docs, doctor, mcp, serve, version
    check/            - brain check: the rules, the vault's .brain/policy.yaml, the report
    drift/            - brain drift: code refs checked in local checkouts (git), external sources fetched (--online)
    stale/            - brain stale: the computed review queue, grouped by CODEOWNERS owner, steward or unowned
    lifecycle/        - the status model: statuses, lifecycle fields, dates a write fills, revocation, banners
    config/           - loads the config file (vaults, semantic settings, code_roots)
    doctor/           - the checks `brain doctor` and `brain_doctor` share
    library/          - the vaults opened together: ref lookup, per-vault index refresh under a lock, cross-vault search, count, semantic and hybrid queries
    lockfile/         - cross-process flock on the files under the state dir's locks/
    atomicfile/       - writes through a uniquely named temporary file: renamed over a target, or linked to a new path
    gitvault/         - git for one vault: the autocommit of changed paths, upstream, fetch, ff-only pull, push, worktrees
    ghcli/            - gh for a shared vault's repo: pr create, pr view, issue create
    sharing/          - promote --to, edit --pr, a shared flag's issue and brain sync, over library, gitvault and ghcli
    vault/            - note read, write, edit, promote, revocation, peek, id migration, the secret scan before a write, the vault Set and note refs, the one wikilink resolver behind backlinks, promote, the graph and check; the logic CLI and MCP share
    noteid/           - ULID note ids
    markdown/         - frontmatter parse and render (unknown keys kept), wikilink parsing
    search/           - zoekt index build, staleness fingerprint, query
    chunk/            - splits notes into chunks for embedding
    embedding/        - Ollama embeddings client
    semantic/         - chromem-go index and indexer
    hybrid/           - reciprocal rank fusion of literal and semantic results
    rerank/           - optional LLM rerank of fused results
    mcpserver/        - MCP tool handlers, thin adapters over the packages above
    server/           - brain serve: the HTTP API of docs/serve.md over library and vault, the layout cache in the state dir's layout/
    server/ui/        - the brain.this UI embedded in the binary: shell.html, app.js, graph.js, the graph bundle and its SHA256SUMS in assets/
    testhome/         - test helper isolating HOME, XDG and BRAIN_CONFIG, with BRAIN_PORT on a closed port
    testgit/          - test helper running git in a test's own temp repos and bare remotes
    testgh/           - test helper putting a fake gh on PATH that records its argv
  docs/cli.md         - agent CLI guide, embedded and printed by `brain docs`
  docs/lifecycle.md   - statuses, revocation, check and freshness, printed by `brain docs lifecycle`
  docs/sharing.md     - git vaults, sync, promote --to, edit --pr, similar, printed by `brain docs sharing`
  docs/serve.md       - the brain serve HTTP API and UI contract (routes, JSON shapes, layout, browser globals), printed by `brain docs serve`
  examples/shared-vault/ - a shared vault's repo to copy: policy, CODEOWNERS, CI workflow; a test keeps it passing check
  docs/mcp.md         - full MCP tool reference (inputs, outputs, evidence scheme)
  web/                - the npm package, lockfile and esbuild script behind the UI's graph bundle
  THIRD-PARTY.md      - notices for the npm packages in the graph bundle
```

## Build, test, install

```bash
make build      # ./brain, linked with $(BUILDINFO_LDFLAGS)
make test       # go test ./...
make lint       # golangci-lint, using the repo-root .golangci.yml
make test-js    # the UI's JavaScript tests under node's test runner (CI runs them too)
make bundle     # rebuilds the graph bundle and its SHA256SUMS in a throwaway Apple container
make bundle-local  # the same build on the host, only when the container runtime is missing
make install    # copies to ~/code/bin/brain and re-signs on macOS
```

The graph bundle in `internal/server/ui/assets/` is committed build output (ADR-0031): `web/package.json` pins each direct dependency and `web/package-lock.json` the rest. A version bump is a lockfile change, `make bundle` and a `THIRD-PARTY.md` update in one commit.

`brain version` prints the bare version token; `brain version -o json` prints the shared four-key build object (`version`, `commit`, `tag`, `build_time`) from the `buildinfo` package.

Tests never read or write the real config or state. A test that resolves a default path calls `testhome.Isolate(t)` first. It points `HOME` at a temp dir, clears `XDG_CONFIG_HOME`, `XDG_STATE_HOME` and `BRAIN_CONFIG`, and sets `BRAIN_PORT` to a loopback port nothing listens on, so `brain doctor` in a test never probes a real serve. Everything else takes explicit temp paths. Keep it that way; a test that falls through to the real config indexes the real vault.

Tests never reach the network, the real `gh` or a real checkout. A git test calls `testgit.Isolate(t)`, which points `GIT_CONFIG_GLOBAL` and `GIT_CONFIG_SYSTEM` at `/dev/null` and sets an explicit author, and its remotes are bare repos from `testgit.Bare(t)` in `t.TempDir()`. A test that runs `gh` calls `testgh.Install(t, ...)` first: a fake script on `PATH` that records each call's argv and prints canned URLs or `pr view` JSON.

## Config and state

`config.md` is the reference. In short: the config file is `--config`, then `BRAIN_CONFIG`, then `config.yaml` in `confdir.Dir("brain")`. It names one vault (`vault: <path>`, loaded as the vault `default`) or a `vaults:` list with `name`, `path`, `default` and `shared`. State lives in `brain.StateDir()`, `$XDG_STATE_HOME/brain` or `~/.local/state/brain`, with one `index/<vault>/` (shards and `state.json`) and `semantic/<vault>/` per vault, the lock files in `locks/`, the temporary pull request worktrees in `worktrees/` and serve's layout cache in `layout/`. A vault entry may set `autocommit: false`. Every index refresh goes through `library`, which takes the vault's literal or semantic lock, so the CLI and the MCP server never build one index at once. The semantic store keeps a `manifest.json` (settings, fingerprint, a content hash per note), so a sync re-embeds only changed notes and purges deleted ones. An install whose `~/.config/brain/index/state.json` exists keeps `~/.config/brain`, unless `XDG_STATE_HOME` is set. The probe is that file because only a finished `brain index` writes it, while provisioning links `config.yaml` into `~/.config/brain` on every machine. A present but invalid config fails every command with exit 2.

Evals isolate brain with exactly `BRAIN_CONFIG=<tmp>/config.yaml` and `XDG_STATE_HOME=<tmp>/state`; `TestIsolatedByConfigAndStateHome` pins that nothing else is written. User-facing defaults are constants in `facts.go`; don't add a bare literal default elsewhere.

## CLI contract

Agents drive brain from the shell, so the CLI keeps a stable contract:

- Every command takes `-o text|json`, and `check` and `similar` also take `-o github` (workflow annotations). Under `-o json` stdout is one JSON document with the same shape the matching MCP tool returns, and a failure prints `{"error":{"code","message"}}` there.
- Errors print once on stderr as `brain <cmd>: <message>` and exit with a documented code:

| Code | JSON `code` | Meaning |
|---|---|---|
| 0 | | success |
| 1 | `error` | anything else (config, I/O, Ollama) |
| 2 | `usage` | bad flag, argument, path, tier or vault; a title with a control character; a note with a NUL byte or unclosed frontmatter; a note path that is a symbolic link; an edit that sets superseded, retracted or withdrawn, or edits a tombstone |
| 2 | `ambiguous` | a bare path or title names notes in more than one vault |
| 2 | `invalid_config` | config file does not parse or describes no usable vault |
| 3 | `not_found` | note or title does not exist |
| 4 | `already_exists` | write or promote target exists, also when another writer created it first |
| 5 | `invalid_query` | zoekt query does not parse |
| 6 | `read_only` | write, edit, promote, migrate or a revocation in a shared vault (the message names `edit --pr` and `promote --to`); `promote --to` a vault that is not shared; a shared flag on a clone with no remote |
| 7 | `conflict` | `edit --if-match` sha no longer matches the note |
| 8 | `secret_found` | the note would hold a secret (kit/secretscan); nothing was written |
| 9 | `possible_duplicate` | write or capture: a note likely duplicates an existing one (same normalized title or alias, or semantic similarity of at least `library.DuplicateSimilarity`); nothing was written |

- `docs/cli.md` is the agent guide `brain docs` prints. Keep it under 200 lines and in step with `internal/cli/errors.go`: a new exit code or error class changes both. Lifecycle and freshness detail goes in `docs/lifecycle.md` (`brain docs lifecycle`), git vaults and sharing in `docs/sharing.md` (`brain docs sharing`), each under the same cap.
- Writes are safe against each other: a new note is created exclusively (a temporary file hard-linked into place), every rewrite renames a uniquely named temporary file, and write, edit, promote and migrate hold the vault's `locks/<vault>.write.lock`. `read -o json` returns the note's `sha`; `edit --if-match <sha>` refuses with exit 7 when the note changed since.
- Frontmatter keys brain does not model survive write, edit and promote in source order, nested values included (`internal/markdown/fields.go`). `--set key=<yaml>` and `--unset key` reach any key; typed keys are shape-checked. An edit rewrites only the keys it changes (`internal/markdown/patch.go`): the comments, quoting, indentation, fences and line endings of the rest stay. Frontmatter it cannot patch line by line (a flow mapping, two keys on one line, a repeated key) is written fresh, and a new note gets 2-space YAML.

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

Each vault's literal index lives at `index/<vault>/` and its semantic index at `semantic/<vault>/` in the state directory (above). Every search, from the CLI or MCP, refreshes the indexes of the vaults it covers first: a vault mtime fingerprint check, then a re-index of that vault alone when stale. A manual `brain index` is only needed after a config change or to force a rebuild.

Semantic indexing needs Ollama running locally with `nomic-embed-text` pulled; the rerank pass uses `llama3.2:3b`. When Ollama isn't reachable, `brain index` falls back to the literal index alone.

## MCP tools

Fourteen tools: `brain_search`, `brain_semantic_search`, `brain_hybrid_search`, `brain_count`, `brain_query_validate`, `brain_read`, `brain_peek`, `brain_list_tier`, `brain_backlinks`, `brain_graph`, `brain_write`, `brain_edit`, `brain_promote`, `brain_doctor`. Schemas: `docs/mcp.md`. The server instructions come from `agentdoc.Instructions(brain.Facts())`, and `brain mcp --help` ends with `agentdoc.RegistrationSnippet`.

- `brain_search` and `brain_count` take zoekt query syntax; `brain_query_validate` checks a query before running it.
- A `tier` input on search, list and count expands to `file:^<tier>/`, so don't add a `file:` prefix yourself when passing `tier`.
- `brain_promote` writes the note under the target tier, sets `status: validated`, rewrites every `[[oldstem]]` wikilink across the vault, then deletes the inbox file. It is all or nothing: a failure puts every file back. `moc` must be a note in `MOCs/`.
- `brain_edit` takes `body` (replace) or `append`, which are mutually exclusive, plus a `frontmatter_patch`.
- `brain_doctor` is read-only and returns the `brain doctor -o json` report; a failing check is a result, not a tool error.
- Write, edit and promote autocommit in a git vault through the same `library` and `vault` path as the CLI. Sync, `promote --to`, `edit --pr` and `similar` are CLI only; `brain_promote` takes no `to`.
- Revocation and `brain check` are CLI only. Search tools hide revoked notes unless `include_revoked`; write tools return `warnings` for citations of notes that are not validated and refuse a secret through the same `vault` path as the CLI.

Smoke test:

```bash
( printf '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}\n{"jsonrpc":"2.0","method":"notifications/initialized"}\n{"jsonrpc":"2.0","id":2,"method":"tools/list"}\n'; sleep 1 ) | brain mcp
```

Expect the `initialize` response, then `tools/list` naming all fourteen `brain_*` tools.

## Serve, recipe, sandbox

- `brain serve` is the read-only web view at brain.this, on `127.0.0.1:7427`: `--port`, then `BRAIN_PORT`, then `brain.DefaultPort` in `facts.go`. `docs/serve.md` is the contract the server, the UI and the docs build against; change it there first.
- serve never writes a vault and runs no git or gh. Its one write is the layout cache in the state dir's `layout/`. A config with no usable vault does not stop it: `/api/doctor` and the UI report the problem, so t-man does not restart it in a loop.
- `brain doctor` checks a running serve (`serve-reachable`, `version-skew`, `serve-vaults`, `serve-state-dir`). CI lists brain among the serve-bearing components that must ship a doctor, and runs `make test-js`.
- Recipe: `recipes/brain/` builds the binary as `packages.brain` (wave 0) and registers the t-man agent `brain-serve`, which runs `srt-run ~/.config/srt/brain-serve.json brain serve --port 7427`. The consuming repo's overlay keys its edges on `packages.brain`; keep the key, the wave and the name. `recipes/brain/CLAUDE.md` lists the items.
- Sandbox: srt under the label `brain-serve`, rendered from `spec.access` in `service-info.yaml` to `recipes/brain/brain-serve.srt.json` by `make sandbox-settings`. It denies `~`, reads `~/.config/brain`, `~/code/bin` and the state dir, writes the state dir and the temp dirs, binds 7427 and calls Ollama on loopback. Three things follow from it:
  - **Machine layer.** The vault roots are machine paths, and so is `semantic.index_dir` when it points outside the state dir. Each machine grants them in `~/.config/srt/brain-serve.local.json`, which may only widen. The shipped settings never grant `~`.
  - **Symlink target.** srt matches resolved paths. A `config.yaml` linked into `~/.config/brain` needs its target in that layer too, or serve starts and reports the config as unreadable.
  - **Directory read.** The grant is all of `~/.config/brain`, not just the config file, because the legacy probe `index/state.json` sits there; serve and the CLI then resolve the same state dir, which `serve-state-dir` checks. While the probe exists the state dir is `~/.config/brain`, where serve cannot write, so `docs/UPGRADING.md` moves those indexes to `~/.local/state/brain`.
