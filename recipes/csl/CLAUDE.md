# CLAUDE.md — csl recipe

Public-layer recipe for `services/csl` (see docs/adr/0006 for the layering).
Builds and installs the `csl` binary from the thismoon sources cache,
registers the `csl-web` t-man agent on port 7424, and defines two shell
functions: `repo-sync` (`csl sync`) and `repo` (cd to a repo via `csl repo`).

Item keys are the dotfiles-era names (`packages.code_search_local`,
`hooks.builds.csl_web_service`) so ralph state carried over at cutover — do
not rename them.

What stays in the consuming repo (machine wiring, per ADR-0006):

- `~/.config/csl/config.yaml` symlink — which directories a machine indexes
  is per-machine, as is the semantic embedding model override
  (`semantic.embed_model` / `semantic.dim`).
- MCP registration — `csl mcp` in the consuming repo's `claude-mcp` recipe `servers.json`.
- Ollama and the embedding model pull (`ollama pull unclemusclez/jina-embeddings-v2-base-code:f16`) —
  semantic search needs a running Ollama; lexical search works without it.

Release artifacts exist (`csl/vX.Y.Z` tarballs, built with cgo on a macOS
arm64 runner in release.yml) and serve the mise and Homebrew paths; the fleet
ignores them and builds from the sources cache through this recipe.

The `repo` and `repo-sync` functions are written to
`~/.config/ralph/generated/generated_functions.sh` and reach a shell only
where ralph's rc-file integration sources that script, so a machine with the
binary but no ralph-managed rc file has neither helper. The standalone
equivalent is `eval "$(csl shell-init zsh)"`; a test in
`services/csl/internal/cli` keeps the bodies here and the command's output
identical, so a change to one must land in the other.
