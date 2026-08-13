# d-man, local domain front door: `<name>.this` hostnames to localhost services

d-man maps friendly hostnames (`csl.this`, `present.this`) to localhost
services, reachable on port 80 with no port in the URL. It's a CLI plus a
long-running daemon (Go, cobra) that installs to `~/code/bin/d-man`. d-man is
a platform foundation of the `.this` stack: other recipes may take a hard
`depends_on` on it (`docs/adr/0006`).

## Module layout

```
d-man/
  cmd/d-man/
    main.go              entrypoint, delegates to internal/cli.Execute
  internal/
    cli/                 cobra command tree: root, serve, sync, list, config, version, ca
      root.go              flags (--config/DMAN_CONFIG, --hosts-file)
      serve.go             the daemon: reload loop, fsnotify watch, binary self-watch,
                           :80 proxy + :443 block-page TLS listener
      sync.go              one-shot managed-block write
      list.go              print resolved host -> backend routes
      config.go            print the resolved routes file + the effective config
                           as TOML; annotated key reference lives in --help
      version.go           build metadata from the shared buildinfo package;
                           -o json convention shared with sibling tools
      ca.go                install/uninstall/path for the block-page CA (system keychain)
    config/               pure: parse + validate routes.toml (+ config_test.go);
                           Hosts(), RouteMap(), Sites(), BlockedHosts()
    hosts/                pure Validate/Render/Splice + the Sync shell (+ hosts_test.go);
                           the fail-safe hosts-file writer
    proxy/                one httputil.ReverseProxy; Host-header routing + the
                           sites.json endpoint + block-page interception (+ proxy_test.go)
    tlsca/                local CA: persist + mint per-SNI leaf certs (+ tlsca_test.go)
    blockpage/            embedded retro arcade served on blocked hosts, random
                           pick per visit; more games load as plugins from the
                           optional games_dir config key (go:embed assets/ +
                           blockpage_test.go)
    notify/               events.this emit, best-effort (per-tool copy, see Gotchas)
  Makefile               part of module github.com/mad01/thismoon (no own go.mod)
```

## How it works

d-man does two jobs, both driven by one routes file (`routes.toml`).

### Why this shape (read before changing the resolution mechanism)
macOS 26 (Tahoe) `mDNSResponder` hijacks DNS for non-IANA TLDs (`.this`,
`.test`, `.lan`) and answers them as multicast DNS, so a local DNS server
plus `/etc/resolver` silently fails. `.localhost` names only resolve in
Chrome/Firefox, not Safari/`curl`. So d-man uses `/etc/hosts` (read by
`getaddrinfo` before `mDNSResponder`): it works in every client and keeps the
`.this` suffix, at the cost of listing every name explicitly instead of
using a wildcard.

### Two jobs (functional core, imperative shell)
1. hosts sync: write a marker-delimited managed block into `/etc/hosts`
   mapping each `<name>.<suffix>` (and each `blocklist` host) → `127.0.0.1`.
2. reverse proxy: `127.0.0.1:80`, route by `Host` header to the backend port.

`serve` also runs a second listener on `127.0.0.1:443` sharing the same handler,
so a blocked host reaches the block page over **both** http and https (the
`:443` listener mints a trusted leaf per SNI from `internal/tlsca`). No scheme
redirect is involved — each port serves the page directly.

