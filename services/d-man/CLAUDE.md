# d-man, local domain front door: `<name>.this` hostnames to localhost services

d-man maps friendly hostnames (`csl.this`, `present.this`) to localhost
services, reachable on port 80 with no port in the URL. It's a CLI plus a
long-running daemon (Go, cobra) that installs to `~/code/bin/d-man`. d-man is
a platform foundation of the `.this` stack: other recipes may take a hard
`depends_on` on it (`docs/adr/0006`).

## Module layout

```
d-man/
  operating.md           agent-facing operating doc, embedded via embed.go and
                         rendered by `d-man docs` from the facts in facts.go
                         (kit/agentdoc; facts.go also holds the routes-file
                         path constants internal/cli resolves with and the
                         UI settings file name)
  cmd/d-man/
    main.go              entrypoint, delegates to internal/cli.Execute
  internal/
    cli/                 cobra command tree: root, serve, sync, list, config, profile,
                         version, ca, docs
      root.go              flags (--config/DMAN_CONFIG, --hosts-file); the routes-file
                           cascade, whose ~/.config leg resolves through kit/confdir
                           (so XDG_CONFIG_HOME relocates it)
      serve.go             the daemon: reload loop, fsnotify watch, binary self-watch,
                           :80 proxy + :443 block-page TLS listener, the UI settings
                           store opened once and kept across route reloads
      sync.go              one-shot managed-block write
      list.go              print resolved host -> backend routes
      config.go            print the resolved routes file + the effective config
                           as TOML; annotated key reference lives in --help
      profile.go           print or switch the block-list profile (internal/profile)
      version.go           build metadata from the shared buildinfo package;
                           -o json convention shared with sibling tools
      ca.go                install/uninstall/path for the block-page CA (system keychain)
    config/               pure: parse + validate routes.toml (+ config_test.go);
                           Hosts(), RouteMap(), Sites(), BlockedHosts()
    profile/              pure Render (block list above the base routes, games_dir
                           ~/ expanded) + the Switch/Current shell that writes the
                           routes file and the profile record; testdata/ holds the
                           golden output per profile
    hosts/                pure Validate/Render/Splice + the Sync shell (+ hosts_test.go);
                           the fail-safe hosts-file writer
    proxy/                one httputil.ReverseProxy; Host-header routing + the
                           sites.json endpoint + block-page interception + the
                           settings.json branch (+ proxy_test.go, settings_test.go);
                           cors.go is the sites.json cross-origin allowlist
    settings/             the UI settings that follow the user between .this hosts:
                           settings.go is the document, the value table and the
                           Store (open, read, patch, save); handler.go is GET/PATCH,
                           the same-origin check, content type and body cap;
                           drift_test.go pins the table to the webkit source
    atomicfile/           temp file + rename writer shared by profile and settings
    tlsca/                local CA: persist + mint leaf certs for the configured
                           hosts only, set on every routes load (+ tlsca_test.go)
    blockpage/            embedded retro arcade served on blocked hosts, random
                           pick per visit; more games load as plugins from the
                           optional games_dir config key (go:embed assets/ +
                           blockpage_test.go); the kit (assets/arcade.js) steps
                           games at a fixed 60 a second, and arcade.test.mjs
                           runs it under node (make test-js)
  Makefile               - part of module github.com/mad01/thismoon (no own go.mod)
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

`serve` also runs a second listener on `127.0.0.1:443` sharing the same handler.
So a blocked host reaches the block page over **both** http and https (the
`:443` listener mints a trusted leaf from `internal/tlsca`). No scheme
redirect is involved — each port serves the page directly.

The CA is system-trusted, so it signs only for names d-man answers:
`reloadOnce` hands it `cfg.Hosts()` (route hosts plus block-list hosts, the
same names `/etc/hosts` gets) after every good load. Any other server name
gets `nil, nil` from `GetCertificate`, which crypto/tls turns into a fatal
`unrecognized_name` alert; a reload that drops a host drops its cached leaf.
A client with no server name gets the one fixed `localhost` leaf. **Never
mint for an arbitrary server name**: the cache would grow without bound and
the trusted CA would sign names d-man does not serve.

`internal/config` is the pure route model: a route is either port-backed
(`port`) or a CNAME alias (`cname` → another route's name); `resolve`/
`BackendFor` follow the chain and reject cycles + missing targets. `Validate`
fails fast on illegal hostnames before any I/O.

`internal/hosts` is pure `Validate`/`Render`/`Splice` plus the `Sync` shell.
It is fail-safe: it only ever replaces text between the two markers, backs up
to `/etc/hosts.d-man.bak`, and writes via atomic temp+rename. So it never
produces a file worse than it read (pre-existing foreign breakage is
preserved and warned, not aborted).

`internal/proxy` is one `httputil.ReverseProxy` whose `Rewrite` picks the
backend by `Host` (502 on miss). A host on the config `blocklist` is served
the local block page on every path instead of being proxied. The block page is
`internal/blockpage`, an embedded dependency-free retro arcade that picks one
game at random per visit. The check sits at the top of `ServeHTTP`, before the
route lookup. `normalizeHost` strips case / trailing dot / port so
`present.this`, `present.this.`, `PRESENT.this:80` all match. `ModifyResponse`
rewrites a backend self-redirect `Location` back to the client's hostname so
redirects don't leak `127.0.0.1:<port>`.

The proxy also answers `GET /__this/sites.json` itself for the webkit ⌘K
site picker, on configured route hosts only: it sits after the block check
and the route lookup, so an unknown host gets the 502 and a blocked host the
block page. **Keep every path d-man answers itself below those two checks**:
a page that points its own name at 127.0.0.1 (DNS rebinding) arrives with
that name as its `Host`, and the 502 is what stops it reading the answer
same-origin. Pages behind d-man reach it same-origin. A page on a localhost
port gets it cross-origin from a configured host only when its `Origin` is
loopback or ends in `.this` (`internal/proxy/cors.go`, reflected with
`Vary: Origin`). **Never widen that to `*`**. The response enumerates the
local services running on this machine.

The list is live-filtered: each fetch probes every port-backed route's backend
(`GET /`, anything `<500` = up) and lists only the ones responding. So a
service gated off or not running on a host never shows up. The probe result is
cached `sitesTTL` (30s) and a mutex makes a burst of fetches share one probe
round. `routes.toml` stays the full catalog of possible sites, and the picker
shows only the ones actually present.

`/__this/settings.json` is the other path d-man answers itself: the webkit
UI settings (theme, palette per mode, font, size, fixation) that follow the
user from one `.this` host to the next. Like `sites.json` it sits after the
block check and the route lookup in `ServeHTTP`. It never carries a CORS
header: every page reaches it same-origin, and a write must prove that with
its `Origin`. `serve` opens the store (`internal/settings`) once and hands the
same handler to every proxy it builds, so a route reload keeps the document.

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
make test-js                   # the arcade kit's node tests (make test runs both)
```

