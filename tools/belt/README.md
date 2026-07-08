# belt

Guard hooks for Claude Code sessions. belt runs as a PreToolUse hook, inspects a tool call before it executes, and denies risky ones with a reason the agent can act on.

Named for the layer it adds: suspenders holds up the git side (pre-commit secret scanning and internal-name guard). belt holds up the session side, before anything reaches git.

## How it works

Three guards run against tool calls before they execute:

- **git-push-main**: blocks `git push` to main/master on machines with the work profile. Personal machines push to main freely.
- **script-deny-list**: deep deny inspection, applies the Bash deny list from the Claude settings inside scripts. `bash cleanup.sh` looks harmless to the permission system even when the script runs `kubectl delete`; this guard reads executed and sourced script files, `-c` strings, and heredocs, and denies when they contain a deny-listed command. Extra patterns (like `rm -rf`) come from the belt config.
- **write-internal-names**: blocks file writes that would put internal org/repo names into a public github.com repo. Uses the same name config as the suspenders pre-commit guard.

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
belt version
```

`belt hook <event>` is the real hook entrypoint (payload on stdin, deny JSON on stdout); Claude Code invokes it, not the user.

## Configuration

Toggles, per-guard `exclude_paths`, and `extra_patterns` live in `~/.config/belt/config.toml`. Denials are logged to the local events timeline (events.this).

## Develop

```bash
make build
make install
make test
make lint
```
