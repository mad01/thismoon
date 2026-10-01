# belt

Guard and hint hooks for Claude Code sessions. belt inspects tool calls and either denies the risky ones before they run or advises after them, always with a reason the agent can act on.

Named for the layer it adds: suspenders holds up the git side (pre-commit secret scanning and internal-name guard). belt holds up the session side, before anything reaches git.

belt has two halves, and the split is deliberate (see `docs/adr/0008`):

- **Guards** run as a PreToolUse hook and can deny. They are reserved for damage that is hard to undo.
- **Hints** only advise. Most run as a PostToolUse hook after the tool call whose result stands; two ride session lifecycle events instead (SessionStart and UserPromptSubmit). A hint that misfires costs a few lines of ignored text rather than a stalled session.

The one-paragraph summaries below say what each guard and hint does; [docs/hooks.md](docs/hooks.md) covers how each one decides, why it exists, how it fails, and the `~/.claude/settings.json` entries that wire belt up.

## Guards

Six built-in guards run against tool calls before they execute:

- `git-push-main`: blocks `git push` to main/master unless the repo is allowlisted. Machines where direct pushes are fine (personal machines, say) disable the guard in their rendered config.
- `git-identity`: blocks `git commit` when the repo's `git config user.email` doesn't match the email the config expects for that repo. Rules match by repo pattern (exact `host/owner/repo` or a trailing `/*` org wildcard), so the repo decides the identity whatever machine the commit happens on. Soft mode downgrades the block to a warn event.
- `commit-guard`: nudges personal-repo work out of work hours. Commits to configured repos inside a local-time window are warned about (soft, the default) or blocked (hard). `always_allow` exempts repos needed at any hour, and `belt override set <name> --reason "..."` switches a rule off for a timed window. That window is 10 minutes by default and `--for` sizes it. The required `--reason` is archived to the events service. `belt override extend` pushes the window forward, and it expires on its own so a forgotten override can't disarm a rule for days.
- `script-deny-list`: deep deny inspection, applies the Bash deny list from the Claude settings inside scripts. `bash cleanup.sh` looks harmless to the permission system even when the script runs `kubectl delete`. This guard reads executed and sourced script files, inline `-c`/`-e` strings, heredocs, and stdin redirects and pipes into interpreters. It also reads the commands behind `eval`/`xargs`/`find -exec`, and a file written and run inside the same command. It denies when any of that contains a deny-listed command. Piping a `curl`/`wget` download straight into an interpreter is denied outright, since nothing can read what would run. Extra patterns (like `rm -rf`, or `re:`-prefixed regexes) come from the belt config; `mode: soft` turns denials into warn events.
- `write-internal-names`: blocks file writes that would put internal org/repo names into a public-bound repo: one on the config's `public_repos` list, or any github.com repo when a rendering has no such list. Names come from the `internal_names` section of belt's own config, plus the shared names files that section lists under `include`; belt reads no other tool's config. Leaving the section unset means an empty name set, called out by `belt doctor`. The suspenders pre-commit guard derives its list the same way from its own config, so the two layers agree when their configs do.
- `publish-internal-names`: the same name set and the same `public_repos` test, checked on the way out to a public-bound remote instead of on the way into a file. On the `bash` event it covers `git push`, `git commit -m`, `git tag`, and branch creation. For a push that means the pushed ref names plus the messages of the commits the remote doesn't have yet. It also covers the gh subcommands that write (pr, issue, release, repo, gist, label, api), reading any `--body-file` the call names. On the `external-text` event it covers the gh MCP tools the consuming repo routes to it. It checks every string in the call except the owner/repo pair, so PR bodies, `head` branch names, labels, and `push_files` content all count. Deletions are allowed, `allow_repos` exempts a private companion repo, and `mode: soft` turns denials into warn events for rollout. Targets that are public by nature (a gist, the gists API endpoints, `gh repo create` without `--private` or `--internal`, and their MCP twins) are guarded even when no `public_repos` list exists.

Beyond the built-ins, a `custom_guards` config section registers named guards that shell out to an external command. The tool-call fields arrive as JSON on stdin. Exit 0 allows, and exit 1 denies with stdout as the reason. Anything else (a crash, a missing binary, a 5-second timeout) allows with a warn event, so a broken external never blocks work.

## Hints

Eight hints add advisory context the agent reads next to a tool result or at session boundaries:

- `prefer-csl`: after a bash command sweeps multiple files inside a repo csl already indexes, hands back the equivalent `csl_search` call with the pattern translated to zoekt syntax. Pipe filters (`cmd | grep x`) and single-file greps don't fire, since they aren't what csl replaces.
- `commit-policy`: after a `git commit` lands on main or master of a repo not opted out via `exclude_repos`, states the branch + PR commit policy. It also hands back the two commands that move the commit onto a branch. The post-commit half of the branch-discipline pair: the git-push-main guard denies the push, this hint catches the mistake while it is still a cheap fix. Repos with no origin remote stay silent. A repo can version its own policy in a root `.belt.yaml` overlay (opt-out, replacement protected branches, an appended message); hints-only by design, guards never read repo files (docs/adr/0012).
- `lint-policy`: after a `git commit` in a repo whose root `.belt.yaml` declares a `hints.lint-policy.message`, relays that message once per session per repo. The repo states its own fmt/lint toolchain, belt only picks the moment (no language→tool table, nothing executed). A trigger that already names the toolchain (`make lint && git commit`) is skipped without spending the nudge.
- `kof-assertions`: after a csl search, surfaces stored kof assertions about the code the search hit, so prior conclusions get read instead of re-derived. Caps at three, marks stale ones, and repeats nothing within a session.
- `kof-consult`: when a session opens inside a git repo, surfaces that repo's kof assertions before any searching happens, so the session starts from what earlier sessions concluded. Caps at five; outside a repo, or with kof down, it stays silent.
- `agent-memory`: when a session opens, injects the agent memory indexes (`~/.config/agent-memory/MEMORY.md`, plus `~/.config/agent-memory-work/MEMORY.md` on machines that have one). Cross-agent facts then arrive as context instead of relying on instruction prose to prompt a read. A missing store is silence, so the work index never appears on personal machines.
- `kof-deposit`: once per session, nudges a session that did substantial work but never recorded a kof assertion to deposit what it derived before the conclusions evaporate with the context.
- `humanizer-check`: once per session, after a tool call publishes text to a system other people read, points at `humanizer_detect` while the wording can still be edited. A session that already ran a humanizer tool is left alone.

