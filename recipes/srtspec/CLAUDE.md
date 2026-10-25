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
  time. The binary is here for `srtspec explain <label>`, which shows the
  policy srt-run merges from `~/.config/srt` and `SRT_RUN_ALLOW_DOMAINS`.

## See also

- Source + module notes: `tools/srtspec/CLAUDE.md`
- Authoring a manifest: `docs/SANDBOXING.md`, "Declaring access"
