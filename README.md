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
page you read at `present.this`. Services are sandboxed launchd agents, the
MCP servers run sandboxed too, notifications are native, binaries are
codesigned. No cloud, no accounts: your code, notes, and dashboards stay
on your disk.

One of the things I use daily is the reading support. The shared web chrome
has a fixation toggle that bolds the first half of every word to pull your
eye along the line. present can read a page aloud section by section at
a speed you pick, with speak preparing the audio ahead of you. For a long
briefing that combination is easier on the eyes and the attention, and it
genuinely helps if you read with dyslexia. speak's MCP tools extend it to
agents. An agent can hand it a markdown file and it reads the whole thing
aloud on the speakers, so you can take in a long doc by ear.

This is a personal project in the plainest sense: each tool grew out of a
problem I hit building my own things. An agent that pushed straight to
master, internal names that nearly reached a public commit, sessions
re-deriving what a previous session had already worked out. I experiment
here first, run everything daily on my own machines, and plan to keep
building and sharing in the open. The hope is that some of these tools earn
a place in someone else's setup too; the State column below is the honest
record of what has stuck on mine.

> [!NOTE]
> I run this as one system: [ralph](https://github.com/mad01/ralph) installs
> and updates everything. The core set (ralph, t-man, d-man, csl, belt,
> suspenders, humanizer, worklog, events, present, toss-bin) is what I use
> daily, wired together. Every tool works standalone, but some run degraded
> that way: belt does nothing until its hooks are wired, and events is only
> as useful as what reports into it. The cross-tool loops (search hints,
> guard audit trails, memory injection) exist only in the full setup. The
> remaining services are easier to leave out. If you take just one thing,
> csl is the best standalone pick.

Start with one tool, csl for code search over your local checkouts, through
[mise](https://mise.jdx.dev):

```sh
mise tool-alias set csl github:mad01/thismoon
mise use -g "csl[version_prefix=csl/]"
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
levels are honest labels: some of them won't stay. The tables are sorted by
state, proven first, then by name.

## Services

Long-running local web services under `services/`, each on its own `*.this`
address, managed as launchd agents by t-man.

| Service | What it does | Interfaces | State |
|---------|--------------|------------|-------|
| [csl](services/csl/README.md) | Code search over local checkouts (zoekt index) | web · CLI · MCP | proven |
| [d-man](services/d-man/README.md) | The `.this` front door: managed `/etc/hosts` entries + reverse proxy | CLI | proven |
| [deps](services/deps/README.md) | Supply-chain scanner: checks dependencies against OSV.dev, flags advisories | web · CLI · MCP | proven |
| [events](services/events/README.md) | Local event and audit log, archive-only JSONL store | web · CLI · MCP | proven |
| [present](services/present/README.md) | Single-page HTML briefings and slide decks, authored as structured JSON | web · CLI · MCP | proven |
| [speak](services/speak/README.md) | Reads markdown aloud through a local TTS model | web · CLI · MCP | proven |
| [status](services/status/README.md) | Status page with 30-day uptime history for the fleet | web · CLI | proven |
| [moondb](services/moondb/README.md) | Edit history between commits: every file edit an agent session makes, linked to the turn and prompt behind it | web · CLI · MCP | experimental |

## Tools

CLI tools under `tools/`, installed to your local bin.

| Tool | What it does | Interfaces | State |
|------|--------------|------------|-------|
| [clipboard](tools/clipboard/README.md) | macOS clipboard bridge over pbcopy/pbpaste | CLI · MCP | proven |
| [humanizer](tools/humanizer/README.md) | AI-writing detection and voice profiling; its STE check for agent-to-agent text is experimental | CLI · MCP | proven |
| [opener](tools/opener/README.md) | macOS open bridge: URLs, files, apps, Finder reveal | CLI · MCP | proven |
| [t-man](tools/t-man/README.md) | Declarative launchd agent/daemon manager | CLI | proven |
| [toss-bin](tools/toss-bin/README.md) | Safe `rm` replacement: moves files to a dated `~/.Trash` folder | CLI | proven |
| [worklog](tools/worklog/README.md) | Resumable cross-session work state, keyed by ticket or topic | CLI · MCP | proven |
| [belt](tools/belt/README.md) | Claude Code guard and hint hooks: denies risky tool calls, injects context | CLI | experimental |
| [suspenders](tools/suspenders/README.md) | Git secret scanner and pre-commit hook orchestrator | CLI | experimental |
| [agent-sync](tools/agent-sync/README.md) | Syncs Claude Code settings, mods and MCP servers from a data repo onto the machine | CLI | experimental |

The MCP column is the AI half of the toolbox. Register those components as
stdio MCP servers. Then your agent gets the code search, the audit log, and
the work-state checkpoints you already use, reading the same files you do
rather than a copy of them.

## Skills

Alongside the binaries, `skills/` ships seven agent skills: instructions
Claude Code and Codex load on demand. Invoke one by name (`/golang-style`)
or let the agent pick it up when a task matches its description. Each
skill's ralph recipe symlinks it into `~/.claude/skills` and
`~/.agents/skills`, so a fleet machine has them in every session; without
ralph, symlink the skill directory there yourself.

| Skill | Use it when | Backed by |
|-------|-------------|-----------|
| [golang-pro](skills/golang-pro/SKILL.md) | Go work needs specialist depth on concurrency, generics, profiling, or gRPC (vendored from [jeffallan/claude-skills](https://github.com/jeffallan/claude-skills), MIT) | nothing — guidance only |
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

## Memory and hooks

Every agent session starts with an empty context window. On a fleet machine
the stores below carry context from one session to the next, and belt's hooks
inject the memory store without the agent having to ask:

| Store | Holds | Reaches the session through |
|-------|-------|-----------------------------|
| agent-memory | facts about you, for every agent on every machine (a directory of one-fact files, usually a private git clone, set up from [`examples/dotfiles/recipes/agent-memory/`](examples/dotfiles/recipes/agent-memory/)) | belt's `agent-memory` hint at session start |
| [worklog](tools/worklog/README.md) | where a task stands, keyed by ticket or topic | the `/worklog` skill |
| [handoff](skills/handoff/SKILL.md) | a bridge document for the next session | the `/handoff` skill |
| [events](services/events/README.md) | what the guards, hints and services did | belt's ledger writes it; you query it |

Claude Code's built-in auto memory sits beside these: per repo, per machine,
Claude-only, and managed with `/memory` rather than a CLI. None of the stores
needs a git host: each can stay on the machine or sync through a private
Google Drive folder.
[docs/MEMORY-STACK.md](docs/MEMORY-STACK.md) covers which store gets which
fact, how each one is read and written, how to keep each one private, and
the hook behind each.

The memory hints are one part of belt. Its guards deny risky tool calls
before they run: a push to main, a command that discards uncommitted work,
internal names headed for a public repo. Its other hints advise afterwards,
such as handing back a `csl_search` in place of a recursive grep, or asking
for a humanizer pass before text leaves the machine. Its ledger records
session lifecycle to events. [tools/belt/README.md](tools/belt/README.md)
lists every guard and hint, and
[tools/belt/docs/hooks.md](tools/belt/docs/hooks.md) has the wiring.

## Sandboxed by default

Every service and MCP server in the fleet runs under
[anthropics/sandbox-runtime](https://github.com/anthropics/sandbox-runtime)
(srt). Each gets no network except the hosts its policy names, no reads
under your home directory except what it declares, and writes only to its
own state directory and temp. The policy is a few lines of YAML beside the
code, in the component's `service-info.yaml`:

```yaml
spec:
  access:
    sandbox: srt
    workdir: ~/.local/share/events
    listen: 7430
    reads: [~/code/bin, workdir]
    writes: [workdir, temp]
```

`srtspec` renders that to the settings file srt enforces, and CI fails when
the two drift. `srt-run`, the one launcher every sandboxed process goes
through, merges the hosts a machine adds at launch and tags each process.
So when the kernel denies something, `sandbox-watch` can name the process:
a banner, a line in the denial ledger, an event. A third-party binary you
install (an MCP server, a theme watcher) gets the same treatment from a
short manifest in your config repo. The one process srt can't confine, the
text-to-speech engine that needs Metal, keeps a hand-written seatbelt
profile and says so in its manifest.
[docs/SANDBOXING.md](docs/SANDBOXING.md) walks through sandboxing a new
process and explains each piece; the decision is
[ADR-0028](docs/adr/0028-sandbox-runtime-replaces-seatbelt.md).

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

The longer version of this story, what the fleet adds over standalone
tools, the ralph vocabulary, and the rollout order, is
[docs/HOW-IT-FITS-TOGETHER.md](docs/HOW-IT-FITS-TOGETHER.md).
[`examples/dotfiles/`](examples/dotfiles/) is a working private config repo
to start from, including an example global `CLAUDE.md`.

## Install

The short version of both paths is below;
[docs/GETTING-STARTED.md](docs/GETTING-STARTED.md) covers them in depth:
prerequisites, what each path gives you, the step-by-step fleet bootstrap,
and how to verify the result.

### One tool

The fastest path is [mise](https://mise.jdx.dev). It installs the release
tarballs through its `github` backend, one tool alias per component:

```sh
mise tool-alias set csl github:mad01/thismoon
mise use -g "csl[version_prefix=csl/]"
csl web --port 7424   # web UI on http://127.0.0.1:7424
```

The same two commands with another component's name install it;
[the getting-started guide](docs/GETTING-STARTED.md#one-tool-with-mise)
lists every component worth installing on its own. To keep a service
running, register it as a [t-man](tools/t-man/README.md) agent instead of
running it in a terminal.

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

1. Install ralph: `mise use -g github:mad01/ralph`
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
| `mods/` | Claude Code mods: optional status lines and hooks, installed from the `thismoon` plugin marketplace |
| `examples/dotfiles/` | Worked private config repo: overlays + example CLAUDE.md |
| `docs/adr/` | Architecture decision records |
| `docs/GETTING-STARTED.md` | Install guide: one tool vs the fleet |
| `docs/HOW-IT-FITS-TOGETHER.md` | The fleet pitch, ralph vocabulary, rollout order |
| `docs/MEMORY-STACK.md` | Which store holds what an agent should remember, and the hooks behind them |
| `docs/RELEASING.md` | Release process |
| `docs/SANDBOXING.md` | Sandboxing with srt: how to sandbox a new process, then the launcher, access manifests, machine layers and denial triage |
| `docs/CONTAINER-BUILDS.md` | Package-manager build steps in a throwaway Apple container VM: when it is the right tool, the export-only pattern, the runtime setup |

Everything is one Go module: `github.com/mad01/thismoon`. Each component
under `services/` or `tools/` has its own Makefile with `build`, `test`, and
`install` targets; the root Makefile discovers and delegates to them
(`make components` lists them).

## License

BSD-3-Clause, see [LICENSE](LICENSE). No per-file headers; the root license
covers the repo.
