<p align="center">
  <img src="docs/assets/logo.png" alt="thismoon logo: an open sardine tin" width="220">
</p>

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

One of the things I use daily is the reading support. The shared web chrome
has a fixation toggle that bolds the first half of every word to pull your
eye along the line, and present and speak can read a page aloud section by
section at a speed you pick. For a long briefing that combination is easier
on the eyes and the attention, and it genuinely helps if you read with
dyslexia. speak extends it to files: hand it a markdown document and it reads
the whole thing aloud, so you can take in a long doc by ear.

This is a personal project in the plainest sense: each tool grew out of a
problem I hit building my own things — an agent that pushed straight to
master, internal names that nearly reached a public commit, sessions
re-deriving what a previous session had already worked out. I experiment
here first, run everything daily on my own machines, and plan to keep
building and sharing in the open. The hope is that some of these tools earn
a place in someone else's setup too; the State column below is the honest
record of what has stuck on mine.

> [!NOTE]
> I run this as one system: [ralph](https://github.com/mad01/ralph) installs
> and updates everything, and the core set (ralph, t-man, d-man, csl, belt,
> suspenders, humanizer, worklog, events, present, toss-bin) is what I use
> daily, wired together. Every tool works standalone, but some run degraded
> that way: belt does nothing until its hooks are wired, events is only as
> useful as what reports into it, and the cross-tool loops (search hints,
> guard audit trails, memory injection) exist only in the full setup. The
> remaining services are easier to leave out. If you take just one thing,
> csl is the best standalone pick.

Start with one tool:

```sh
brew install mad01/tap/csl    # code search over your local checkouts
```

or install the whole fleet with [ralph](https://github.com/mad01/ralph)
(see [Install](#install)).

## Component states

The **State** column in the tables below says how settled a component is, not
how well it works — everything listed is running on real machines.

- **proven** — earns its keep, used often enough that it is likely to stick.
  Treat its shape and interfaces as stable.
- **evaluating** — works, but still on trial. It might graduate to proven, or
  it might be reworked or dropped once it's had a fair run.
- **experimental** — early and unsettled. Expect it to change or disappear;
  don't build anything load-bearing on top of it yet.

A component only moves up once it's been used enough to trust. The lower two
levels are honest labels: some of them won't stay.

## Services

Long-running local web services under `services/`, each on its own `*.this`
address, managed as launchd agents by t-man.

| Service | What it does | Interfaces | State |
|---------|--------------|------------|-------|
| [catalog](services/catalog/README.md) | Reads `service-info.yaml` across your repos, serves a service catalog | web · CLI | evaluating |
| [csl](services/csl/README.md) | Code search over local checkouts (zoekt index) | web · CLI · MCP | proven |
| [d-man](services/d-man/README.md) | The `.this` front door: managed `/etc/hosts` entries + reverse proxy | CLI | proven |
| [deps](services/deps/README.md) | Supply-chain scanner: checks dependencies against OSV.dev, flags advisories | web · CLI · MCP | proven |
| [events](services/events/README.md) | Local event and audit log, archive-only JSONL store | web · CLI · MCP | proven |
| [keeper-of-facts](services/keeper-of-facts/README.md) | Assertion store (`kof`): evidence-pinned claims about code that go stale with it | web · CLI · MCP | proven |
| [present](services/present/README.md) | Single-page HTML briefings, authored as structured JSON | web · CLI · MCP | proven |
| [prs](services/prs/README.md) | Open-PR dashboard: polls the GitHub hosts of every local checkout | web · CLI · MCP | experimental |
| [reminder](services/reminder/README.md) | Reminders that fire macOS notifications | web · CLI · MCP | evaluating |
| [speak](services/speak/README.md) | Reads markdown aloud through a local TTS model | web · CLI · MCP | proven |
| [status](services/status/README.md) | Status page with 30-day uptime history for the fleet | web · CLI | proven |
| [wire](services/wire/README.md) | Channels agent sessions talk over, two of them or ten, with blocking reads | web · CLI · MCP | experimental |

## Tools

CLI tools under `tools/`, installed to your local bin.

| Tool | What it does | Interfaces | State |
|------|--------------|------------|-------|
| [belt](tools/belt/README.md) | Claude Code guard and hint hooks: denies risky tool calls, injects context | CLI | proven |
| [clipboard](tools/clipboard/README.md) | macOS clipboard bridge over pbcopy/pbpaste | CLI · MCP | proven |
| [humanizer](tools/humanizer/README.md) | AI-writing detection and voice profiling | CLI · MCP | proven |
| [opener](tools/opener/README.md) | macOS open bridge: URLs, files, apps, Finder reveal | CLI · MCP | proven |
| [suspenders](tools/suspenders/README.md) | Git secret scanner and pre-commit hook orchestrator | CLI | evaluating |
| [t-man](tools/t-man/README.md) | Declarative launchd agent/daemon manager | CLI | proven |
| [toss-bin](tools/toss-bin/README.md) | Safe `rm` replacement: moves files to a dated `~/.Trash` folder | CLI | proven |
| [worklog](tools/worklog/README.md) | Resumable cross-session work state, keyed by ticket or topic | CLI · MCP | proven |

The MCP column is the AI half of the toolbox. Register those components as
stdio MCP servers and your agent gets the code search, the audit log, and the
work-state checkpoints you already use, reading the same files you do rather
than a copy of them.

## Skills

Alongside the binaries, `skills/` ships six agent skills — instructions
Claude Code and Codex load on demand. Invoke one by name (`/golang-style`)
or let the agent pick it up when a task matches its description. Each
skill's ralph recipe symlinks it into `~/.claude/skills` and
`~/.agents/skills`, so a fleet machine has them in every session; without
ralph, symlink the skill directory there yourself.

| Skill | Use it when | Backed by |
|-------|-------------|-----------|
| [golang-style](skills/golang-style/SKILL.md) | writing or reviewing Go: naming, package layout, error handling, the HTTP/CLI/store patterns this codebase uses | nothing — guidance only |
| [handoff](skills/handoff/SKILL.md) | a session is ending mid-task and the next one must continue from a cold start | nothing — guidance only |
| [humanizer](skills/humanizer/SKILL.md) | prose is headed for docs, PR descriptions, or commit bodies and should not read as AI-written | the humanizer MCP |
| [loom](skills/loom/SKILL.md) | several agent sessions work one repo in parallel: each claims its own git worktree and commits there, one weaver session lands the branches in order | nothing — guidance only |
| [present](skills/present/SKILL.md) | a work summary or research result deserves a scrollable briefing page with graphs and charts | the present service |
| [worklog](skills/worklog/SKILL.md) | a long task spans sessions and repos and needs to be resumable by ticket or topic | the worklog MCP |

The skill is the workflow; the MCP server or service behind it is its hands.
A skill loads fine without its backing component, but its tool-backed steps
have nothing to call — registration is machine-private wiring (see
[docs/HOW-IT-FITS-TOGETHER.md](docs/HOW-IT-FITS-TOGETHER.md)).

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
install steps. Anything machine-private lives in your own config repo as small
companion recipes that layer on top: which `.this` names exist, MCP
registration, secrets, config overlays (see `docs/adr/0006`). A change to a service ships by merging to main; the next
`ralph up` on each machine rebuilds and restarts it.

The longer version of this story — what the fleet adds over standalone
tools, the ralph vocabulary, and the rollout order — is
[docs/HOW-IT-FITS-TOGETHER.md](docs/HOW-IT-FITS-TOGETHER.md), and
[`examples/dotfiles/`](examples/dotfiles/) is a working private config repo
to start from, including an example global `CLAUDE.md`.

## Install

The short version of both paths is below;
[docs/GETTING-STARTED.md](docs/GETTING-STARTED.md) covers them in depth:
prerequisites, what each path gives you, the step-by-step fleet bootstrap,
and how to verify the result.

### One tool

The fastest path is the [Homebrew tap](https://github.com/mad01/homebrew-tap):

```sh
brew tap mad01/tap
brew install mad01/tap/csl
brew services start mad01/tap/csl   # web UI on http://127.0.0.1:7424
```

Most components have formulas (csl, kof, present, speak, d-man, belt,
t-man, toss-bin, and ralph itself); the rest follow as they prove useful
outside the fleet. [mise](https://mise.jdx.dev) reaches the same release
tarballs, one component per `mise use`:

```sh
mise use -g "github:mad01/thismoon[exe=csl,tag_regex=^csl/]"
```

Everything installs from the module path too:

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
4. Register the d-man daemon (the one sudo step) and verify — walkthrough in
   [the getting-started guide](docs/GETTING-STARTED.md)

See `recipes/` and
[ralph's configuration reference](https://github.com/mad01/ralph/blob/main/docs/configuration.md)
for profiles, host pinning, and overlays.

## Configuration

Precedence is the same across every component: flag > env > config file >
compiled default. Each component documents its own flags, env vars, and
defaults in a `config.md` linked from its README — there's no single
combined reference. Config files live under `~/.config/<tool>/`
(`XDG_CONFIG_HOME` is honored). Services bind
`127.0.0.1` only; see [docs/GETTING-STARTED.md](docs/GETTING-STARTED.md) for
the port table. `<name>.this` addresses need d-man running; every service
also answers on `http://localhost:<port>` with no `.this` setup at all.

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
| `kit/` | Shared Go packages for cross-tool concerns |
| `buildinfo/` | Shared build-metadata package behind every `/version` |
| `recipes/` | ralph recipes, consumed remotely via `[[recipe_sources]]` |
| `skills/` | Agent skills (Claude Code + Codex), symlinked in by their recipes |
| `examples/dotfiles/` | Worked private config repo: overlays + example CLAUDE.md |
| `docs/adr/` | Architecture decision records |
| `docs/GETTING-STARTED.md` | Install guide: one tool vs the fleet |
| `docs/HOW-IT-FITS-TOGETHER.md` | The fleet pitch, ralph vocabulary, rollout order |
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
public. mise fetches those same tarballs with a `MISE_GITHUB_TOKEN`, so the
`mise use` path works today;
[docs/GETTING-STARTED.md](docs/GETTING-STARTED.md#one-tool-with-mise) has
the details.

## License

BSD-3-Clause, see [LICENSE](LICENSE). No per-file headers; the root license
covers the repo.
