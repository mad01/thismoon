# Example: consuming thismoon from a dotfiles repo

A minimal dotfiles repo layout that pulls this monorepo's recipes in as a
remote source and layers machine-private wiring on top. This is the two-layer
model from `docs/adr/0006`: thismoon ships the portable recipes (build,
install, hooks, skills); the consuming repo owns everything machine-private.
Copy it, point ralph at it, and `ralph up` gives you a machine whose services,
agent settings, mods and MCP servers come from this repo and stay in step
with it.

```
your-dotfiles/
  config.toml                # ralph config: [[recipe_sources]] points at thismoon,
                             #   plus a host-scoping override example
  config.local.toml.example  # template for the machine-local profiles file
  CLAUDE.md.example          # a global ~/.claude/CLAUDE.md tuned for the fleet
  recipes/
    agent-config/    # overlay: Claude Code settings, mods and MCP servers, applied by agent-sync
    mcp-sandbox/     # overlay: a third-party MCP server under srt (manifest, rendered settings, wrapper), via agent-sync's drop-in dir
    agent-shell/     # overlay: a coding agent's shell tool under srt (manifest, rendered settings, wrapper, watcher ignore line)
    humanizer/       # overlay: the srt settings for a thismoon MCP server whose binary and wrapper ship from the source
    mise/            # overlay: the toolchain mise installs per user, the srt and bun pins included
    claude-hooks/    # overlay: belt config + the internal-names file belt and suspenders share
    d-man/           # overlay: machine-personal routes for the d-man front door
    config-overlay/  # overlay: per-machine service config (watch list, host gate)
    csl-config/      # overlay: which directories csl indexes on this machine
    agent-memory/    # overlay: clone the shared memory store belt injects
    secrets-env/     # overlay: secret/env wiring without committing values
```

`config.local.toml` sits beside the ralph config on each machine and selects
that machine's profiles. It is machine-local and gitignored: this repo tracks
only `config.local.toml.example` as a template, and `ralph profile set` writes
the real file per machine.

## The split

| Layer | Lives in | Examples |
|-------|----------|----------|
| Portable recipes | thismoon `recipes/` | build + install, service registration hooks, skills, `SETUP.md` guides |
| Machine-private wiring | your dotfiles repo | the `[[recipe_sources]]` pin, and the overlay patterns below |

Hard `depends_on` across the boundary is allowed only onto the platform
foundations (`packages.t_man`, `packages.d_man`); see `docs/adr/0006`.

Each kind of machine-private wiring has a worked example in `recipes/`:

| Pattern | Example | What it owns |
|---------|---------|--------------|
| Agent config: settings, mods, MCP | [`recipes/agent-config/`](recipes/agent-config/) | the Claude Code settings template and its machine overlay, the mods list, the MCP server set, and the agent-sync config that names them; synced on every `ralph up` |
| Sandboxed MCP server | [`recipes/mcp-sandbox/`](recipes/mcp-sandbox/) | the access manifest, the srt settings srtspec renders from it (linked to `~/.config/srt/<label>.json`) and the launch wrapper for a third-party server, registered through agent-sync's drop-in dir instead of the base file |
| Agent shell under srt | [`recipes/agent-shell/`](recipes/agent-shell/) | the policy every command a coding agent runs gets: the access manifest, the settings srtspec renders from it (linked to `~/.config/srt/agent-shell.json`), the wrapper the agent's tool spawns in place of `bash -c`, and the watcher ignore line that keeps the agent's denials in the ledger only |
| Settings for a thismoon MCP server | [`recipes/humanizer/`](recipes/humanizer/) | the access manifest and rendered settings for humanizer; its binary and spawn wrapper ship from the source. The code roots it may read are this machine's choice, so the manifest is yours (`docs/adr/0006`) |
| Toolchain (mise) | [`recipes/mise/`](recipes/mise/) | the runtimes and CLIs mise installs per user, with srt pinned exactly and bun for the srt launcher |
| Agent hooks (belt) | [`recipes/claude-hooks/`](recipes/claude-hooks/) | the belt config and the internal-names file belt and suspenders share; the hooks block that registers belt sits in the settings template |
| Routes + block list | [`recipes/d-man/`](recipes/d-man/) | which `.this` names resolve on this machine, and which hosts the work profile blocks |
| Config overlay | [`recipes/config-overlay/`](recipes/config-overlay/) | per-machine service config: watch lists, host gates, local paths |
| csl index list | [`recipes/csl-config/`](recipes/csl-config/) | which directories csl walks for git checkouts, and the remote-host allowlist if you need one |
| Agent memory store | [`recipes/agent-memory/`](recipes/agent-memory/) | the private facts repo cloned to `~/.config/agent-memory`, with the index and fact-file format belt injects at session start |
| Secrets / env | [`recipes/secrets-env/`](recipes/secrets-env/) | a service's secret, resolved at runtime or from a gitignored env-file, never committed |
| Host-scoping | [`config.toml`](config.toml) | enabling a remote recipe on a subset of machines (`hosts = [...]`) |

