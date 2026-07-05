# pr

A Go CLI that serves a pull request review dashboard for repos you watch at
`http://pr.this/` (port 7427).

It polls open PRs from configured GitHub repos (github.com and GitHub
Enterprise hosts), renders diffs with syntax-aware line coloring, and supports
approve, request-changes, and squash-merge straight from the page.

## Install

```bash
make install   # builds and installs ~/code/bin/pr (adhoc codesigned on macOS)
```

Or via ralph — it ships from the thismoon monorepo:

```bash
ralph up   # builds pr and registers the t-man agent
```

## Configure

All GitHub access goes through the `gh` CLI (`gh api` with `GH_HOST` set per
source), so `gh` must be installed and authenticated against each host you
watch.

Watch lists live in `~/.config/pr/config.toml`:

```toml
poll_interval = "5m"

[[source]]
host = "github.com"      # default when omitted
owner = "someorg"
repos = ["repo-one", "repo-two"]
```

## Run

`pr serve` runs as a launchd user agent via t-man:

```bash
pr serve --port 7427
```

```bash
t-man status pr      # check the agent
t-man restart pr     # restart after a binary rebuild
t-man logs pr        # stdout logs
```

| Flag | Default | Description |
|------|---------|-------------|
| `--port` | `7427` | Listen port |
| `--config` | `~/.config/pr/config.toml` | Watch-list config |
| `--workdir` | `~/.local/share/pr` | Working data location |

A background poller refreshes all sources every `poll_interval` (default 5m)
and caches results in memory; PRs from archived repos are dropped
automatically.

## Endpoints

| Path | Description |
|------|-------------|
| `GET /` | PR list dashboard (client-rendered from `/api/prs`) |
| `GET /pr/{host}/{owner}/{repo}/{number}` | PR detail: markdown body + rendered diff |
| `GET /api/prs` | All cached PRs as JSON |
| `GET /api/pr/{host}/{owner}/{repo}/{number}` | One PR's detail as JSON (live fetch) |
| `POST /api/approve` | Approve a PR |
| `POST /api/request-changes` | Request changes (JSON body with message) |
| `POST /api/merge` | Squash-merge a PR |
| `POST /api/refresh` | Force re-poll all repos |
| `GET /healthz` | 204 |
| `GET /version` | `{"version":"<sha>"}` build sha |
| `GET /webkit/` | Shared chrome from the in-module `webkit` package |

## Develop

```bash
make test           # go test ./...
make build          # ./pr
```

Not sandboxed: the service shells out to `gh` for every API call, and a
seatbelt profile would block the subprocess.
