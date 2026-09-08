# Getting started

There are two ways to put thismoon on a Mac:

- **One tool with Homebrew.** A standalone binary with its web UI on a
  localhost port. Right for trying a single tool, for CI, or when you don't
  want a fleet of launchd agents on the machine.
- **The fleet with [ralph](https://github.com/mad01/ralph).** Every component
  built from source, services running as launchd agents, `<name>.this`
  addresses in every browser, and the wiring that lets the tools feed each
  other.

The install commands look similar; what you end up with doesn't. This guide
covers both paths, what each one gives you, and how to verify the result.

## What each path gives you

| | Homebrew | ralph fleet |
|---|---|---|
| What installs | one binary | every component, built from the repo |
| Web UI address | `http://127.0.0.1:<port>` | `http://<name>.this/` |
| Services run as | `brew services` jobs | launchd agents managed by t-man |
| Fleet wiring (status page, shared event log, agent hints) | no | yes |
| Updates | `brew upgrade`, per formula, on releases | `ralph up`, whole fleet, on every merge to main |

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
- **Xcode Command Line Tools** (`xcode-select --install`). csl compiles its
  tree-sitter grammars with cgo, which needs a C toolchain.
- **Homebrew**, for the tap path and the easiest ralph install.

While this repo is private, you also need an SSH key with access to it. The
ralph path works today because ralph clones recipe sources over SSH. Two
caveats in the private phase:

- `go install` needs module fetches routed over SSH:

  ```sh
  export GOPRIVATE=github.com/mad01/*
  git config --global url."git@github.com:".insteadOf "https://github.com/"
  ```

- `brew install` doesn't work yet: the tap formulas download release
  tarballs from this repo, which needs the repo public. Until then, use the
  `go install` or checkout variant wherever this guide shows a `brew`
  command.

## One tool with Homebrew

The [tap](https://github.com/mad01/homebrew-tap) carries formulas for most
components (csl, kof, present, speak, d-man, belt, suspenders, t-man, and
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

## The fleet with ralph

ralph reconciles a machine against TOML recipes. This repo ships a recipe
per component under `recipes/`, so pointing ralph at the repo installs the
whole platform and keeps it current.

### 1. Install ralph

```sh
brew install mad01/tap/ralph
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
so this works while the repo is private.

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
| `prs.this` | prs | 7427 |
| `reminder.this` | reminder | 7428 |
| `speak.this` | speak | 7425 |
| `status.this` | status | 7426 |
| `wire.this` | wire | 7432 |

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
claude mcp add csl -- csl mcp
```

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
hook in [tools/belt/docs/hooks.md](../tools/belt/docs/hooks.md)), and
[`CLAUDE.md.example`](../examples/dotfiles/CLAUDE.md.example) for the
instruction file that teaches the agent when to reach for which tool. The
six skills under `skills/` need no registration; their recipes symlink
them into `~/.claude/skills`, and they load when invoked by name
(`/golang-style`, `/handoff`, `/humanizer`, `/loom`, `/present`,
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
