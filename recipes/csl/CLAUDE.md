# CLAUDE.md — csl recipe

Public-layer recipe for `services/csl` (see docs/adr/0006 for the layering).
Builds and installs the `csl` binary from the thismoon sources cache and
registers the `csl-web` t-man agent on port 7424.

Item keys are the dotfiles-era names (`packages.code_search_local`,
`hooks.builds.csl_web_service`) so ralph state carried over at cutover — do
not rename them.

What stays in the consuming repo (machine wiring, per ADR-0006):

- `~/.config/csl/config.yaml` symlink — which directories a machine indexes
  is per-machine, as is the semantic embedding model override
  (`semantic.embed_model` / `semantic.dim`).
- MCP registration — `csl mcp` in the claude-mcp recipe's `servers.json`.
- Ollama and the embedding model pull (`ollama pull unclemusclez/jina-embeddings-v2-base-code:f16`) —
  semantic search needs a running Ollama; lexical search works without it.

Release artifacts do not exist for csl (tree-sitter is unconditional cgo — see
the release.yml comment); this recipe is the only install path.
