# Migrated from

Every directory imported from another repo gets a row here at import time. Imports are clean (no git history — see docs/adr/0001), so this table is the only link back to where the code came from.

| Directory | Source repo | SHA at import | Imported | Notes |
|-----------|-------------|---------------|----------|-------|
| `webkit/` | github.com/mad01/webkit | `5a18e5d02f129f828733dbc2214151827dccc85c` | 2026-07-05 | Dropped `go.mod` and `.golangci.yml`; module path rewritten to `github.com/mad01/thismoon/webkit`; consumption docs updated for in-module use. |
| `services/reminder/` | github.com/mad01/dotfiles (`reminder/`) | `0a4582b7b6e3aace96e2d74813a8bdbe0a7fd66b` | 2026-07-05 | Dropped `go.mod`/`go.sum`; imports rewritten to `github.com/mad01/thismoon/services/reminder` and in-module webkit; removed `update-webkit` target and the webkit-pin field from `GET /version`. |
