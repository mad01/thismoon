# belt

Guard and hint hooks for Claude Code sessions. belt inspects tool calls and either denies the risky ones before they run or advises after them, always with a reason the agent can act on.

Named for the layer it adds: suspenders holds up the git side (pre-commit secret scanning and internal-name guard). belt holds up the session side, before anything reaches git.

belt has two halves, and the split is deliberate (see `docs/adr/0008`):

- **Guards** run as a PreToolUse hook and can deny. They are reserved for damage that is hard to undo.
- **Hints** only advise. Most run as a PostToolUse hook after the tool call whose result stands; two ride session lifecycle events instead (SessionStart and UserPromptSubmit). A hint that misfires costs a few lines of ignored text rather than a stalled session.

## Guards

Three guards run against tool calls before they execute:

- **git-push-main**: blocks `git push` to main/master on machines with the work profile. Personal machines push to main freely.
- **script-deny-list**: deep deny inspection, applies the Bash deny list from the Claude settings inside scripts. `bash cleanup.sh` looks harmless to the permission system even when the script runs `kubectl delete`; this guard reads executed and sourced script files, `-c` strings, and heredocs, and denies when they contain a deny-listed command. Extra patterns (like `rm -rf`) come from the belt config.
- **write-internal-names**: blocks file writes that would put internal org/repo names into a public github.com repo. Names come from the `internal_names` section of belt's own config; when that section is unset, belt falls back to the name config of the suspenders pre-commit guard.

## Hints

Six hints add advisory context the agent reads next to a tool result or at session boundaries:

- **prefer-csl**: after a bash command sweeps multiple files inside a repo csl already indexes, hands back the equivalent `csl_search` call with the pattern translated to zoekt syntax. Pipe filters (`cmd | grep x`) and single-file greps do not fire — they are not what csl replaces.
- **kof-assertions**: after a csl search, surfaces stored kof assertions about the code the search hit, so prior conclusions get read instead of re-derived. Caps at three, marks stale ones, and repeats nothing within a session.
- **kof-consult**: when a session opens inside a git repo, surfaces that repo's kof assertions before any searching happens, so the session starts from what earlier sessions concluded. Caps at five; outside a repo, or with kof down, it stays silent.
- **agent-memory**: when a session opens, injects the shared agent memory index (`~/.config/agent-memory/MEMORY.md`) so cross-agent facts arrive as context instead of relying on instruction prose to prompt a read.
- **kof-deposit**: once per session, nudges a session that did substantial work but never recorded a kof assertion to deposit what it derived before the conclusions evaporate with the context.
- **humanizer-check**: once per session, after a tool call publishes text to a system other people read, points at `humanizer_detect` while the wording can still be edited. A session that already ran a humanizer tool is left alone.

## Install

Built and installed by ralph (`recipes/belt/recipe.toml`); hook registration lives in the consuming repo's Claude settings recipe. To build and install manually:

```bash
make build    # produces ./belt binary
make install  # build + cp to ~/code/bin/belt + adhoc codesign
```

## Usage

Manual dry-runs, useful for checking what a guard would decide without going through a real Claude Code session:

```bash
belt check bash "git push origin main"
belt check bash "bash cleanup.sh"
belt check write --file README.md --content "mentions something internal"
belt doctor
belt config
belt version
belt version -o json
```

`belt doctor` prints the resolved state behind those decisions: the build that is installed (version, commit, release tag, build time), which config file loaded (or failed to parse), which guards and hints are enabled with their allow/exclude counts, whether the kof serve instance behind the kof-* hints is reachable and how many assertions it stores (the hints render "kof down" and "store empty" identically, as silence — doctor tells them apart), and the full blocked-name set the write-internal-names guard matches against. `belt config` prints the config file locations and every setting in effect after defaults and fallbacks are applied — guard and hint toggles, resolved profiles and internal_names whichever file supplied them, and the Claude-settings Bash deny patterns the script guard enforces; `belt config --help` carries the annotated reference of every setting — including how `allow_repos` exempts a repo from a guard and `exclude_paths` exempts paths. When a deny surprises you: `check` shows the verdict, `doctor` shows the state that produced it, `config` shows which file and key to change.

`belt version` prints the bare version token of the installed build. With `-o json` it prints the full build metadata — `version`, `commit`, `tag`, `build_time` — the same four keys every tool in this repo reports, so one probe can ask any of them what build is running.

`belt hook <event>` and `belt hint <event>` are the real hook entrypoints (payload on stdin, output on stdout); Claude Code invokes them, not the user. `hook` carries deny decisions for guards; `hint` carries `additionalContext` JSON for the search, bash, and session-start events, and plain stdout text for prompt (UserPromptSubmit adds stdout to context directly).

## Configuration

Toggles, `exclude_paths`, `extra_patterns`, and `allow_repos` live in `~/.config/belt/config.yaml`, under `guards.<id>` for guards and `hints.<id>` for hints (a legacy `config.toml` in the same directory is still read when no YAML file exists). Both default to enabled when the file or entry is missing. `allow_repos` exempts specific repositories from a guard by canonical `host/owner/repo` — for example `guards.git-push-main` with `allow_repos: [github.com/mad01/dotfiles]` permits direct pushes to the default branch in that repo while every other repo stays fail-closed. `allow_repos_by_profile` scopes an entry to machines carrying a ralph profile (`personal: [github.com/you/store]` allows the repo on personal machines only; everywhere else it stays fail-closed). Denials and hints are logged to the local events timeline (events.this).

## Develop

```bash
make build
make install
make test
make lint
```
