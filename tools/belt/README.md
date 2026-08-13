# belt

Guard and hint hooks for Claude Code sessions. belt inspects tool calls and either denies the risky ones before they run or advises after them, always with a reason the agent can act on.

Named for the layer it adds: suspenders holds up the git side (pre-commit secret scanning and internal-name guard). belt holds up the session side, before anything reaches git.

belt has two halves, and the split is deliberate (see `docs/adr/0008`):

- **Guards** run as a PreToolUse hook and can deny. They are reserved for damage that is hard to undo.
- **Hints** run as a PostToolUse hook and only advise. The tool has already run and its result stands, so a hint that misfires costs one line of ignored text rather than a stalled session.

## Guards

Three guards run against tool calls before they execute:

- **git-push-main**: blocks `git push` to main/master on machines with the work profile. Personal machines push to main freely.
- **script-deny-list**: deep deny inspection, applies the Bash deny list from the Claude settings inside scripts. `bash cleanup.sh` looks harmless to the permission system even when the script runs `kubectl delete`; this guard reads executed and sourced script files, `-c` strings, and heredocs, and denies when they contain a deny-listed command. Extra patterns (like `rm -rf`) come from the belt config.
- **write-internal-names**: blocks file writes that would put internal org/repo names into a public github.com repo. Uses the same name config as the suspenders pre-commit guard.

## Hints

Two hints run after a tool call and add advisory context the agent reads next to the result:

- **prefer-csl**: after a bash command sweeps multiple files inside a repo csl already indexes, hands back the equivalent `csl_search` call with the pattern translated to zoekt syntax. Pipe filters (`cmd | grep x`) and single-file greps do not fire — they are not what csl replaces.
- **keep-assertions**: after a csl search, surfaces stored keep assertions about the code the search hit, so prior conclusions get read instead of re-derived. Caps at three, marks stale ones, and repeats nothing within a session.

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

`belt doctor` prints the resolved state behind those decisions: the build that is installed (version, commit, release tag, build time), which config file loaded (or failed to parse), which guards and hints are enabled with their allow/exclude counts, and the full blocked-name set the write-internal-names guard matches against. `belt config` prints the config file locations and an annotated reference of every setting — including how `allow_repos` exempts a repo from a guard and `exclude_paths` exempts paths. When a deny surprises you: `check` shows the verdict, `doctor` shows the state that produced it, `config` shows which file and key to change.

`belt version` prints the bare version token of the installed build. With `-o json` it prints the full build metadata — `version`, `commit`, `tag`, `build_time` — the same four keys every tool in this repo reports, so one probe can ask any of them what build is running.

`belt hook <event>` and `belt hint <event>` are the real hook entrypoints (payload on stdin, JSON on stdout); Claude Code invokes them, not the user. `hook` carries deny decisions for guards, `hint` carries `additionalContext` for hints.

## Configuration

Toggles, `exclude_paths`, `extra_patterns`, and `allow_repos` live in `~/.config/belt/config.yaml`, under `guards.<id>` for guards and `hints.<id>` for hints (a legacy `config.toml` in the same directory is still read when no YAML file exists). Both default to enabled when the file or entry is missing. `allow_repos` exempts specific repositories from a guard by canonical `host/owner/repo` — for example `guards.git-push-main` with `allow_repos: [github.com/mad01/dotfiles]` permits direct pushes to the default branch in that repo while every other repo stays fail-closed. Denials and hints are logged to the local events timeline (events.this).

## Develop

```bash
make build
make install
make test
make lint
```