`internal/config` is the pure route model: a route is either port-backed
(`port`) or a CNAME alias (`cname` → another route's name); `resolve`/
`BackendFor` follow the chain and reject cycles + missing targets. `Validate`
fails fast on illegal hostnames before any I/O.

`internal/hosts` is pure `Validate`/`Render`/`Splice` plus the `Sync` shell.
It is fail-safe: it only ever replaces text between the two markers, backs up
to `/etc/hosts.d-man.bak`, and writes via atomic temp+rename, so it never
produces a file worse than it read (pre-existing foreign breakage is
preserved and warned, not aborted).

`internal/proxy` is one `httputil.ReverseProxy` whose `Rewrite` picks the
backend by `Host` (502 on miss). A host on the config `blocklist` is served the
local block page (`internal/blockpage`, an embedded dependency-free retro
arcade, one game picked at random per visit) on every path instead of being
proxied — the check sits at the top of
`ServeHTTP`, before the route lookup. `normalizeHost` strips case / trailing dot
/ port so `present.this`, `present.this.`, `PRESENT.this:80` all match.
`ModifyResponse` rewrites a backend self-redirect `Location` back to the
client's hostname so redirects don't leak `127.0.0.1:<port>`. The proxy also
answers `GET /__this/sites.json` itself (any host) for the webkit ⌘K site
picker. The list is live-filtered: each fetch probes every port-backed
route's backend (`GET /`, anything `<500` = up) and lists only the ones
responding, so a service gated off or not running on a host never shows up.
The probe result is cached `sitesTTL` (30s) and a mutex makes a burst of
fetches share one probe round; `routes.toml` stays the full catalog of
possible sites, and the picker shows only the ones actually present.

### Sudo model (the whole point)
Two one-time sudos: `t-man --daemon add` (root daemon for `:80`/`:443` +
`/etc/hosts`) and, only if you use the block list, `d-man ca install` (trust the
block-page CA in the system keychain). After that, `serve` handles routine
changes on its own:
- route edits: it watches `routes.toml` (fsnotify) and re-syncs + reloads;
- binary upgrades: it watches `os.Executable()` and `exit(0)`s on a change;
  launchd KeepAlive relaunches the new build, so `ralph up` needs no sudo.

A bad `routes.toml` edit is logged and ignored; the previous good routes +
`/etc/hosts` stay in place.

## Build / install / test

```bash
make build && make install     # ~/code/bin/d-man, codesigned ("mad01 Local Signing" when present)
go test ./...
```

## HTTP API

- `GET /__this/sites.json`: navigable site list as JSON for the webkit ⌘K
  picker, live-filtered to backends currently responding (30s cache, any
  host). Answered by d-man itself, not proxied.
- Everything else: proxied to the matching route's backend by `Host` header
  (502 if no route matches).

## Commands

| Command | Description |
|---|---|
| `d-man serve [--port 80] [--tls-port 443]` | The long-running daemon: sync `/etc/hosts`, serve the reverse proxy (`:80`) and block-page TLS listener (`:443`), watch `routes.toml` + own binary. |
| `sudo d-man sync` | Write the managed `/etc/hosts` block once and exit (manual fallback). |
| `sudo d-man ca install` | Generate the block-page CA (if absent) and trust it in the system keychain; `uninstall`/`path` too. |
| `d-man list` | Print resolved host -> backend routes. |
| `d-man config` | Print which routes file was loaded (`loaded` / `missing` / `parse error`) and the effective config as TOML; `--help` carries the annotated key reference. |
| `d-man version [-o json]` | Print the build sha; `-o json` prints the full build metadata object (`version`, `commit`, `tag`, `build_time`). |

## Gotchas

- **`serve` needs root in production** (binds `:80`/`:443`, writes `/etc/hosts`).
  For local testing use `--port`/`--tls-port <high>`, `--hosts-file <temp>`, and
  a throwaway `--ca-dir <temp>` to avoid root.
- **The CA files are root-owned** (`ca-key.pem` is `0600`). The root daemon (or
  `sudo d-man ca install`) creates them, so even though they live under the
  user's `~/.config/d-man/ca/`, a non-root process cannot read the key — that is
  the point: the key mints system-trusted certs, so only root should hold it. A
  non-root `serve` for testing must therefore use its own `--ca-dir`.
- **`httputil.ReverseProxy` handles WebSocket upgrades natively**, so no
  custom Upgrade handling is needed.
- **`internal/notify` is a deliberate per-tool copy** of the same ~25-line
  events.this emit helper carried by reminder, deps, t-man, status, present,
  and speak. The tools are separate Go modules (and t-man is a separate repo),
  so a shared package would need require+replace coupling across module
  boundaries; the copy is cheaper.

## See also

- Recipe: `recipes/d-man/recipe.toml` (+ `recipes/d-man/CLAUDE.md`): wave 0,
  platform foundation (`docs/adr/0006`)
- Setup: `recipes/d-man/SETUP.md`: one-time daemon registration on a new machine
- Docs: `docs/getting-started.md` (install to first request), `docs/commands.md`
  (every subcommand and flag)
