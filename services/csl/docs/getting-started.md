# Getting started

This walks from a blank machine to your first cross-repo search. Allow about five minutes.

## Prerequisites

- Go 1.25 or newer. Check with `go version`.
- `git` on `PATH`. `csl` reads `.git/config` directly for repo names, but uses `git` for pull and fingerprinting.
- At least one local git checkout.

## 1. Install

```sh
go install github.com/mad01/thismoon/services/csl/cmd/csl@latest
```

This puts `csl` in `$(go env GOBIN)` (or `$(go env GOPATH)/bin` if `GOBIN` is unset). Make sure that directory is on your `PATH`.

From a checkout:

```sh
git clone https://github.com/mad01/thismoon.git
cd thismoon/services/csl
make install
```

`make install` builds with the short git hash embedded as the version, copies the binary to `~/code/bin/csl`, strips the macOS quarantine attribute, and ad-hoc codesigns it.

Verify:

```sh
csl version
```

## 2. Configure

Create `~/.config/csl/config.yaml` and list the directories that contain your git checkouts. One entry per root. `csl` walks each concurrently.

```yaml
dirs:
  - ~/code/src/github.com
  - ~/code/src/github.com/myorg
  - ~/workspace
```

Tildes are expanded. Missing directories are skipped, not errored.

## 3. List discovered repos

```sh
csl repo --list
```

Output, one line per repo, tab-separated name and path:

```
myorg/service-a	/Users/you/code/src/github.com/myorg/service-a
myorg/service-b	/Users/you/code/src/github.com/myorg/service-b
mad01/thismoon	/Users/you/code/src/github.com/mad01/thismoon
```

Repo names come from the `origin` remote URL in `.git/config`. If a checkout has no remote, the name falls back to the parent directory plus the repo directory.

If the list is empty, the `dirs` paths don't contain git repos. Re-check the config; use absolute paths to rule out tilde expansion issues.

## 4. Run your first search

```sh
csl search "func Walk"
```

On the first call with no prior index, `csl` prints progress:

```
Indexing 17 repo(s)...
  [1/17] myorg/service-a
  [2/17] myorg/service-b
  ...
myorg/service-a/internal/walker/walker.go
mad01/thismoon/services/csl/internal/repo/finder/walker.go
```

The default output mode, `files_with_matches`, prints one line per matched file as `<repo>/<path>`. To see the matching lines instead:

```sh
csl search "func Walk" --output-mode content --context-lines 2
```

## 5. Filter a search

```sh
csl search "TODO" --repo thismoon --lang go
```

All filters compose. Under the hood, each flag adds a zoekt clause to the query: `repo:<regex>`, `lang:<name>`, `file:<regex>`, `case:yes`.

## 6. Check index health

```sh
csl doctor
```

Output:

```
Index directory: /Users/you/.config/csl/search-index
Index size:      84.3 MB
Total repos:     17
Index shards:    17 (17 healthy, 0 corrupted)
Search daemon:   running

Healthy (17):
  myorg/service-a                          indexed 3m12s ago
  ...
```

`doctor` reports corrupted shards, stale repos, dirty working trees, and daemon status in one view. Run it when results look off.

## 7. Register the MCP server

```sh
claude mcp add --scope user csl -- csl mcp
claude mcp list
```

Expected output:

```
csl: csl mcp - ✓ Connected
```

From a Claude Code session, the agent can now call `csl_search`, `csl_repo_lookup`, `csl_read`, and the seven other `csl_*` tools directly. See the [MCP reference](mcp.md) for per-tool contracts.

## Next

- [CLI reference](cli.md) — every subcommand and flag.
- [Configuration reference](configuration.md) — every file under `~/.config/csl/`.
- [Architecture](architecture.md) — how the daemon, index, and MCP server fit together.
