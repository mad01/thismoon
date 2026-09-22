# Getting started

There are two ways to put thismoon on a Mac:

- **One tool with Homebrew or mise.** A standalone binary with its web UI on
  a localhost port. Right for trying a single tool, for CI, or when you don't
  want a fleet of launchd agents on the machine.
- **The fleet with [ralph](https://github.com/mad01/ralph).** Every component
  built from source, services running as launchd agents, `<name>.this`
  addresses in every browser, and the wiring that lets the tools feed each
  other.

The install commands look similar; what you end up with doesn't. This guide
covers both paths, what each one gives you, and how to verify the result.

## What each path gives you

| | Homebrew or mise | ralph fleet |
|---|---|---|
| What installs | one binary | every component, built from the repo |
| Web UI address | `http://127.0.0.1:<port>` | `http://<name>.this/` |
| Services run as | `brew services` jobs, or a t-man agent you add | launchd agents managed by t-man |
| Fleet wiring (status page, shared event log, agent hints) | no | yes |
| Updates | `brew upgrade` or `mise upgrade`, per tool, on releases | `ralph up`, whole fleet, on every merge to main |

A single tool standalone is fully functional. The fleet makes each tool
better: [status](../services/status/README.md) watches every service with a
30-day uptime history, every service reports problems to one audit log at
`events.this`, and [belt](../tools/belt/README.md) turns
[kof](../services/keeper-of-facts/README.md)'s assertions into hints inside
your agent sessions.

## Prerequisites

- **macOS on Apple Silicon.** darwin/arm64 is the only build target
  ([docs/adr/0007](adr/0007-macos-arm64-only-artifacts.md)).
- **Go 1.26 or newer**, for the fleet path and `go install`. Everything
  builds from source.
- **Xcode Command Line Tools** (`xcode-select --install`), for the fleet path
  and `go install`. csl compiles its tree-sitter grammars with cgo, which
  needs a C toolchain; the release tarballs have them compiled in.
- **Homebrew**, for the tap path and the easiest ralph install.
- **[mise](https://mise.jdx.dev)**, if you take the mise path instead of the
  tap. It installs ralph too.

## One tool with Homebrew

The [tap](https://github.com/mad01/homebrew-tap) carries formulas for most
components (csl, kof, present, speak, d-man, belt, t-man, toss-bin, and
ralph itself):

```sh
brew tap mad01/tap
brew install mad01/tap/csl
brew services start mad01/tap/csl   # web UI on http://127.0.0.1:7424
```

You get the binary on your PATH and, for services, a `brew services` launchd
job. Everything the tool does on its own works: the web UI on its localhost
port, the CLI, and the MCP stdio server if you register it with your agent
(see [Wire up your agent](#wire-up-your-agent)).

What this path doesn't set up: no `.this` hostname (that comes from d-man),
no t-man agent, no status monitoring, no shared event log, and no
rebuild-on-merge. Each formula updates through `brew upgrade` when a release
is tagged, independent of the rest.

Everything also installs from the module path:

```sh
go install github.com/mad01/thismoon/services/present/cmd/present@latest
```

or from a checkout:

```sh
make -C services/present install    # one component
make install-all                    # everything
```

## One tool with mise

[mise](https://mise.jdx.dev) reaches the same release tarballs through its
`github` backend. Every component releases from this one repository under
its own tag prefix (`csl/v0.18.2`, `keeper-of-facts/v0.13.0`), so mise
needs a tool alias per component and a `version_prefix` that picks the
component's tags out of the shared feed. Two commands put csl on your PATH:

```sh
mise tool-alias set csl github:mad01/thismoon
mise use -g "csl[version_prefix=csl/]"
```

The first writes a `[tool_alias]` entry to `~/.config/mise/config.toml` and
the second a `[tools]` entry, so the same setup as config is:

```toml
[tool_alias]
csl = "github:mad01/thismoon"

[tools]
csl = { version = "latest", version_prefix = "csl/" }
```

The alias is the binary name and the prefix is the component's directory
name under `services/` or `tools/`, so both commands take the same word for
every component below except keeper-of-facts, whose binary is `kof`:

| Component | Alias | `version_prefix` |
|-----------|-------|------------------|
| belt | `belt` | `belt/` |
| clipboard | `clipboard` | `clipboard/` |
| csl | `csl` | `csl/` |
| d-man | `d-man` | `d-man/` |
| deps | `deps` | `deps/` |
| events | `events` | `events/` |
| humanizer | `humanizer` | `humanizer/` |
| keeper-of-facts | `kof` | `keeper-of-facts/` |
| opener | `opener` | `opener/` |
| present | `present` | `present/` |
| speak | `speak` | `speak/` |
| status | `status` | `status/` |
| suspenders | `suspenders` | `suspenders/` |
| t-man | `t-man` | `t-man/` |
| toss-bin | `toss-bin` | `toss-bin/` |
| worklog | `worklog` | `worklog/` |

That is every proven component from the [README](../README.md) tables plus
belt and suspenders. The other components release the same way, but they
are still settling and aren't worth installing standalone yet.

Each alias installs into its own directory, so any number of components sit
side by side, and `mise upgrade` re-resolves each one within its own
prefix. Don't skip the alias: `mise use` on the bare `github:mad01/thismoon`
backend keys every component to the same tool, so a second component
overwrites the first.

You get the binary and nothing else: there is no `brew services` block, so
a service runs under [t-man](../tools/t-man/README.md) or straight from
your shell. Releases are cosign-signed; [RELEASING.md](RELEASING.md) shows
how to verify a download by hand.

The binary alone indexes nothing. Each component's README says what to
configure next. For csl that is a `config.yaml` naming the directories that
hold your checkouts, and the walkthrough from there to the MCP server and a
t-man-supervised web UI is
[services/csl/docs/getting-started.md](../services/csl/docs/getting-started.md).
When you register a mise-installed tool with t-man, give it the shim path
(`$HOME/.local/share/mise/shims/<tool>`): t-man bakes the resolved command
into the launchd plist, and a versioned install path stays on the old build
after `mise upgrade`.

## The fleet with ralph

ralph reconciles a machine against TOML recipes. This repo ships a recipe
per component under `recipes/`, so pointing ralph at the repo installs the
whole platform and keeps it current.

### 1. Install ralph

```sh
brew install mad01/tap/ralph
# or
mise use -g github:mad01/ralph
# or
go install github.com/mad01/ralph/cmd/ralph@latest
```

### 2. Initialize

```sh
ralph init
```

An interactive setup that creates `~/.config/ralph/config.toml`. ralph is a
dotfiles manager first, and the fleet rides on its package and recipe
machinery, so init also asks where your dotfiles repo lives.

### 3. Point ralph at this repo

Add to `~/.config/ralph/config.toml` (or to the config in your own dotfiles
repo):

```toml
[[recipe_sources]]
name = "thismoon"
url = "git@github.com:mad01/thismoon.git"
ref = "main"
update = true
```

ralph clones the repo into `~/.config/ralph/sources/thismoon` and discovers
every recipe in it, merged under the identity `thismoon/<recipe>`. With
`ref = "main"` and `update = true`, every `ralph up` pulls main before
applying; pin `ref` to a tag or commit to stay put. The clone runs over SSH,
so the machine needs an SSH key registered with GitHub.

### 4. Declare the machine's profiles

```sh
ralph profile set personal    # or: work, or both
```

This writes the git-ignored `~/.config/ralph/config.local.toml` beside your
config, the per-machine answer to "what kind of machine is this". Your own
profile-gated recipes and profile-gated `[[recipe_sources]]` key off it, and
so does belt's `git-push-main` guard, which reads the same file at runtime.
thismoon's recipes carry no profile gate of their own: to keep one off a
machine class, override it from your config
(`[recipes_config.overrides."thismoon/<name>"]`) or from a role source's
`overrides.toml`. Do this before the first `ralph up`: a machine with no
profiles silently skips every profile-gated recipe and source, and only
`ralph doctor` will mention it.

### 5. First `ralph up`

```sh
ralph up            # add --dry-run to preview
```

What happens on this run:

1. ralph pulls the source cache under `~/.config/ralph/sources/thismoon`.
2. Each component builds with its own Makefile out of that cache and
   installs to `~/code/bin` (codesigned with a local identity when one is
   present). Add `~/code/bin` to your PATH if it isn't there yet; the
   verify commands below run from it.
3. Each web service registers as a t-man launchd user agent (`csl-web` on
   port 7424, `present` on 7423, and so on) and starts. Service recipes
   depend on t-man, so it's in place before they need it.

Later runs are incremental: a component rebuilds when its source changed,
and a service restarts only when the installed binary's bytes actually
differ.

### 6. Register the d-man daemon (the one sudo)

[d-man](../services/d-man/README.md) is the `.this` front door: it writes a
managed block into `/etc/hosts` and reverse-proxies `127.0.0.1:80` by `Host`
header. Both need root, so it runs as a root launchd daemon, and registering
it is a deliberate manual step: `ralph up` keeps printing the
`t-man --daemon add` command below until the daemon exists.

First write a routes file at `~/.config/d-man/routes.toml`. Which `.this`
names exist is a per-machine choice, so no public recipe writes this file
for you:

```toml
suffix = "this"

[[route]]
name = "csl"
port = 7424

[[route]]
name = "present"
port = 7423

[[route]]
name = "status"
port = 7426
```

Add a `[[route]]` per service you want a name for (ports in the table
below; full format in [services/d-man/README.md](../services/d-man/README.md)).
Then register the daemon:

```sh
sudo t-man --daemon add --name d-man -- \
  $HOME/code/bin/d-man serve --config $HOME/.config/d-man/routes.toml
```

That's the only sudo the platform needs. Afterwards the daemon watches its
routes file, so edits apply on save, and it watches its own binary, so a
rebuild by `ralph up` makes it exit and launchd relaunch the new build. The
full walkthrough, including the optional block-page CA, is in
[recipes/d-man/SETUP.md](../recipes/d-man/SETUP.md).

### 7. Verify

```sh
ralph doctor       # config, symlinks, missing tools
t-man list         # every registered agent and its running state
csl doctor         # the search daemon and its index
open http://status.this/
```

`status.this` shows a card per t-man-managed service with a 30-day uptime
strip. Green across the board means the fleet is up.

Green from `csl doctor` does not mean csl is indexing anything: an
unconfigured csl is a valid starting state and passes every check as long as
the `csl-web` agent ralph registered is up (the two web probes fail without
it). Which
directories a machine indexes is yours to declare in
`~/.config/csl/config.yaml` (a `dirs` list; the worked overlay recipe is
[`examples/dotfiles/recipes/csl-config/`](../examples/dotfiles/recipes/csl-config/)).
Then `csl repo --list` should show your checkouts, and the first `csl search`
builds the index. The recipe's `repo` and `repo-sync` shell functions land in
`~/.config/ralph/generated/generated_functions.sh`, which ralph's rc snippet
sources.

### What done looks like

These addresses answer in any browser and in `curl`, one per declared route
(the example `routes.toml` above declares three of them; add the rest as
you want them):

| Address | Service | Port |
|---|---|---|
| `catalog.this` | catalog | 7575 |
| `csl.this` | csl | 7424 |
| `deps.this` | deps | 7429 |
| `events.this` | events | 7430 |
| `kof.this` | keeper-of-facts | 7431 |
| `present.this` | present | 7423 |
| `speak.this` | speak | 7425 |
| `status.this` | status | 7426 |

The names are whatever your `routes.toml` says; each recipe fixes its
service's port. The CLI tools (belt, humanizer, suspenders, t-man, toss-bin,
worklog) sit in `~/code/bin`.

A `<name>.this` address needs d-man running; every service in the table
above also answers on plain `http://localhost:<port>` with no d-man setup
at all. Beyond ports, each component resolves its own flags, env vars, and
config file the same way: flag > env > config file > compiled default, with
config files under `~/.config/<tool>/`. See
[Configuration](../README.md#configuration) in the README, and the
component's own `config.md` for its specific keys.

## What the fleet adds

```
             you  +  your agent
                     |
            http://<name>.this/
                     |
        d-man: /etc/hosts + proxy on :80
                     |
    csl   present   events   kof   status   ...
        launchd agents, managed by t-man
```

- **t-man runs everything.** Every service is a launchd agent with
  `t-man logs <name>` for output and `t-man restart <name>` for a bounce.
- **d-man names everything.** One routes file maps names to ports, and every
  client on the machine resolves them, Safari and `curl` included. The
  shared web UI's command palette lists whichever services are up right now,
  fed by d-man's live site list.
- **status watches everything.** It probes each t-man service once a minute,
  keeps 30 days of uptime, and flags a service whose running binary is older
  than the one installed on disk.
- **events collects from everything.** Services report warnings and errors
  to `events.this`, one audit log instead of a folder of log files.
- **kof and belt close the loop for agents.** kof stores evidence-pinned
  assertions about your code; belt's hints surface them in Claude Code
  sessions when you work in the matching repo.

[HOW-IT-FITS-TOGETHER.md](HOW-IT-FITS-TOGETHER.md) is the longer version of
this argument: what the connections between the tools add up to, the ralph
vocabulary, and the full rollout order including the private overlay layer.

## Wire up your agent

The MCP column in the [README](../README.md) component tables marks which
components ship an MCP server. Each of those serves MCP over stdio through a
subcommand (`csl mcp`, for example); register that command as a stdio server
in your agent's MCP configuration and the agent reads the same local data
you do. For Claude Code that's one command per tool:

```sh
claude mcp add --scope user csl -- csl mcp
```

`--scope user` registers the server for every project; without it Claude
Code registers the tool for the current directory only. Registration alone
does not change what the agent reaches for: each MCP-bearing component's
README carries a CLAUDE.md section that tells the agent when to use its
tools, and the csl one keeps the agent on lexical search until the semantic
index exists (`csl docs --claude-md >> ~/.claude/CLAUDE.md` appends it).

Registration is deliberately not part of the public recipes. Which agents
run on a machine, with which servers and which config, is machine-private
wiring, and it lives in your own config repo as small companion recipes
layered over these ([docs/adr/0006](adr/0006-recipe-layering-and-platform-deps.md)).
The same split covers belt's Claude Code hooks and per-machine config files
like csl's index list: the public recipe installs the binary, your private
overlay wires it up.

Every piece of that wiring has a worked example in
[`examples/dotfiles/`](../examples/dotfiles/):
[`recipes/mcp-registration/`](../examples/dotfiles/recipes/mcp-registration/)
for the MCP server set,
[`recipes/claude-hooks/`](../examples/dotfiles/recipes/claude-hooks/) for
the settings block that turns belt's guards and hints on (explained hook by
hook in [tools/belt/docs/hooks.md](../tools/belt/docs/hooks.md); the
`prefer-csl` hint is the one that matters for csl, handing a multi-file
`grep` in an indexed repo back as the equivalent `csl_search` call), and
[`CLAUDE.md.example`](../examples/dotfiles/CLAUDE.md.example) for the
instruction file that teaches the agent when to reach for which tool. The
seven skills under `skills/` need no registration; their recipes symlink
them into `~/.claude/skills`, and they load when invoked by name
(`/golang-pro`, `/golang-style`, `/handoff`, `/humanizer`, `/loom`, `/present`,
`/worklog`) or when a task matches; the MCP-backed ones assume their server
from this section is registered.

## Updating

**Fleet:** merging to main is the deploy. The next `ralph up` on each
machine pulls the source cache, rebuilds components whose source changed,
and restarts their services when the installed binary changed. d-man
restarts itself when its binary changes, so updates stay sudo-free. If an
install ever lands without a restart, `status.this` shows the service as
"Stale binary" and fires a notification; `t-man restart <name>` clears it.

**Homebrew:** `brew upgrade` per formula, then
`brew services restart <formula>` if it runs as a service. Formulas track
tagged releases, so you update when a release is cut rather than on every
merge.

**mise:** `mise upgrade <alias>` re-resolves the newest tag under the
component's prefix. Restart the t-man agent yourself if the tool runs as a
service. The restart only helps when the agent was registered with the mise
shim path (`$HOME/.local/share/mise/shims/<tool>`); a plist that names a
versioned install directory keeps running the old build, and re-adding the
agent with the shim path is the fix.
