# d-man

Local domain front door for macOS: friendly hostnames (`csl.this`,
`present.this`) resolve to localhost services, reachable on port 80 with no port
in the URL. CLI + long-running daemon (Go, cobra). Installs to `~/code/bin/d-man`.

## Why this shape (read before changing the resolution mechanism)
macOS 26 (Tahoe) `mDNSResponder` hijacks DNS for non-IANA TLDs (`.this`, `.test`,
`.lan`) and answers them as multicast DNS — a local DNS server + `/etc/resolver`
**silently fails**. `*.localhost` only resolves in Chrome/Firefox, not
Safari/`curl`. So d-man uses **`/etc/hosts`** (read by `getaddrinfo` before
`mDNSResponder`): works in every client, keeps the `.this` suffix, no wildcards.

## Two jobs (functional core, imperative shell)
1. **hosts sync** — write a marker-delimited managed block into `/etc/hosts`
   mapping each `<name>.<suffix>` → `127.0.0.1`.
2. **reverse proxy** — `127.0.0.1:80`, route by `Host` header to the backend port.

## Layout
- `cmd/d-man/main.go` — entry → `cli.Execute()`.
- `internal/cli/` — `root` (flags: `--config`/`DMAN_CONFIG`, `--hosts-file`),
  `serve` (daemon), `sync` (one-shot hosts write), `list`, `version`.
- `internal/config/` — pure: parse + validate `routes.toml`, `Hosts()`,
  `RouteMap()`. Fails fast on illegal hostnames before any I/O. A route is
  either port-backed (`port`) or a CNAME alias (`cname` → another route's name);
  `resolve`/`BackendFor` follow the chain and reject cycles + missing targets.
- `internal/hosts/` — pure `Validate`/`Render`/`Splice` + the `Sync` shell.
  **Fail-safe:** only ever replaces text between the two markers; backs up to
  `/etc/hosts.d-man.bak`; atomic temp+rename; never produces a file worse than
  it read (pre-existing foreign breakage is preserved + warned, not aborted).
- `internal/proxy/` — one `httputil.ReverseProxy` whose `Rewrite` picks the
  backend by `Host` (502 on miss). `normalizeHost` strips case / trailing dot /
  port so `present.this`, `present.this.`, `PRESENT.this:80` all match.
  `ModifyResponse` rewrites a backend self-redirect `Location` back to the
  client's hostname so redirects don't leak `127.0.0.1:<port>`. The proxy also
  answers `GET /__this/sites.json` itself (any host) for the webkit ⌘K site
  picker. The list is **live-filtered**: each fetch probes every port-backed
  route's backend (`GET /`, anything `<500` = up) and lists only the ones
  responding, so a service gated off or not running on a host never shows up.
  The probe result is cached `sitesTTL` (30s) and a mutex makes a burst of
  fetches share one probe round — `routes.toml` stays the full catalog of
  *possible* sites; the picker sees only *present* ones.

## Sudo model (the whole point)
Only **one** sudo ever: the one-time `t-man --daemon add` (root daemon for `:80`
+ `/etc/hosts`). After that:
- **Route edits** — `serve` watches `routes.toml` (fsnotify) → re-sync + reload.
- **Binary upgrades** — `serve` watches `os.Executable()` → `exit(0)`; launchd
  KeepAlive relaunches the new build. So `ralph up` needs no sudo.

A bad `routes.toml` edit is logged and ignored; the previous good routes +
`/etc/hosts` stay in place.

## Build / test
```bash
make build && make install     # ~/code/bin/d-man, codesigned ("mad01 Local Signing" when present)
go test ./...
```

## Gotchas
- `serve` needs root in production (binds `:80`, writes `/etc/hosts`). For local
  testing use `--port <high>` and `--hosts-file <temp>` to avoid root.
- `httputil.ReverseProxy` handles WebSocket upgrades natively.
- Setup + daemon registration: see `recipes/d-man/SETUP.md`.