## HTTP API

- `GET /__this/sites.json`: navigable site list as JSON for the webkit ⌘K
  picker, live-filtered to backends currently responding (30s cache).
  Answered by d-man itself, not proxied, and only on a configured route
  host, after the block check: an unknown host gets the 502 and a blocked
  host the block page.
- `GET /__this/settings.json`: the webkit UI settings document,
  `{"v":1,"settings":{...}}` with only the keys ever set. `Cache-Control:
  no-store`, `X-Content-Type-Options: nosniff`. Answered only on a configured
  route host, after the block check: an unknown host gets the 502 and a
  blocked host the block page.
- `PATCH /__this/settings.json`: merges `{"v":1,"settings":{...}}` key by key
  and returns the full document. Needs `Content-Type: application/json`
  (else 415) and an `Origin` equal to the request's own scheme and host (else
  403); body capped at 4 KiB (413). A known key with a value the kit does not
  accept, or `v` other than 1, is a 400 and changes nothing. Unknown keys are
  dropped. A failed save is a 500 and memory stays as it was. Any other
  method, `OPTIONS` included, is a 405 with `Allow: GET, PATCH`. The path
  never sends an `Access-Control-*` header.
- Everything else: proxied to the matching route's backend by `Host` header
  (502 if no route matches).

## Commands

