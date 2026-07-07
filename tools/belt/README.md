# belt

Guard hooks for Claude Code sessions. belt runs as a PreToolUse hook, inspects the tool call before it executes, and denies risky ones with a reason the agent can act on.

Named for the layer it adds: suspenders holds up the git side (pre-commit secret scanning and internal-name guard). belt holds up the session side, before anything reaches git.

## Guards

- **git-push-main** — blocks `git push` to main/master on machines with the work profile. Personal machines push to main freely.
- **script-deny-list** — deep deny inspection: applies the Bash deny list from the Claude settings inside scripts. `bash cleanup.sh` looks harmless to the permission system even when the script runs `kubectl delete`; this guard reads executed and sourced script files, `-c` strings, and heredocs, and denies when they contain a deny-listed command. Extra patterns (like `rm -rf`) come from the belt config.
- **write-internal-names** — blocks file writes that would put internal org/repo names into a public github.com repo. Uses the same name config as the suspenders pre-commit guard.

## Usage

Built and installed by ralph (`recipes/belt/`); hook registration lives in the consuming repo's Claude settings recipe. Manual dry-runs:

```
belt check bash "git push origin main"
belt check bash "bash cleanup.sh"
belt check write --file README.md --content "mentions something internal"
```

Toggles live in `~/.config/belt/config.toml`. Denials are logged to the local events timeline (events.this).
