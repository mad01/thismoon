# `srtspec` recipe

Builds and installs the `srtspec` CLI to `~/code/bin/srtspec`. Source lives in
this repo: `tools/srtspec/` (see `tools/srtspec/CLAUDE.md`).

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`), and
ralph merges the recipe with the identity `thismoon/srtspec`. The package
`working_dir` therefore names the source cache
(`~/.config/ralph/sources/thismoon/tools/srtspec`), not a dev checkout.

## What this recipe does

- Package only, public layer per docs/adr/0006. No hooks: the srt settings
  files are rendered at commit time (`make sandbox-settings`) and linked by
  each service's own recipe, so a machine never renders anything at apply
  time.
- The binary is the launcher. `~/code/bin/srt-run` (speak recipe) is a shim
  that execs `srtspec run`, so every sandboxed t-man service and MCP server
  starts through it; `srtspec explain <label>` shows the policy it merges
  from `~/.config/srt` and `SRT_RUN_ALLOW_DOMAINS`.
- Wave 0, with the service recipes. Their registration hooks name
  `packages.srtspec` in `depends_on`, which orders them after this build
  within the wave (docs/adr/0006, 2026-10-26 amendment names srtspec a
  platform foundation).

## See also

- Source + module notes: `tools/srtspec/CLAUDE.md`
- Authoring a manifest: `docs/SANDBOXING.md`, "Declaring access"
