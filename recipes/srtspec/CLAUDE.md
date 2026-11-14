# `srtspec` recipe

Builds and installs the `srtspec` CLI to `~/code/bin/srtspec` and links the
`srt-run` sandbox launcher path. Source lives in this repo: `tools/srtspec/`
(see `tools/srtspec/CLAUDE.md`).

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`), and
ralph merges the recipe with the identity `thismoon/srtspec`. The package
`working_dir` therefore names the source cache
(`~/.config/ralph/sources/thismoon/tools/srtspec`), not a dev checkout.

## What this recipe does

- **`packages.srtspec`**: `make build` + `make install` to
  `~/code/bin/srtspec`. Public layer per docs/adr/0006. The srt settings
  files are rendered at commit time (`make sandbox-settings`) and linked by
  each service's own recipe, so a machine never renders anything at apply
  time.
- **`dotfiles.srt_run`**: symlinks `srt-run.sh` to `~/code/bin/srt-run`, the
  launcher path every sandboxed MCP server and t-man service execs through:
  `srt-run <settings-file> <argv...>`. The script is a shim over
  `srtspec run`, which runs the command under srt (anthropics/sandbox-runtime,
  a mise tool the consuming repo declares) as `env SRT_RUN=<label>
  <argv...>`, where the label is the settings file's name
  (`~/.config/srt/present.json` gives `present`). It sets the child's
  `TMPDIR` and merges the machine-local expansion layer
  `~/.config/srt/<label>.local.json` and `SRT_RUN_ALLOW_DOMAINS` into the
  copy srt reads. A layer that does more than widen is refused (exit 1).
  srt itself runs under bun when mise has one, else node; `SRT_RUN_RUNTIME`
  overrides, and a missing runtime exits 127. Each launch prints
  `srt-run: <label> runtime <bun|node>` to stderr. `srtspec explain <label>`
  shows the merged result. Settings that are the same on every machine are
  rendered from each service's `spec.access` manifest and ship beside its
  recipe in this repo: events, moondb, present serve, deps, status and
  speak-web. The rest come from the consuming repo. The launcher's rules and
  the bun measurements: `tools/srtspec/CLAUDE.md` and `docs/SANDBOXING.md`,
  "bun or node".
- **`hooks.builds.srtspec_chmod_srt_run`**: keeps `srt-run.sh` executable
  inside the sources cache. It is the step a registration hook orders
  against: ralph's `depends_on` takes only `builds.` and `packages.` keys, so
  a hook that starts a process through `srt-run` names this build and
  `packages.srtspec`.
- **`testdata/srt-local/`**: the expansion-layer cases (`ok-*` must launch,
  `refuse-*` must exit 1 before srt starts) that srtspec's launch and layers
  tests both run (`go test ./tools/srtspec/...`). Add a case there, not in
  a test.
- **`srt-settings-test.sh`**: run by `make test-recipes`, it checks every
  shipped `recipes/*/*.srt.json`. Each file needs the six lists srt
  requires, no host, and `denyRead` from `~`. No `allowWrite` may cover
  `~/.config/srt` or the sources cache. Its recipe must link it to
  `~/.config/srt/<label>.json` and launch through `srt-run`. Each t-man
  registration there needs the ledger `--extra-log` and the
  `builds.srtspec_chmod_srt_run` edge.
- Wave 0, with the service recipes. Their registration hooks name
  `packages.srtspec` and `builds.srtspec_chmod_srt_run` in `depends_on`,
  which orders them after this recipe within the wave (docs/adr/0006,
  2026-10-26 amendment names srtspec a platform foundation).
- The denial side stays with the speak recipe: the `sandbox-watch` agent,
  `sandbox-audit.sh`, the fleet ignore list and the ledger under
  `~/.local/share/speak/logs/` (`recipes/speak/CLAUDE.md`).

## See also

- Source + module notes: `tools/srtspec/CLAUDE.md`
- Authoring a manifest: `docs/SANDBOXING.md`, "Declaring access"
