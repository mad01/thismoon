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

## How it works

On macOS 26 (Tahoe), `mDNSResponder` hijacks DNS queries for any non-IANA TLD
(`.this`, `.test`, `.lan`) and answers them itself. A local DNS server with
`/etc/resolver/this` is registered but **never receives the query**.
`*.localhost` only resolves in Chrome/Firefox, not Safari or `curl`.
`/etc/hosts` is read by `getaddrinfo` *before* `mDNSResponder`, so it resolves
in **every** client and keeps the short `.this` suffix. The cost is no
wildcards: every name is listed explicitly, which is exactly what the routes
file already is. See [`CLAUDE.md`](CLAUDE.md) for the module layout and the
fail-safe writer/proxy design.

## Install

```bash
make install   # builds and installs ~/code/bin/d-man (codesigned on macOS)
```

See [`docs/getting-started.md`](docs/getting-started.md) for the full
walkthrough, install to first request.

## Usage

`d-man serve` is the long-running daemon. Binding `:80` and writing `/etc/hosts`
both need root, so it runs as a **root launchd daemon** via t-man. This is a
one-time, explicit step (see [`recipes/d-man/SETUP.md`](../../recipes/d-man/SETUP.md)):

```bash
sudo t-man --daemon add --name d-man -- \
  $HOME/code/bin/d-man serve --config $HOME/.config/d-man/routes.toml
```

That is the **only** time you need `sudo`. After it:

- **Edit routes:** change `routes.toml`; the daemon watches it (fsnotify) and
  re-syncs `/etc/hosts` plus reloads the proxy automatically. No sudo, no
  restart. If an edit is invalid (bad hostname, unknown `cname` target, a
  `cname` cycle), it's logged and ignored, and the previous good routes stay
  live; check `t-man logs d-man` for the validation error, and confirm the
  daemon is watching the file you edited (`t-man logs d-man` shows
  `config=…` on start).
- **Upgrade the binary:** run `make install` (or `ralph up`); the daemon
  notices its binary changed, exits, and launchd's `KeepAlive` relaunches the
  new build. If it doesn't take effect, check `t-man status d-man` and
  launchd directly: `launchctl print system/d-man`.

Other subcommands:

```bash
d-man list                 # print resolved host -> backend routes
d-man config               # print the routes file in use and the effective config
sudo d-man sync             # write the /etc/hosts block once (manual fallback)
sudo d-man ca install       # trust the local block-page CA (one-time, see below)
d-man version [-o json]    # build sha; -o json adds commit, tag, build time
```

| Flag | Env | Default | Description |
|------|-----|---------|-------------|
| `--config` | `DMAN_CONFIG` | `~/.config/d-man/routes.toml`, then `/etc/d-man/routes.toml` | Path to the routes file, first existing path wins. |
| `--hosts-file` | _(none)_ | `/etc/hosts` | Hosts file to sync into; point at a temp file to dry-run. |
| `--port` (serve) | _(none)_ | `80` | Port the proxy listens on (loopback only). |
| `--tls-port` (serve) | _(none)_ | `443` | Port the block-page TLS listener uses (loopback only). |
| `--ca-dir` (serve, ca) | _(none)_ | `<config dir>/ca` | Directory holding the block-page CA cert and key. |

If a hostname isn't resolving, confirm the entry landed in `/etc/hosts`:

```bash
grep 'present.this' /etc/hosts
```

See [`docs/commands.md`](docs/commands.md) for every subcommand in detail.

## Endpoints

- `GET /__this/sites.json`: navigable site list as JSON for the webkit ⌘K
  picker, live-filtered to backends currently responding (any host; answered
  by d-man itself, not proxied). If a running site is missing from the list,
  confirm its backend is actually up on the declared port: a dead backend is
  dropped, not shown as down.
- Everything else: proxied to the matching route's backend by `Host` header.
  A `502` means the backend isn't running, or the port in `routes.toml`
  doesn't match the backend's listen port; check with
  `curl -i http://127.0.0.1:<port>/`.

## Configuration

```toml
suffix = "this"            # default TLD; "csl" => csl.this

blocklist = [              # hosts sent to the local block page instead
  "reddit.com",
  "www.reddit.com",
]

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

## Blocking sites

`blocklist` is a top-level array of hostnames (put it before the `[[route]]`
tables, like `suffix`). Each listed host is pinned to `127.0.0.1` in
`/etc/hosts` and served a local "you're blocked" page hosting a small
dependency-free retro arcade, rather than being proxied. List `www.` and the
bare domain separately — there is no wildcard matching.

The arcade picks one game at random per visit (`n` switches). More games can be
added as plugins with the optional top-level `games_dir` key:

```toml
games_dir = "~/.config/d-man/games"
```

Any flat `<name>.js` file in that directory (lowercase letters, digits, `-`,
`_`) is loaded into the page after the built-in games and joins the rotation by
calling `ARCADE.register(name, factory)` — the same one-file contract the
bundled games use (see `internal/blockpage/assets/smash.js` for a complete
example). The directory is read per request, so dropping a file in takes effect
on the next page load; a missing directory simply means no extra games.

Distraction sites force HTTPS via HSTS, so the browser hits port 443 before you
ever see plain HTTP. `d-man serve` listens on `127.0.0.1:443` too and mints a
certificate per host from a local certificate authority. Trust that CA once so
the block page loads without a warning (writing the system keychain needs root):

```bash
sudo d-man ca install      # generate the CA if needed and trust it
```

`d-man ca uninstall` removes it; `d-man ca path` prints the CA directory.

## Where things live

- Binary: `~/code/bin/d-man`
- Routes config: `~/.config/d-man/routes.toml`, falling back to
  `/etc/d-man/routes.toml` for root daemons with no useful HOME (or
  `--config`/`DMAN_CONFIG`)
- Managed block: inside `/etc/hosts`, between the `# >>> d-man managed >>>` /
  `# <<< d-man managed <<<` markers; every other line stays untouched
- Backup: `/etc/hosts.d-man.bak`, written before each change
- Block-page CA: `~/.config/d-man/ca/` (`ca.pem` + `ca-key.pem`), or `--ca-dir`
- Daemon logs: `/var/log/d-man/` (the root launchd daemon registered by t-man)

## Develop

```bash
make test    # go test ./...
make build   # ./d-man
make tidy    # go mod tidy
```

See [`CLAUDE.md`](CLAUDE.md) for the module layout and design rationale, and
[`recipes/d-man/SETUP.md`](../../recipes/d-man/SETUP.md) for first-time setup
on a new machine.

## Docs

- [architecture](architecture.md): internal structure and data flow
- [operating](operating.md): runtime behavior, failure modes, first moves
- [why](why.md): why this component exists
- [config](config.md): configuration reference