## How the pieces fit

1. `config.toml` declares the thismoon source. ralph clones it to
   `~/.config/ralph/sources/thismoon` and discovers every `recipes/*/recipe.toml`
   there, merged under the identity `thismoon/<recipe>`.
2. Your local `recipes/` load as usual. Overlay recipes keep only the items the
   machine owns. The binary always ships from the thismoon source; the overlay
   layers wiring on top. `recipes/d-man/` symlinks a personal route set,
   `recipes/config-overlay/` a service's config, `recipes/secrets-env/` a
   secret resolved at runtime, and `recipes/agent-config/` the data agent-sync
   merges into Claude Code's settings, mods and MCP registrations.
3. Item keys are global across all loaded recipes: an overlay must not
   redeclare a key the thismoon recipe already defines (ralph rejects it).
4. To disable or host-scope a remote recipe, use the namespaced override key:
   `enable = false` turns it off everywhere, `hosts = [...]` runs it only on the
   listed machines. `config.toml` carries a worked block; the short form is:

```toml
[recipes_config.overrides."thismoon/catalog"]
enable = false
```

   Recipe-level `hosts`/`profiles` gate a recipe you own (see
   `recipes/secrets-env/recipe.toml`, which only applies on a `personal` host);
   the override above gates a recipe that ships from a source. A second,
   profile-gated source (a work overlay repo, say) can carry the same table in
   an `overrides.toml` at its root. So its machines get their own answer
   without this repo restating it.

## Agent config through agent-sync

`recipes/agent-config/` holds three data files and the agent-sync config that
names them. agent-sync is a thismoon CLI (`tools/agent-sync`) with one section
per kind of data; each section runs in three modes. `sync` applies, `plan`
prints what sync would do and writes nothing, and `check` writes nothing and
exits 1 on drift, with a one-line summary first. The recipe runs `sync` as a
`run = "always"` build hook on every `ralph up` and `check` as that hook's
`verify`, so `ralph doctor` shows the summary line per section.

| Data | File | Reaches | How |
|------|------|---------|-----|
| Claude Code settings | `settings.json`, the template; `settings.local.json.example`, the machine's overlay, copied by hand | `~/.claude/settings.json` | template merged over the live file, overlay merged last |
| Mods | `mods.json`: one marketplace, each mod on, off or retired, with `profiles` and `hosts` gates | Claude Code's plugin state | `claude plugin` calls for the difference |
| MCP servers | `servers.json`, plus drop-ins in `~/.config/claude-mcp/servers.d/` | Claude Code's user-scope registrations | `claude mcp` calls for the difference |

The settings template carries the hooks block that registers belt, the
permission lists and the other keys every machine shares. It never carries
`enabledPlugins`: the mods section owns that key through the claude CLI, and a
template value would put it back on the next settings sync. Machine-only
settings (an extra allowed command, a local URL) go in the overlay, whose
permission lists union with the template's instead of replacing them.

Drop-in directories let another recipe or a second recipe source contribute
without editing this repo's files. `recipes/mcp-sandbox/` symlinks its
`gh-com.json` into `~/.config/claude-mcp/servers.d/`, and the mcp sync
merges it over `servers.json` by server name. `~/.config/claude-mods/mods.d/`
works the same way for a second marketplace. Both paths are agent-sync's
defaults; the `dropins` key in each section of `config.yaml` moves one.

### Ordering

