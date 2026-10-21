# Migrating to agent-sync

This guide is for a dotfiles repo that uses ralph to keep Claude Code's
settings, mods and MCP servers in step across machines. Its own sync scripts
do that work today, or you do it by hand, and you want agent-sync to take
it over. Each section is one task:

- [Cut over from sync scripts to agent-sync](#cut-over-from-sync-scripts-to-agent-sync)
- [Retire a settings key](#retire-a-settings-key)
- [Uninstall a mod](#uninstall-a-mod)
- [Prune MCP servers](#prune-mcp-servers)

Every agent-sync section has three modes. `sync` applies the change, `plan`
prints it and writes nothing, and `check` writes nothing and exits 1 on
drift, with a one-line summary first. Exit 2 means a `claude` command
failed. The commands below call the binary by its install path,
`~/code/bin/agent-sync`. [config.md](config.md) lists every key, and
[operating.md](operating.md) the failure modes.

## Cut over from sync scripts to agent-sync

You need the thismoon recipe source in your ralph config (a
`[[recipe_sources]]` stanza, as in
[examples/dotfiles/config.toml](../../examples/dotfiles/config.toml)), and
Claude Code on every machine that syncs mods or MCP servers.

1. Run `ralph up` to install the binary. The `thismoon/agent-sync` recipe
   builds and installs `~/code/bin/agent-sync` in wave 0, and never runs a
   sync itself. Then run:

   ```bash
   ~/code/bin/agent-sync version
   ```

   It prints the build's version. Without ralph, `make install` in
   `tools/agent-sync` installs the same binary.

2. Keep the data files. agent-sync reads the data your scripts read:

   | Section | File | Shape |
   |---------|------|-------|
   | settings | the template `settings.json` | unchanged |
   | mods | `mods.json` | `{"marketplace": {"name", "path"}, "mods": {<name>: {"enabled", "installed", "hosts", "profiles"}}}` |
   | mcp | `servers.json` | `{"servers": {<name>: <spec>}}`, one entry per `claude mcp add --scope user` |

   A mods list in another format maps onto `mods.json`: a mod that is on
   is `"enabled": true` (the default), off is `"enabled": false`, and
   retired is `"installed": false`. Take `enabledPlugins` out of the
   settings template if it has one. The mods section owns that key through
   `claude plugin`, and a template value would put it back on every
   settings sync. If you kept this config by hand, write the files from a
   machine you trust: `claude mcp get <name>` prints a server's spec, and
   `claude plugin list --json` lists the installed mods.

3. Write a `config.yaml` beside the data, naming each file:

   ```yaml
   settings:
     template: ~/code/src/github.com/you/dotfiles/recipes/agent-config/settings.json
   mods:
     file: ~/code/src/github.com/you/dotfiles/recipes/agent-config/mods.json
   mcp:
     servers: ~/code/src/github.com/you/dotfiles/recipes/agent-config/servers.json
   ```

   A path your script hardcoded becomes a key: `settings.live` and
   `settings.overlay`, the drop-in directories (`mods.dropins`,
   `mcp.dropins`), the profiles file (`profiles_file` in mods and mcp) and
   the MCP stamp (`mcp.stamp`). Leave a key out to take its default
   ([config.md](config.md) lists them). A section you leave out stays
   yours: its commands fail with `section missing`, and doctor skips it.

   Link the file to `~/.config/agent-sync/config.yaml`, the default path,
   with a `[dotfiles.<key>]` symlink item (the example's
   [recipes/agent-config/recipe.toml](../../examples/dotfiles/recipes/agent-config/recipe.toml)
   has one), or point `--config` or `$AGENT_SYNC_CONFIG` at it. Then run:

   ```bash
   ~/code/bin/agent-sync doctor
   ```

   Success is an `ok` line per check, ending in
   `all checks passed; for semantics and gotchas run 'agent-sync docs'`.
   A failed check prints `FAIL <check>: <error>`. When the config sets a
   key wrong, the error names the file, line and key at fault.

4. Prove parity before you swap. Pick a machine the scripts left in
   sync, and run each script's own check there first, where it has one,
   to confirm it is clean. Then run agent-sync's checks:

   ```bash
   ~/code/bin/agent-sync settings check
   ~/code/bin/agent-sync mods check
   ~/code/bin/agent-sync mcp check
   ```

   Each one exits 0 and prints its in-sync line:

   ```
   settings already in sync
   mods already in sync (2 enabled, 1 disabled, 1 retired, 0 gated off; marketplace thismoon -> /Users/you/.config/ralph/sources/thismoon)
   mcpServers already in sync (9 tracked) [cached]
   ```

   The `[cached]` suffix means the script's MCP stamp matches the desired
   set, so check compared server names only. The mcp parity test is `plan`, which
   compares every server's spec:

   ```bash
   ~/code/bin/agent-sync mcp plan
   ```

   It prints `mcpServers already in sync (9 tracked)` when every spec
   matches. `check` and `plan` write nothing, so they are safe while the
   scripts still own the machine. A check that exits 1, or a plan that
   lists a server, means agent-sync reads your data differently from the
   script. Run that section's `plan` to see the difference, and fix the
   config or the data, not the machine. Some differences are expected:

   - A server with `env`, or with an argument that holds a space, shows as
     `~ <name>` in every `mcp plan`. With no stamp, or one that does not
     match, `mcp check` compares every spec too and exits 1 on such a
     server, with `drift: 1 MCP server to update`. This matches the original
     sync script, so it is not a parity gap.
   - A `settings.local.json` beside the live settings file is merged as the
     overlay. If your script ignored it, its keys show as drift: move the
     file, or set `settings.overlay`.
   - A live settings file that is a symlink reports drift until the first
     sync replaces it with a regular file.

5. Swap the recipe commands. Each script call becomes
   `agent-sync <section> sync`, and its check or dry-run flag becomes
   `agent-sync <section> check` in `verify`:

   ```toml
   # before
   [hooks.builds.claude_mods_sync]
   commands = ["sh ./sync-mods.sh"]
   working_dir = "~/code/src/github.com/you/dotfiles/recipes/claude-mods"
   run = "always"
   verify = "sh ./sync-mods.sh --check"

   # after
   [hooks.builds.claude_mods_sync]
   commands = [
     "if command -v claude >/dev/null; then $HOME/code/bin/agent-sync mods sync; else echo 'skip mods sync (claude not on PATH)'; fi",
   ]
   working_dir = "~/code/src/github.com/you/dotfiles/recipes/agent-config"
   run = "always"
   verify = "$HOME/code/bin/agent-sync mods check"
   ```

   Point `working_dir` at the directory that holds `config.yaml`. The
   script's directory may be empty once you delete the scripts, and
   `ralph doctor` fails a hook whose `working_dir` does not exist.
   Keep `run = "always"`. ralph's `idempotent` flag hashes the command
   string, so it would miss every edit to the data files. Keep a
   `command -v claude` guard on the mods and mcp hooks, which sync through
   the claude CLI; settings needs none. Name the binary by its install path
   so the hook does not depend on ralph's PATH. A hook without a verify
   step gets one now: `ralph doctor` runs it and shows
   `verified: <first line>` when it passes.

6. Run the hooks in wave 1 or later. The binary builds in wave 0 from
   the thismoon source. Your recipe cannot `depends_on` `packages.agent-sync`:
   a dependency across sources is reserved for t-man and d-man (thismoon
   `docs/adr/0006`). The wave is the ordering instead. ralph pulls the
   sources cache before any wave, and finishes wave 0 before wave 1
   starts. Recipe items default to wave 1, but items in ralph's main
   `config.toml` run in wave 0. Keep the hooks in a recipe, and leave its
   `wave` unset or at 1 or higher. A hook that runs before the binary
   exists fails like this:

   ```
   sh: /Users/you/code/bin/agent-sync: No such file or directory
   command failed: $HOME/code/bin/agent-sync settings sync: exit status 127
   ```

   With the wave right, the first `ralph up` on a new machine builds the
   binary and then runs the syncs, in that one run.

7. Delete the scripts. Remove them and their tests in the same
   commit as the recipe change. If the change deletes a whole recipe, drop
   its `pre_uninstall` and `post_uninstall` hooks first when they remove
   MCP servers or uninstall mods: with cleanup on, ralph runs a deleted
   recipe's uninstall hooks after the builds, so they would undo the sync.
   `ralph up --dry-run` previews every item, the agent-sync hooks
   included, without running them.

8. Roll out. On each machine, one `ralph up` pulls the sources cache,
   builds agent-sync and runs the syncs. Then run `ralph doctor`: each sync
   hook shows `verified: ` followed by its section's in-sync line. Drift is
   a warning, cleared by the next `ralph up`; run that section's `plan` to
   see what it is. The first sync on a machine can change a few things the
   scripts left alone:

   - A live `settings.json` that was a symlink into the repo becomes a
     regular file. The template stays where it is.
   - A machine with no live settings file gets the template with the
     overlay merged.
   - A marketplace registered at another path is registered again at the
     path in `mods.json`, shown as `+ marketplace thismoon -> <path>`.

## Retire a settings key

Dropping a key from the template leaves it in every live file, because the
merge keeps keys that only the live file has. `settings.prune` deletes it.

1. Remove the key from the template.

2. Add its key path to `settings.prune` in `config.yaml`, with a comment
   saying what retired it. Each entry is a list of keys from the root, so a
   key with a dot in it stays one key:

   ```yaml
   settings:
     template: ~/code/src/github.com/you/dotfiles/recipes/agent-config/settings.json
     prune:
       - [includeCoAuthoredBy]   # superseded by attribution
       - [hooks, Notification]   # moved to the belt hooks
   ```

3. Preview on a machine that still has the key:

   ```
   $ ~/code/bin/agent-sync settings plan
   pruned (retired keys):
     - includeCoAuthoredBy: false
   ```

   A path the template still sets stops every mode before it writes
   anything, with exit 1:

   ```
   error: settings.prune retires keys the template still sets: includeCoAuthoredBy (drop them from the template or from settings.prune)
   ```

4. Commit and run `ralph up` on each machine. Until a machine syncs, its
   check counts the key as drift:

   ```
   drift: 1 key differs from template (run 'ralph up' to merge)
   ```

   After the sync, check prints `settings already in sync`.

5. Delete the entry once every machine has synced. Left in place it costs
   nothing on a machine without the key, but it stops the template from
   setting that key again (the error in step 3). A machine that missed the
   sync keeps the key once the entry is gone.

Prune covers the template and the live file, not the overlay: an overlay
that sets a pruned key merges it back in after the prune, with no error.

## Uninstall a mod

Deleting a mod's entry from `mods.json` does nothing: the sync never
touches a mod the files do not name. Retire it first.

1. Set `"installed": false` on the entry, and leave `enabled` out:

   ```json
   "loom-pane": {
     "_comment": "retired: delete this entry once every machine has synced",
     "installed": false
   }
   ```

   `"enabled": true` beside it is an error, since a mod cannot be both
   uninstalled and enabled. agent-sync checks every entry before it applies
   the `hosts` and `profiles` gates, so the error stops the sync on every
   machine:

   ```
   error: /Users/you/code/src/github.com/you/dotfiles/recipes/agent-config/mods.json: mod 'loom-pane' sets 'installed' false and 'enabled' true; drop 'enabled' or set it false
   ```

2. Preview:

   ```
   $ ~/code/bin/agent-sync mods plan
   - uninstall loom-pane@thismoon
     $ /Users/you/.local/bin/claude plugin uninstall loom-pane@thismoon --scope user
   (dry-run, no changes applied)
   ```

   The marketplace may have dropped the mod already: a retired mod is
   uninstalled whether the marketplace lists it or not. The marketplace
   directory itself must still be on disk, because agent-sync skips a mods
   file whose marketplace is missing, with a note.

3. Commit and run `ralph up` on each machine. Wherever the mod has a
   user-scope install record, the sync runs
   `claude plugin uninstall <mod>@<marketplace> --scope user`. A gate
   still applies: a retired mod that `hosts` or `profiles` keeps off a
   machine is left alone there. After the sync, check counts the mod as
   retired:

   ```
   mods already in sync (1 enabled, 0 disabled, 1 retired, 0 gated off; marketplace thismoon -> /Users/you/.config/ralph/sources/thismoon)
   ```

4. Delete the entry once every machine has run `ralph up`. A machine that
   missed it keeps the mod installed until someone runs the uninstall there
   by hand.

## Prune MCP servers

`mcp.prune` defaults to `false`, and then the sync only adds and updates.
A server you drop from the files stays registered on every machine, and
`check` does not count it: the in-sync line covers only the servers the
files name. Remove it by hand on each machine:

```bash
claude mcp remove --scope user <name>
```

To have the sync remove such servers instead:

1. Preview what prune would remove. Copy `config.yaml`, set `prune: true`
   in the copy, and run plan against it:

   ```
   $ ~/code/bin/agent-sync --config /tmp/agent-sync-prune.yaml mcp plan
   - oldtool
     $ /Users/you/.local/bin/claude mcp remove --scope user oldtool
   ```

   Every `- <name>` line is a server prune would remove: every name
   `claude mcp list` shows that the files do not name for this machine.
   That includes servers you added by hand, and servers whose `hosts` or
   `profiles` gate keeps them off this machine.

2. Look for names prune cannot remove. It removes at user scope only, so
   these fail the sync with exit 2:

   - a claude.ai connector, listed as `claude.ai <Name>`
   - a server a plugin provides, listed as `plugin`
   - a local- or project-scope server visible from the directory the sync
     runs in

   The failed remove prints `error: remove '<name>' failed: ` and claude's
   stderr. Adds and updates run first, then removes in name order, and the
   run stops at the failure: the servers after it stay registered, and it
   writes no stamp. On a machine that has one of these names, prune never
   finishes. Remove those servers where they live, or keep prune off there
   and remove stale servers by hand.

3. Set `prune: true` in the real `config.yaml`, commit, and run `ralph up`
   on each machine. Success is an `agent-sync mcp check` with no
   `- <name>` lines.

4. Turn prune off again once every machine has run. With prune on, sync
   and check compare every spec, so a server with `env` or a spaced
   argument drifts on every run.
