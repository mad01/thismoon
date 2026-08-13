# deps

Scan every tool repo for the external packages it pulls in, check each version
against the [OSV.dev](https://osv.dev) advisory database, and get a macOS
notification when something is flagged. Mostly you'll read the findings by asking
Claude or opening `http://deps.this/`; there's also a `deps` CLI. They're all the
same scan.

```
$ deps check
24 flagged of 851 checked:
  Go golang.org/x/net@0.54.0  GO-2026-5025 → fix 0.55.0
      key: Go:golang.org/x/net@0.54.0:GO-2026-5025
  Go github.com/go-jose/go-jose/v4@4.1.3  GHSA-78h2-9frx-2jm8 [HIGH] → fix 4.1.4
      key: Go:github.com/go-jose/go-jose/v4@4.1.3:GHSA-78h2-9frx-2jm8
  ...
```

## How it works

One background agent (`deps serve`) owns a JSON file of scan results, runs the
web page, checks OSV, and fires the notifications. Everything else (the CLI, the
Claude Code tools, the web page) talks to that agent, so you see the same
findings everywhere and only one process touches the network or the file.

The repos it scans come from the catalog registry
(`~/.config/catalog/registry.yaml`), so a repo enrolls in scanning the moment
it's catalogued. In each repo it finds Go modules (the resolved graph from
`go list -m -json all`) and npm lockfiles (`package-lock.json`), then asks OSV
whether any of those exact versions has a known advisory. Git worktrees and
nested checkouts are skipped, so a repo isn't scanned twice.

A flagged finding carries the advisory id, severity, and the version that fixes
it. You can **resolve** (acknowledge) one to drop it from the active list; it
comes back if the package version changes or a new advisory lands on it, since
the resolve is tied to that exact version.

`deps serve` runs a full check on startup and keeps it no more than a day old. It
checks staleness rather than counting ticks, so if the Mac was asleep past the
daily mark it runs the missed scan shortly after waking. When a scan fails
(usually because you're offline and OSV is unreachable), it backs off
exponentially up to six hours instead of retrying every few minutes, and resets
on the next success.

Notifications are coalesced: one banner summarizing everything newly flagged in a
scan, not one per advisory. Acknowledged findings don't notify.

## Install

It ships from the thismoon monorepo via ralph, personal Mac only. `ralph up` builds the binary to
`~/code/bin/deps`, registers the `deps serve` agent with t-man, adds the
`deps.this` route, symlinks the discovery config, and wires the Claude Code
tools.

To check the service is up:

```bash
t-man status deps
```

## Usage

```bash
deps scan                          # discover dependencies, per-ecosystem counts
deps check                         # discover + check against OSV, print flagged
deps check --repo dotfiles         # rescan one repo (by path or basename)
deps resolve <key> [<key>...]      # acknowledge advisories by key (from `deps check`)
deps notify                        # fire notifications for anything not yet notified
```

The CLI and MCP tools both need `deps serve` running: they're HTTP clients to
it.

**Web UI.** Open `http://deps.this/`. It shows the per-ecosystem counts, the
active findings, and an acknowledged section. From here you can:

- **Rescan all**: run a full check now
- **Rescan** one repo: the ↻ button on each repo chip
- **Resolve** a finding: acknowledge it so it drops to the acknowledged section

## Endpoints

- `GET  /`: web UI
- `GET  /api/deps` / `GET /api/flagged`: read the inventory
- `POST /api/scan` / `POST /api/check[?repo=]`: trigger a scan
- `POST /api/resolve`: acknowledge advisories by key
- `POST /api/notify`: fire pending notifications now
- `GET  /version`: build metadata (`version`, `commit`, `tag`, `build_time`)

## MCP

Ask in plain language: "scan for vulnerable dependencies", "what's flagged",
"rescan the dotfiles repo", "resolve that go-jose advisory". Claude calls the
`deps_*` MCP tools, registered in the consuming repo's `recipes/claude-mcp/servers.json`:

- `deps_scan`: discover all dependencies, no advisory check
- `deps_check`: discover and check against OSV; returns the flagged packages
- `deps_list_flagged`: re-read the last findings without re-scanning
- `deps_scan_repo`: rescan one repo (by path or basename) and merge the result
- `deps_resolve`: acknowledge advisories by their key

## Configuration

The discovery config is at `~/.config/deps/config.toml` (symlinked from
`recipes/deps/config.toml`). The repo set comes from the catalog registry; this
file trims it. Both lists are glob patterns matched against the full path and the
basename. Edits take effect on the next scan without a restart.

```toml
exclude_repos = ["archive-*"]      # skip whole repos
exclude_paths = ["third_party"]    # skip sub-directories while walking a repo
```

## Where things live

- Scan results: `~/.local/share/deps/scan.json`
- Config: `~/.config/deps/config.toml`
- Binary: `~/code/bin/deps`
- Web + API: `http://deps.this/` (or `http://localhost:7429/`)

Removing the service doesn't delete the scan file.

## Develop

```bash
make build    # ./deps binary
make install  # build + cp to ~/code/bin/deps + adhoc codesign
make test     # go test ./...
```

Tests are hermetic: they use temp dirs and canned `go list` / OSV / lockfile
fixtures and a fake notifier, so they never hit the network or fire a real
notification.

See [`CLAUDE.md`](CLAUDE.md) for the architecture: the data model, the HTTP
API, the MCP tools.
