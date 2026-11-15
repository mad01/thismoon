# harvest: recipe

Builds and installs the `harvest` CLI (`tools/harvest`), links the launcher
its SessionEnd hook runs, links the srt settings its steps run under, and
ships the harvest agent skill from the repo-root `skills/harvest/` dir.

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`), and
ralph merges the recipe with the identity `thismoon/harvest`.

## What this recipe does

- **`packages.harvest`**: builds the binary and installs it to
  `~/code/bin/harvest`.
- **`dotfiles.harvest_env`**: symlinks `harvest-env.sh` to
  `~/code/bin/harvest-env`. The script pulls the provider variables the model
  step reads from the secrets file, then execs harvest.
- **`dotfiles.harvest_step`**: symlinks `harvest-step.sh` to
  `~/code/bin/harvest-step`. `harvest run` starts each step through it, and
  it execs srt-run with the settings below.
- **`hooks.builds.harvest_chmod_scripts`**: keeps both scripts executable
  in the sources cache, after the step that keeps srt-run executable.
- **`dotfiles.harvest_srt_settings`**: symlinks `harvest.srt.json` to
  `~/.config/srt/harvest.json`. It is rendered from `spec.access` in
  `tools/harvest/service-info.yaml` by `make sandbox-settings`; edit the
  manifest, not the JSON.
- **`dotfiles.harvest_skill`** and **`dotfiles.harvest_codex_skill`**:
  symlink `skills/harvest/` into `~/.claude/skills/harvest` and
  `~/.agents/skills/harvest`. `harvest run` reads the same `SKILL.md` as the
  model's system prompt.

No profile filter: the recipe lands on every machine.

## What the consuming repo adds

The recipe registers no hook. A machine turns harvest on with three
machine-private pieces:

- **The SessionEnd hook.** In the Claude Code settings template, a
  `SessionEnd` command hook that runs `~/code/bin/harvest-env session-end`.
- **The belt hint.** A `custom_hints.harvest` entry in the belt config, with
  `event: session-start` and `command: [harvest, hint]`. belt expands no `~`
  in a command, so `harvest` must be on the PATH belt runs with.
- **The vault grants.** `~/.config/srt/harvest.local.json`, the machine's
  layer over the shipped settings. The apply step runs `brain capture`, which
  writes the note and commits it, so the layer grants:
  - read and write on the vault
  - read on the real target of `~/.config/brain/config.yaml` when that file
    is a link
  - read on the real target of `~/.gitconfig` when it is a link, on every
    file it includes, and on the global gitignore
  - read on what the vault's own commit hook reads, such as the secret
    scanner's config and the files that config includes

  Without the vault grant the apply step cannot write a capture. Without the
  git grants brain writes the note and the commit fails, which the log shows
  as a `failed:` line. Find a missing grant in the denial ledger
  (`docs/SANDBOXING.md`, "Denial triage") and add only what a denial shows.

## What it needs

`brain` on PATH (the `brain` recipe), and `srt-run` at `~/code/bin/srt-run`
(the `srtspec` recipe) unless the machine sets `HARVEST_SANDBOX=off`. With
the sandbox on and either missing, every harvest fails closed with a `fail`
line in the log.

## See also

- The tool: `tools/harvest/CLAUDE.md`, and `harvest docs` for runtime
  behavior
- Skill content: `skills/harvest/SKILL.md`
- Sandboxing: `docs/SANDBOXING.md`