Beyond the built-ins, a `custom_hints` config section registers named hints that shell out to an external command when a session starts. The hook payload fields arrive as JSON on stdin, and the command's stdout becomes the advice under the usual `belt[<name>]:` prefix. A non-zero exit, a run past the budget (400 ms unless `timeout_ms` says otherwise), or empty stdout means silence plus a warn event. A broken external never breaks a session. This is the escape hatch for context that belongs to one machine, such as a private daily journal. The hint lives in that machine's rendered config rather than in a second SessionStart hook or a compiled-in hint.

## Install

Built and installed by ralph (`recipes/belt/recipe.toml`); hook registration lives in the consuming repo's Claude settings recipe. A worked config file and hooks block, the shape the maintainer's fleet installs, is in `examples/dotfiles/recipes/claude-hooks/` at the repo root. To build and install manually:

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

`belt doctor` prints the resolved state behind those decisions. First comes the installed build (version, commit, release tag, build time) and which config file loaded, or failed to parse. Next are the guards and hints in effect, with their allow/exclude counts. Doctor also probes the kof serve instance behind the kof-* hints and reports whether it is reachable and how many assertions it stores. The hints render "kof down" and "store empty" identically, as silence, so doctor is what tells them apart. The last block is the full blocked-name set the write-internal-names and publish-internal-names guards match against. Doctor closes with a warnings section and exits 1 when that section is not empty, on a machine with no config at all included.

`belt config` prints the config file locations and every setting in effect after defaults are applied. That covers the guard and hint toggles, the resolved internal_names, and the Claude-settings Bash deny patterns the script guard enforces. `belt config --help` carries the annotated reference of every setting, including how `allow_repos` exempts a repo from a guard and `exclude_paths` exempts paths.

When a deny surprises you: `check` shows the verdict, `doctor` shows the state that produced it, `config` shows which file and key to change.

`belt version` prints the bare version token of the installed build. With `-o json` it prints the full build metadata (`version`, `commit`, `tag`, `build_time`), the same four keys every tool in this repo reports, so one probe can ask any of them what build is running.

`belt hook <event>` and `belt hint <event>` are the real hook entrypoints (payload on stdin, output on stdout); Claude Code invokes them, not the user. `hook` carries deny decisions for guards; `hint` carries `additionalContext` JSON for the search, bash, and session-start events, and plain stdout text for prompt (UserPromptSubmit adds stdout to context directly).

## Configuration

Toggles, `exclude_paths`, `extra_patterns`, and `allow_repos` live in `~/.config/belt/config.yaml`. Guards sit under `guards.<id>` and hints under `hints.<id>`. Both default to enabled when the file or entry is missing. `$XDG_CONFIG_HOME` moves the directory, and `--config` and `$BELT_CONFIG` move the file. A legacy `config.toml` beside the default file is still read when no YAML file exists.

A config file that is present but fails to parse or validate is different from a missing one. In that case belt can't tell "no rules" from "the rules didn't load". So every guarded tool call is denied with `belt[config]:` and the file to fix, until the file is fixed or moved aside.

`allow_repos` exempts specific repositories from a guard by canonical `host/owner/repo`, or a trailing `/*` org wildcard. For example, `guards.git-push-main` with `allow_repos: [github.com/mad01/dotfiles]` permits direct pushes to the default branch in that repo. Every other repo stays fail-closed. Hints take the mirror-image key, `exclude_repos`.

One shared list, top-level `direct_main_repos`, is purpose-named. It states which repos' workflow is direct-to-main. It is read by exactly git-push-main (push allowed) and the commit-policy hint (advice silenced), never by any other check. That is why editing it can't disarm an unrelated guard (docs/adr/0013).

The other shared list, `public_repos`, states which repos are public or headed there. It is read by exactly the two internal-name guards. Internal names are blocked in listed repos and allowed everywhere else. That split is what lets an internal org hosted on github.com work without exemptions. A rendering without the key keeps the older rule that every github.com repo is public-bound (docs/adr/0015).

An exemption that should hold on only some machines goes in those machines' rendered config file, because there is no machine-profile switch inside the config (docs/adr/0010).

The rule-driven guards and the external hints read four more top-level sections:

- `git_identity` holds the expected email per repo pattern.
- `commit_guards` holds work-hours rules with `block_hours`, `always_allow`, and an `override` name.
- `custom_guards` registers external commands as named guards.
- `custom_hints` registers external commands as named session-start hints.

`belt config --help` carries the annotated reference for all of them. Denials and hints are logged to the local events timeline (events.this).

## Develop

```bash
make build
make install
make test
make lint
```

## Docs

- [architecture](architecture.md): internal structure and data flow
- [operating](operating.md): runtime behavior, failure modes, first moves
- [why](why.md): why this component exists
- [config](config.md): configuration reference
- [hooks](docs/hooks.md): every guard and hint in depth, plus the Claude Code wiring
