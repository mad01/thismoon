# Migrated from

Every directory imported from another repo gets a row here at import time. Imports are clean (no git history — see docs/adr/0001), so this table is the only link back to where the code came from.

| Directory | Source repo | SHA at import | Imported | Notes |
|-----------|-------------|---------------|----------|-------|
| `webkit/` | github.com/mad01/webkit | `5a18e5d02f129f828733dbc2214151827dccc85c` | 2026-07-05 | Dropped `go.mod` and `.golangci.yml`; module path rewritten to `github.com/mad01/thismoon/webkit`; consumption docs updated for in-module use. |
| `services/reminder/` | github.com/mad01/dotfiles (`reminder/`) | `0a4582b7b6e3aace96e2d74813a8bdbe0a7fd66b` | 2026-07-05 | Dropped `go.mod`/`go.sum`; imports rewritten to `github.com/mad01/thismoon/services/reminder` and in-module webkit; removed `update-webkit` target and the webkit-pin field from `GET /version`. |
| `services/status/` | github.com/mad01/dotfiles (`status/`) | `5b0d67562ef0e6aff6b4a97b18719fdbf78b4397` | 2026-07-05 | Dropped `go.mod`/`go.sum`; imports rewritten to `github.com/mad01/thismoon/services/status` and in-module webkit; removed `update-webkit` target and the webkit-pin field from `GET /version`. |
| `services/events/` | github.com/mad01/dotfiles (`events/`) | `5b0d67562ef0e6aff6b4a97b18719fdbf78b4397` | 2026-07-05 | Dropped `go.mod`/`go.sum`; imports rewritten to `github.com/mad01/thismoon/services/events` and in-module webkit; removed `update-webkit` target and the webkit-pin field from `GET /version`. |
| `services/pr/` | github.com/mad01/dotfiles (`pr/`) | `5b0d67562ef0e6aff6b4a97b18719fdbf78b4397` | 2026-07-05 | Same playbook edits as the other services; host examples in help text and docs generalized to "GitHub Enterprise instances"; one `os.Remove` errcheck fix for this repo's stricter lint config. Host-gated `~/.config/pr/config.toml` symlinks stay in the dotfiles overlay. |
| `services/deps/` | github.com/mad01/dotfiles (`deps/`) | `5b0d67562ef0e6aff6b4a97b18719fdbf78b4397` | 2026-07-05 | Same playbook edits as the other services; discovery `config.toml` ships beside the recipe (glob example in a code comment generalized). |
| `services/present/` | github.com/mad01/dotfiles (`present/`) | `5b0d67562ef0e6aff6b4a97b18719fdbf78b4397` | 2026-07-05 | Same playbook edits as the other services. MCP seatbelt wrapper + profile and the present skill moved to `recipes/present/`; webkit docs rewritten for in-module consumption (asset-hash `/webkit/version`). |