```bash
d-man serve [--port 80] [--tls-port 443]  # the long-running daemon: sync /etc/hosts, serve the
                                           # reverse proxy (:80) and block-page TLS listener (:443),
                                           # watch routes.toml + own binary; --settings-file moves
                                           # the UI settings file
sudo d-man sync                           # write the managed /etc/hosts block once and exit (manual fallback)
sudo d-man ca install                     # generate the block-page CA (if absent) and trust it in the
                                           # system keychain; uninstall/path too
d-man list                                # print resolved host -> backend routes
d-man config                              # print which routes file was loaded (loaded/missing/parse error)
                                           # and the effective config as TOML; --help has the key reference
d-man profile [name]                      # print the block-list profile, or switch: writes routes.toml
                                           # from routes.base.toml + blocklist.<name>.toml beside it
d-man version [-o json]                   # print the build sha; -o json prints the full build metadata
                                           # object (version, commit, tag, build_time)
d-man docs                                # print the embedded operating doc: failure modes, reload
                                           # semantics, first moves when a .this host stops resolving
```

## Gotchas

- **Runtime debugging lives in `operating.md`** (embedded in the binary,
  printed by `d-man docs`): the is-it-d-man-or-the-backend split, config
  resolution and reload semantics (symlink watch, blocklist-before-route
  ordering), CA trust, version-skew checks, first moves. Keep those facts
  there, not here.
- **`serve` needs root in production** (binds `:80`/`:443`, writes `/etc/hosts`).
  For local testing use `--port`/`--tls-port <high>`, `--hosts-file <temp>`, and
  a throwaway `--ca-dir <temp>` to avoid root.
- **The CA files are root-owned** (`ca-key.pem` is `0600`). The root daemon (or
  `sudo d-man ca install`) creates them, so even though they live under the
  user's config directory (`~/.config/d-man/ca/`, or under `XDG_CONFIG_HOME`),
  a non-root process cannot read the key. That is the point: the key mints
  system-trusted certs, so only root should hold it. A non-root `serve` for
  testing must therefore use its own `--ca-dir`.
- **`profile` keeps its record beside the routes file**, not under
  `XDG_STATE_HOME`. The record picks the inputs the command builds the routes
  file from, so it sits with them, and `--config <scratch>/routes.toml profile work`
  touches nothing outside the scratch directory. The golden files in
  `internal/profile/testdata/` match the shell generator the command replaced,
  byte for byte; changing them changes what every machine's daemon reads.
- **The UI settings file sits beside the routes file**, as `ui-settings.json`
  (`--settings-file` moves it), not under `XDG_STATE_HOME`. serve runs as
  root with no user HOME, and the routes file's directory is the only user
  path it holds. The root daemon writes the file `0644`. The value table in
  `internal/settings` copies what the kit accepts (theme modes, palette
  families, fonts, size bounds); `drift_test.go` reads the webkit source and
  `webkit.Themes()` and fails when they part. The webkit import stays in that
  test: d-man never links the kit.
- **`httputil.ReverseProxy` handles WebSocket upgrades natively**, so no
  custom Upgrade handling is needed.
- **Events go through `kit/notify`** (async emit; the per-tool
  `internal/notify` copy is gone, and so is the module-boundary argument that
  justified it — everything here is one module now).
- **Version probe.** `d-man version -o json` returns the shared four-key
  build metadata object (`version`, `commit`, `tag`, `build_time`, every key
  present and `""` when unknown) from `github.com/mad01/thismoon/buildinfo`,
  the cross-tool convention `ralph` and `status` use to probe the build a
  sibling tool is running. Plain `d-man version` stays a bare token. Unlike
  the webkit-mounted services, d-man has no `GET /version` HTTP route — it's a
  proxy/daemon, not a web UI.

## See also

- Recipe: `recipes/d-man/recipe.toml` (+ `recipes/d-man/CLAUDE.md`): wave 0,
  platform foundation (`docs/adr/0006`)
- Setup: `recipes/d-man/SETUP.md`: one-time daemon registration on a new machine
- Docs: `docs/getting-started.md` (install to first request), `docs/commands.md`
  (every subcommand and flag)
