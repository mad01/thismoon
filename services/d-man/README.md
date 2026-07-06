# d-man

A local domain front door for macOS. Map friendly hostnames to localhost
services so you reach them by name on port 80 instead of remembering ports:

```
http://present.this/   ->  127.0.0.1:7423
http://csl.this/       ->  127.0.0.1:7424
http://p.this/         ->  alias of present.this
```

It does two things, both driven by one routes file:

1. **hosts sync**: writes a managed block into `/etc/hosts` so `<name>.<suffix>`
   resolves to `127.0.0.1`.
2. **reverse proxy**: listens on `127.0.0.1:80` and routes each request to the
   right backend port by its `Host` header.

## Why /etc/hosts and not a DNS server?

On macOS 26 (Tahoe), `mDNSResponder` hijacks DNS queries for any non-IANA TLD
(`.this`, `.test`, `.lan`) and answers them itself. A local DNS server with
`/etc/resolver/this` is registered but **never receives the query**.
`*.localhost` only resolves in Chrome/Firefox, not Safari or `curl`. `/etc/hosts`
is read by `getaddrinfo` *before* `mDNSResponder`, so it resolves in **every**
client and keeps the short `.this` suffix. The cost is no wildcards: every name
is listed explicitly, which is exactly what the routes file already is.

## Documentation

- [Getting started](docs/getting-started.md): install to first request, step by step.
- [Command reference](docs/commands.md): every subcommand and flag.
- [Setup](../../recipes/d-man/SETUP.md): one-time daemon registration on a new machine.

## Install

```bash
make install   # builds and installs ~/code/bin/d-man (codesigned on macOS)
```

## Run

`d-man serve` is the long-running daemon. Binding `:80` and writing `/etc/hosts`
both need root, so it runs as a **root launchd daemon** via t-man. This is a
one-time, explicit step (see `../../recipes/d-man/SETUP.md`):

```bash
sudo t-man --daemon add --name d-man -- \
  $HOME/code/bin/d-man serve --config $HOME/.config/d-man/routes.toml
```

That is the **only** time you need `sudo`. After it:

- **Edit routes:** change `routes.toml`; the daemon watches it (fsnotify) and
  re-syncs `/etc/hosts` plus reloads the proxy automatically. No sudo, no restart.
- **Upgrade the binary:** run `make install` (or `ralph up`); the daemon notices
  its binary changed, exits, and launchd's `KeepAlive` relaunches the new build.

Other subcommands:

```bash
d-man list                 # print resolved host -> backend routes
sudo d-man sync            # write the /etc/hosts block once (manual fallback)
d-man version [-o json]    # build sha
```

| Flag | Env | Default | Description |
|------|-----|---------|-------------|
| `--config` | `DMAN_CONFIG` | `~/.config/d-man/routes.toml` | Path to the routes file. |
| `--hosts-file` | _(none)_ | `/etc/hosts` | Hosts file to sync into; point at a temp file to dry-run. |
| `--port` (serve) | _(none)_ | `80` | Port the proxy listens on (loopback only). |

See the [command reference](docs/commands.md) for every subcommand in detail.

## Routes file

```toml
suffix = "this"            # default TLD; "csl" => csl.this

[[route]]
name = "present"           # http://present.this/ -> 127.0.0.1:7423
port = 7423

[[route]]
name = "csl"               # http://csl.this/     -> 127.0.0.1:7424
port = 7424

[[route]]
name = "p"                 # http://p.this/       -> alias of present.this
cname = "present"
```

A route is either **port-backed** (a `port`) or a **CNAME alias** (a `cname`
naming another route) that resolves to that route's backend. CNAME chains are
followed and cycles are rejected. `target` (default `127.0.0.1`) lets a
port-backed route point at a non-loopback host.

## Safety

`d-man` never corrupts `/etc/hosts`. It only ever replaces the text between its
two markers; every other line is preserved byte-for-byte. Before writing it
validates the result, backs up to `/etc/hosts.d-man.bak`, and replaces the file
atomically (temp then rename). A pre-existing malformed foreign line is preserved
and reported as a warning rather than aborting, but `d-man` refuses to write if
*its own* change would make a previously-valid file invalid. An invalid
`routes.toml` edit is logged and ignored; the last good routes stay live.

The reverse proxy normalizes the `Host` header (case, trailing FQDN dot, port)
and rewrites a backend's self-redirect `Location` back to the `.this` hostname,
so a redirect never bounces you to `127.0.0.1:<port>`.

## Debugging

**Host not resolving.** Confirm the entry is in `/etc/hosts`:

```bash
grep 'present.this' /etc/hosts
```

If the entry is missing, the daemon may have rejected the last `routes.toml`
edit. Check `t-man logs d-man` for the validation error; the previous good
routes stay live. Force a one-shot sync: `sudo d-man sync`.

**Proxy returning 502.** The backend is not running, or the port in
`routes.toml` doesn't match the backend's listen port. Check with:

```bash
curl -i http://127.0.0.1:<port>/
```

**Route change not applying.** Confirm the daemon is running (`t-man status
d-man`) and that the routes file is the one the daemon is watching
(`t-man logs d-man` shows `config=…` on start). A bad edit is silently
ignored; check for the validation error in the logs.

**`/__this/sites.json` missing a site.** The list is live-filtered: each
fetch probes the backend and excludes services that are not responding.
Confirm the backend is up on the declared port.

**Binary upgrade not taking effect.** The daemon watches its own executable
and exits on change; launchd `KeepAlive` relaunches the new build. If the
daemon is not relaunching, check `t-man status d-man` and launchd directly:
`launchctl print system/d-man`.

## Develop

```bash
make test    # go test ./...
make build   # ./d-man
make tidy    # go mod tidy
```

See `CLAUDE.md` for the module layout and design rationale, and
`../../recipes/d-man/SETUP.md` for first-time setup on a new machine.
