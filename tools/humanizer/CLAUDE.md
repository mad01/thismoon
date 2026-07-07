# humanizer — CLI and MCP server for AI-writing detection and voice profiling

Go CLI + MCP server. Detects AI-writing patterns by shelling out to `vale` against an embedded Humanizer style pack, produces a quantitative voice profile, and exposes both as MCP tools for use inside Claude Code.

## Module layout

```
humanizer/
  cmd/humanizer/     — entrypoint (delegates to internal/cli)
  internal/
    cli/             — cobra command tree (root, mcp subcommand); version injected via ldflags
    mcpserver/       — MCP server wiring (server.go, tools_detect.go, tools_rules.go, tools_status.go, tools_voice.go)
    rules/           — vale style pack embedding and metadata (embed.go, metadata.go, vale/)
    voice/           — voice profiler and diff logic (profile.go, diff.go)
  testdata/          — fixture files for tests
  Makefile           — package path github.com/mad01/thismoon/tools/humanizer (monorepo module, no own go.mod)
```

## Build / install / test

```bash
make build    # produces ./humanizer binary
make install  # build + cp to ~/code/bin/humanizer + adhoc codesign
make test     # go test ./...
make tidy     # go mod tidy
make resign   # re-apply adhoc signature without rebuilding (macOS only)
```

Binary installs to `~/code/bin/humanizer`. Config lives at `~/.config/humanizer/config.yaml`, symlinked into place by the consuming repo's companion recipe.

## Wired in via (two-layer split, see docs/adr/0006)

- **Build/install (this repo):** `recipes/humanizer/recipe.toml` (wave 0 — builds before MCP registration)
- **Config symlink (consuming repo):** the companion recipe symlinks `config.yaml` → `~/.config/humanizer/config.yaml`
- **MCP registration (consuming repo):** registered as transport `stdio` via a sandbox wrapper script that execs `humanizer mcp` under a seatbelt profile (no network, `$HOME` reads default-denied) — wrapper, profile, and registration are machine-private wiring and stay in the consuming repo

## Gotchas

- **Wave 0 builder.** Must build before the consuming repo's MCP registration recipe (wave 1) registers it.
- **Codesign required for MCP.** macOS 15+ `taskgated` kills adhoc-signed binaries whose provenance xattr no longer matches. `make install` handles this; manual copies need `make resign BIN=~/code/bin/humanizer`.
- MCP subcommand is `humanizer mcp`. The server exposes: `humanizer_status`, `humanizer_detect`, `humanizer_detect_file`, `humanizer_detect_statistical`, `humanizer_rules_list`, `humanizer_rules_explain`, `humanizer_voice_profile`, `humanizer_voice_diff`.
- Two detection paths: **Vale span rules** (`humanizer_detect`, file under `internal/rules/vale/styles/Humanizer/*.yml`) flag a matched substring with line/column; the **statistical detector** (`humanizer_detect_statistical`, `internal/voice/statistical.go`) flags whole-sample properties (sentence-length stddev, contraction rate, TTR, short-text em-dash, heading density, anaphora) with no span. Run both for full coverage.
- Vale rule gotcha: `existence`/`occurrence` rules wrap each token in `\b…\b` by default, so a pattern that begins or ends with a non-word char (e.g. a leading `,` plus trailing `\.`, or a trailing `?`/`#`) never matches. Set `nonword: true` on those rules. Single-quoted YAML scalars must escape inner apostrophes as `''`; plain scalars can use `'?` directly.
- `vale` must be available on `$PATH` — installed by the consuming repo's package recipe.
- **The MCP server runs sandboxed** (seatbelt, via the consuming repo's registration wrapper). `humanizer_detect_file` reads only prose files (`.md`/`.markdown`/`.txt`) under `~/code/src` and `~/workspace`, plus `/tmp` paths; anything else under `$HOME` returns a clean error pointing at `humanizer_detect`. New runtime file/network needs require a change to the consuming repo's seatbelt profile, not just code.

## See also

- Recipe: `recipes/humanizer/recipe.toml` (this repo — build/install)
- MCP registration + sandbox wrapper: the consuming repo's companion recipe (machine-private wiring, docs/adr/0006)
