# thismoon

Give your coding agent the same tools you use, on your own disk.

thismoon is a fleet of local-first developer tools for Apple Silicon Macs.
Each one is a small web service on its own `http://<name>.this/` address and
a CLI; most are also [MCP](https://modelcontextprotocol.io) servers, so you
and your agent work against the same local data. The code search you run in
a browser is the index Claude queries; the briefing an agent writes is the
page you read at `present.this`. Services are launchd agents, notifications
are native, binaries are codesigned. No cloud, no accounts: your code,
notes, and dashboards stay on your disk.

Start with one tool:

```sh
brew install mad01/tap/csl    # code search over your local checkouts
```

or install the whole fleet with [ralph](https://github.com/mad01/ralph)
(see [Install](#install)).

The repo also carries the shared web UI package (`webkit/`) and the ralph
recipes that install the fleet.

## Services

Long-running local web services under `services/`, each on its own `*.this`
address, managed as launchd agents by t-man.

| Service | What it does | Interfaces |
|---------|--------------|------------|
| catalog | Reads `service-info.yaml` across your repos, serves a service catalog | web · CLI |
| csl | Code search over local checkouts (zoekt index) | web · CLI · MCP |
| d-man | The `.this` front door: managed `/etc/hosts` entries + reverse proxy | CLI |
| deps | Supply-chain scanner: checks dependencies against OSV.dev, flags advisories | web · CLI · MCP |
| events | Local event and audit log, archive-only JSONL store | web · CLI · MCP |
| keep | Assertion store: evidence-pinned claims about code that go stale with it | web · CLI · MCP |
| present | Single-page HTML briefings, authored as structured JSON | web · CLI · MCP |
| reminder | Reminders that fire macOS notifications | web · CLI · MCP |
| speak | Reads markdown aloud through a local TTS model | web · CLI · MCP |
| status | Status page with 30-day uptime history for the fleet | web · CLI |

## Tools

CLI tools under `tools/`, installed to your local bin.

| Tool | What it does | Interfaces |
|------|--------------|------------|
| belt | Claude Code guard hooks (blocks push-to-main, internal-name writes) | CLI |
| bionic | Bionic-reading text transform | CLI · MCP |
| humanizer | AI-writing detection and voice profiling | CLI · MCP |
| suspenders | Git secret scanner and pre-commit hook orchestrator | CLI |
| t-man | Declarative launchd agent/daemon manager | CLI |
| worklog | Resumable cross-session work state, keyed by ticket or topic | CLI · MCP |

The MCP column is the AI half of the toolbox: register those components as
stdio MCP servers and an agent gets code search, dependency checks, reminders,
briefing pages, an audit log, and work-state checkpoints on the same local
data you see in the web UIs.

## How it fits together

Three layers, two of them public:

```
thismoon (public)      components + recipes/: the code and how to install it
   ↑ consumed by
ralph (public)         the installer: reconciles machines against TOML recipes
   ↑ configured by
your config repo       machine-private wiring: secrets, host config, overlays
(private)
```

Each component ships with a recipe under `recipes/` that builds it, installs
it, and (for services) registers it as a launchd agent. ralph consumes those
recipes remotely through a `[[recipe_sources]]` stanza in its config:

```toml
[[recipe_sources]]
name = "thismoon"
url = "git@github.com:mad01/thismoon.git"
ref = "main"
update = true
```

Recipes merge under the identity `thismoon/<recipe>`. With `ref = "main"` and
`update = true`, every `ralph up` pulls main and converges the machine on the
latest recipes; pin `ref` to a tag or commit to stay put.

The recipes here are deliberately the public layer only: portable build and
install steps. Anything machine-private (which `.this` names exist, MCP
registration, env and secrets, config overlays) lives in your own private
config repo as small companion recipes that layer on top (see
`docs/adr/0006`). A change to a service ships by merging to main; the next
`ralph up` on each machine rebuilds and restarts it.

## Install

### One tool

The fastest path is the [Homebrew tap](https://github.com/mad01/homebrew-tap):

```sh
brew tap mad01/tap
brew install mad01/tap/csl
brew services start mad01/tap/csl   # web UI on http://127.0.0.1:7424
```

Most components have formulas (csl, keep, present, speak, d-man, belt,
suspenders, t-man, and ralph itself); the rest follow as they prove useful
outside the fleet. Everything installs from the module path too:

```sh
go install github.com/mad01/thismoon/services/present/cmd/present@latest
```

or from a checkout:

```sh
make -C services/present install    # one component
make install-all                    # everything
```

Prebuilt darwin/arm64 tarballs hang off each component's GitHub Release,
with `checksums.txt` and a cosign keyless signature.

### The fleet

For the full setup (services as launchd agents, `.this` routing, config
symlinks) on a fresh machine:

1. Install ralph: `brew install mad01/tap/ralph`
   (or `go install github.com/mad01/ralph/cmd/ralph@latest`)
2. Run `ralph init`, then add the `[[recipe_sources]]` stanza shown above to
   `~/.config/ralph/config.toml` (or to your private config repo)
3. Run `ralph up`

See `recipes/` and
[ralph's configuration reference](https://github.com/mad01/ralph/blob/main/docs/configuration.md)
for profiles, host pinning, and overlays.

## Releases

Components release independently: per-component semver tags in the form
`present/v1.2.3`, cut by release-please from conventional commits on merge to
main. Artifacts ship with a `checksums.txt` and a cosign keyless signature.
The full flow, including how to verify a download and how to register a new
component, is in [docs/RELEASING.md](docs/RELEASING.md).

## Layout

| Path | Contents |
|------|----------|
| `services/` | Local web services, one directory per service |
| `tools/` | CLI tools |
| `webkit/` | Shared Go web UI package, compiled in, no separate versioning |
| `recipes/` | ralph recipes, consumed remotely via `[[recipe_sources]]` |
| `docs/adr/` | Architecture decision records |
| `docs/RELEASING.md` | Release process |

Everything is one Go module: `github.com/mad01/thismoon`. Each component
under `services/` or `tools/` has its own Makefile with `build`, `test`, and
`install` targets; the root Makefile discovers and delegates to them
(`make components` lists them).

### While this repo is private

`go install` and module fetches need git-over-SSH plus a `GOPRIVATE` entry:

```sh
export GOPRIVATE=github.com/mad01/*
git config --global url."git@github.com:".insteadOf "https://github.com/"
```

Once the repo is public, neither is needed for this module: drop the
`insteadOf` rewrite and trim `GOPRIVATE` to whatever private repos remain.
The ralph source stanza works unchanged in both worlds; it always clones over
SSH. Homebrew is the opposite: the tap formulas download release tarballs
from this repo, so `brew install` starts working only once the repo is
public.

## License

BSD-3-Clause, see [LICENSE](LICENSE). No per-file headers; the root license
covers the repo.
