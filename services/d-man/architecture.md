# d-man architecture

## Overview

d-man is the front door of the `.this` platform: one long-running process,
`d-man serve`, registered with t-man as a root LaunchDaemon because it binds
`127.0.0.1:80` and `127.0.0.1:443` and writes `/etc/hosts`. It resolves
`<name>.this` hostnames by keeping a managed block in `/etc/hosts` and routes
each request to the right backend port by its `Host` header. Both listeners
are loopback-only; d-man never exposes anything beyond the machine and never
starts or supervises the backends it routes to (that is t-man's job). It has
no `.this` address of its own — every `.this` request passes through it.

## Structure

```
cmd/d-man/           entrypoint, delegates to internal/cli.Execute
internal/cli/        cobra command tree: serve, sync, list, ca, version
internal/config/     pure routes.toml model: parse, validate, resolve cname chains
internal/hosts/      pure Validate/Render/Splice plus the fail-safe Sync writer
internal/proxy/      one httputil.ReverseProxy: Host routing, blocklist, sites.json
internal/tlsca/      local CA: persist the root, mint per-SNI leaf certs
internal/blockpage/  embedded retro arcade served on blocked hosts (go:embed)
internal/notify/     best-effort event emit to events.this
```

d-man is not a webkit consumer: it serves no webkit-chromed page of its own.
It answers `GET /__this/sites.json`, which webkit's command-K site picker in
the other services fetches to list navigable `.this` sites.

## Data flow

One routes file drives everything. On start (and on every change
detected by the fsnotify watch in `internal/cli/serve.go`), `internal/config`
parses and validates `routes.toml`; a bad edit is logged and ignored while the
previous good routes stay live. From the validated routes, `internal/hosts`
renders the managed block and splices it between its two markers in
`/etc/hosts`: validate the result, back up to `/etc/hosts.d-man.bak`, write
via temp file plus atomic rename. The same routes build the proxy's route map.

A request on `:80` enters `internal/proxy.ServeHTTP`: `normalizeHost` strips
case, trailing dot, and port; a blocklisted host is answered directly with the
embedded block page from `internal/blockpage` (one game per visit, plus
optional plugin games read per request from `games_dir`); d-man answers
`/__this/sites.json` itself, live-filtered by probing each port-backed backend
(30 s cache, shared probe round); everything else goes through the
`ReverseProxy` `Rewrite` to the backend resolved from the `Host` header (502
on miss), with `ModifyResponse` rewriting backend self-redirects back to the
`.this` hostname. The `:443` listener shares the same handler and mints a
trusted leaf certificate per SNI from `internal/tlsca`, so blocked hosts that
force HTTPS still reach the block page.

`serve` also watches its own executable (`os.Executable()`) and exits cleanly
when it changes; launchd's KeepAlive relaunches the new build, so upgrades
need no sudo.

## Storage

- `/etc/hosts` — only the text between the `# >>> d-man managed >>>` and
  `# <<< d-man managed <<<` markers; every other line is preserved.
- `/etc/hosts.d-man.bak`: backup written before each change.
- `~/.config/d-man/ca/`: block-page CA (`ca.pem`, root-owned `ca-key.pem`),
  created by `d-man ca install` or the root process; override with `--ca-dir`.
- `~/.config/d-man/routes.toml`: read, never written.
- `/var/log/d-man/`: logs of the root LaunchDaemon registered by t-man.

## Interfaces

HTTP: `GET /__this/sites.json` (answered by d-man), everything else proxied by
`Host` header, on both `:80` and `:443`.

CLI: `d-man serve [--port] [--tls-port]`, `sudo d-man sync` (one-shot hosts
write), `d-man list`, `sudo d-man ca install|uninstall|path`, `d-man version
[-o json]`.

Config: `routes.toml` (path via `--config` or `DMAN_CONFIG`) with top-level
`suffix`, `blocklist`, and `games_dir`, plus `[[route]]` tables that are either
port-backed (`port`, optional `target`) or CNAME aliases (`cname`); chains are
followed and cycles rejected. `--hosts-file` points the writer at an alternate
file for dry runs.