The agent-sync binary ships from the thismoon source (`thismoon/agent-sync`,
wave 0, build and install only). The overlay can't `depends_on` it: a
cross-source dependency is reserved for t-man and d-man (`docs/adr/0006`).
Waves are the ordering instead. ralph finishes every wave-0 package before any
wave-1 build starts, and it pulls the sources cache before any wave runs. So
the wave-1 sync hooks in `agent-config` always find the binary, on the first
`ralph up` of a new machine included. The hooks name it by install path
(`$HOME/code/bin/agent-sync`) so ralph's PATH plays no part. This needs no
guard and no second run.

## Bootstrapping a new machine

1. Install ralph and Claude Code, clone your copy of this layout, and run
   `ralph init` with `dotfiles_repo_path` pointing at the clone (thismoon
   `docs/GETTING-STARTED.md`, "The fleet with ralph").
2. Declare the machine's profiles before the first `ralph up`: `ralph profile
   set personal` (or `work`, or both). This writes `config.local.toml`, which
   gates your profile-scoped recipes and which agent-sync reads for the
   `profiles` entries in `mods.json` and `servers.json`.
3. Optional: copy `recipes/agent-config/settings.local.json.example` to
   `~/.claude/settings.local.json` and edit it. agent-sync merges the overlay
   when it exists and skips it otherwise.
4. `ralph up`. Wave 0 builds every thismoon component into `~/code/bin`,
   agent-sync among them. Wave 1 symlinks the configs and runs the three
   syncs. The settings sync writes `~/.claude/settings.json` from the
   template, which turns belt's hooks on. The mods sync registers the
   thismoon marketplace and installs the mods. The mcp sync registers the
   servers. New Claude Code sessions see all of it.
5. The one sudo step: register the d-man daemon (thismoon
   `recipes/d-man/SETUP.md`). Then verify with `ralph doctor` (one line per
   sync: in sync, or drift and why), `agent-sync doctor`, `claude mcp list`
   and `claude plugin list`.

## Updates

What `ralph up` does for each kind of agent data, and how a retirement
reaches every machine. The sources-cache pull comes first on every run, so a
merged thismoon change (a new mod version, a new agent-sync build) is already
on disk when the syncs run.

### Settings

`agent-sync settings sync` merges the template into the live file. The
template wins, the live file keeps its own extra keys, and the overlay merges
last. So a key you drop from the template stays on every machine. To retire
it, add its key path to `settings.prune` in `config.yaml`
(`[hooks, Notification]`). The next sync deletes it everywhere, and `check`
reports drift until then. A prune path the template still sets is an error.
Drop the entry once every machine has synced.

### Mods

`agent-sync mods sync` compares `mods.json` with Claude Code's plugin state
and runs `claude plugin` for the difference. An `"enabled": true` mod is
installed and enabled; an `"enabled": false` one is disabled. A mod whose
install record's version differs from the `plugin.json` in the sources cache
is updated. The marketplace is that cache, so a session runs each mod from its
folder there and the pull is what delivers a change; a new session or
`/reload-plugins` loads it. To retire a mod, set `"installed": false`: the
next sync uninstalls it wherever it has an install record, even after the
marketplace stops listing it. The marketplace directory must still be on disk:
without it the whole file is skipped with a note, retirements included. Run
`ralph up` on every machine, then delete the entry; a mod absent from the file
is never touched.

### MCP servers

`agent-sync mcp sync` adds servers missing from `claude mcp list` and re-adds
a server whose registered spec differs. A remote server compares by url alone,
so a login done by hand survives. With `prune: false`, this example's setting,
a server dropped from the files stays registered; remove it with
`claude mcp remove --scope user <name>` on each machine. With `prune: true` in
`config.yaml`, the next sync removes every user-scope server the files don't
name, servers added by hand included. It skips a claude.ai connector, a
plugin's server, a server in another scope and a name `claude mcp add`
would reject, and names them in a note.
Prune also turns the stamp shortcut off, so every sync and check runs the
per-server spec check.

`ralph doctor` runs each section's `check` and shows its first line; drift is
a warning that the next `ralph up` clears.

## Migrating from hand-written sync scripts

An earlier version of this example carried two hand-written sync scripts and
a settings block merged by hand. `recipes/agent-config/` replaces all three:

| Before | After |
|--------|-------|
| `recipes/claude-mods/`: `mods.list`, one `<name> on\|off\|retired` line per mod, applied by `sync-mods.sh` in the `claude_mods_sync` hook, verified by `sh ./sync-mods.sh --check` | `mods.json`, where `on` is `"enabled": true`, `off` is `"enabled": false` and `retired` is `"installed": false`; hook `agent_mods_sync`, verified by `agent-sync mods check` |
| `recipes/mcp-registration/`: `servers.json` (stdio entries only), applied by `register.py` in the `mcp_register` hook, which removed and re-added every server on each run, with no verify step | `servers.json` in the same shape; hook `agent_mcp_sync`, verified by `agent-sync mcp check`. The sandbox wrapper moved to `recipes/mcp-sandbox/`, which adds its server as a drop-in |
| `recipes/claude-hooks/settings.hooks.json.example`, the hooks block that registers belt, merged into `~/.claude/settings.json` by hand | `settings.json`, the template; hook `agent_settings_sync`, verified by `agent-sync settings check` |

The example renamed its hooks when it folded the two recipes into one. The
steps for your own repo, from writing the config through rolling it out, are
in [tools/agent-sync/migrating.md](../../tools/agent-sync/migrating.md).

## Runtime profiles: block-list switching (d-man)

`recipes/d-man/` also shows a runtime toggle, distinct from the build-time
`hosts`/`profiles` gates above. The d-man block list lives in the work profile
only, driven by macOS Focus:

- `routes.base.toml` holds the shared routes; `blocklist.work.toml` holds the
  blocked hosts. `d-man profile`, a subcommand of the d-man binary,
  **generates** the active `~/.config/d-man/routes.toml` (nothing symlinks
  it), prepending the block list for the work profile and omitting it for
  personal. It also rewrites a `~` in `games_dir` to an absolute path, because
  the daemon runs as root.
- `d-man profile work` / `personal` rewrites the active file atomically; the
  running daemon reloads it via fsnotify, so no sudo or restart is needed. State
  lives in `~/.config/d-man/profile`, and a build hook re-applies the current
  profile on every `ralph up`. A switch that would write invalid routes, or a
  profile with no block-list file, changes nothing.
- Wire it to macOS **Work Focus** with a Shortcuts automation (run
  `$HOME/code/bin/d-man profile work` when Work turns on, `personal` when it
  turns off) so blocking follows work mode. Blocking HTTPS hosts also needs the
  local CA trusted once: `sudo d-man ca install`.

## Updating thismoon

How a thismoon change reaches a machine wired up like this:

1. A fix merges to thismoon's main.
2. On the next `ralph up`, the source checkout pulls main (the example
   `config.toml` sets `ref = "main"`, `update = true`), the changed component
   rebuilds from source, and its service restarts through t-man.
3. Verify with the service's `/version` endpoint (or `<tool> version` for
   CLIs). ralph can report ok while an old binary keeps running, so check the
   version, not the run report.

Merging to main is the deploy; there is nothing to bump in the consuming
repo. To move slower than main, pin `ref` to a tag or commit:

```toml
[[recipe_sources]]
name = "thismoon"
url = "git@github.com:mad01/thismoon.git"
ref = "present/v1.2.3"   # any git ref pins the whole checkout at that commit
update = true
```

The pin stays put until you edit the config; changing it moves the checkout
on the next run. Note the ref pins the entire monorepo checkout, not just the
named component.

thismoon's per-component GitHub Releases (`present/v1.2.3` tags, tarballs,
cosign signatures; see `docs/RELEASING.md` in the repo root) serve
`go install` and manual downloads. A fleet wired through ralph builds from
source and never touches those artifacts.

## Trying it

Copy `config.toml`, `config.local.toml.example` and `recipes/` into a fresh
repo. Adjust `dotfiles_repo_path` and the `~/code/src/github.com/you/dotfiles`
paths the recipes and `recipes/agent-config/config.yaml` use, copy
`config.local.toml.example` to `config.local.toml` beside your ralph config
and set your machine's profiles, then run `ralph up --dry-run`. The dry run
previews every item, the agent-sync hooks included, without running them.

One-time steps stay manual by design: the d-man daemon registration needs sudo
once per machine (`recipes/d-man/SETUP.md` in this repo). Trusting the d-man
block-page CA needs `sudo d-man ca install` once (only if you use a block
list). The `secrets-env` pattern needs a `~/.config/myservice/secrets.env`
created by hand from the committed `.example` (the recipe never writes the
value). The settings overlay, `~/.claude/settings.local.json`, is machine-local
in the same way.
