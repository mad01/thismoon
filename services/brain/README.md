# brain

A local CLI and MCP stdio server over an Obsidian-style markdown vault. It indexes the vault with zoekt for literal search and with chromem-go for semantic search on Ollama embeddings, and serves both to Claude Code as `brain_*` tools.

brain is the tool only. The vault content lives in your own vault repo, checked out wherever `vault:` in the config points, so its history and backups stay separate from the tool's.

## Install

```bash
make install   # builds and installs ~/code/bin/brain (codesigned on macOS: the stable local identity, ad-hoc without it)
```

## Usage

```bash
brain index                # refresh the literal and semantic indexes if the vault changed
brain search '<query>'     # zoekt query across the vault
brain semantic-search 'q'  # vector search through Ollama embeddings
brain hybrid-search 'q'    # literal and semantic results fused by reciprocal rank
brain mcp                  # MCP stdio server exposing the brain_* tools
brain version -o json      # build metadata: version, commit, tag, build_time
```

`brain --help` lists the rest (read, peek, backlinks, write, edit, promote, list, count, query).

Agents: run `brain docs` for the guide and `brain doctor` to check setup.

## Vault tiers

| Tier | Directory | Default `status` |
|------|-----------|------------------|
| inbox | `inbox/` | `raw` |
| notes | `notes/` | `validated` |
| outputs | `outputs/` | `validated` |
| MOCs | `MOCs/` | `validated` |

The research loop: capture a finding to `inbox/` and check its empirical claims against primary sources. Then promote it to `notes/` with an evidence tier in frontmatter, and write finished artifacts to `outputs/` with wikilinks back to their sources.

## Configuration

`~/.config/brain/config.yaml` (or `$XDG_CONFIG_HOME/brain/config.yaml`; `--config` and `BRAIN_CONFIG` point elsewhere):

```yaml
vault: ~/path/to/your/vault
```

The indexes live in `~/.local/state/brain` (or `$XDG_STATE_HOME/brain`). Semantic indexing needs a local Ollama with `nomic-embed-text` pulled; without it, `brain index` falls back to the literal index alone. [config.md](config.md) has every key, path and environment variable.

Full tool reference: [docs/mcp.md](docs/mcp.md).
