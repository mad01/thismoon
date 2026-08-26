# d-man configuration

d-man is driven by one routes file (TOML) plus a handful of flags that
control where that file lives and which ports the daemon binds.

## Where config lives

The routes file is TOML. It is resolved in this order, first match wins:

1. `--config <path>`
2. `DMAN_CONFIG` environment variable
3. `~/.config/d-man/routes.toml`, if it exists
4. `/etc/d-man/routes.toml`, if it exists (the fallback for the root launchd
   daemon, whose `HOME` points nowhere useful)

With none of those present, the per-user path (`~/.config/d-man/routes.toml`)
is reported as missing and d-man runs on built-in defaults: no routes, no
block list, suffix `this`. A leading `~` in any resolved path is expanded
before the file is opened.

`d-man serve` watches the resolved file with `fsnotify` and reloads on
content change or `SIGHUP`. The watch resolves symlinks once at startup and
watches the real file's directory: editing the file's content reloads;
repointing a symlink at a different file does not. A bad edit is logged and
ignored: the previous good routes and `/etc/hosts` state stay live.

**Ordering constraint:** `suffix`, `games_dir`, and `blocklist` are top-level
keys and must appear before the first `[[route]]` table in the file. TOML
parses every key that follows a `[[route]]` header as a field of that route
table, so a `blocklist` placed after the routes blocks nothing. The parser
never rejects this; it silently attaches the array to the last route instead.
`d-man config` prints the effective config so a misplaced key is visible as a
missing block list rather than a parse error.

## Keys

- `suffix` (string, default `"this"`): the label appended to every route
  name to form its hostname: route name `csl` with suffix `this` becomes
  `csl.this`. Must be a valid dotted hostname.
- `games_dir` (string, default unset): optional directory of extra block-page
  arcade games (`*.js`). Unset serves only the games embedded in the binary.
  A leading `~` is expanded. The directory is read per request, so dropping a
  file in takes effect on the next page load; a missing directory just means
  no extra games. Each file must be named with lowercase letters, digits,
  `-`, `_` and register itself via `ARCADE.register(name, factory)`.
- `blocklist` (array of strings, default empty): hostnames sent to the local
  block page instead of being proxied. Each entry is normalized for
  comparison (lowercased, trailing dot stripped) before being written to
  `/etc/hosts` pointed at `127.0.0.1`. An entry must be a valid hostname and
  must not also be a configured route host. `www.example.com` and
  `example.com` are separate entries; there is no wildcard matching.

### `[[route]]` (repeatable table)

One `[[route]]` block per name. A route is either **port-backed** (sets
`port`) or a **CNAME alias** (sets `cname`, naming another route). Setting
both, or neither, is a validation error.

- `route.name` (string, required): short label; becomes `<name>.<suffix>`.
  Dots are allowed in the name itself (each label is validated per RFC 1123).
  Names must be unique across the file.
- `route.port` (integer, 1-65535): the backend port on `target` this route
  proxies to. Required for a port-backed route; omit for a CNAME alias.
- `route.target` (string, default `"127.0.0.1"`): the backend host. Only
  meaningful on a port-backed route: a CNAME alias inherits its resolved
  target from the route it points at.
- `route.cname` (string): the name of another route in the same file to
  alias. CNAME chains are followed to their terminal port-backed route;
  cycles and missing targets are rejected at load time.

### Flags

- `--config` (string, default: see resolution order above; env `DMAN_CONFIG`):
  path to the routes file. Applies to every subcommand.
- `--hosts-file` (string, default `/etc/hosts`): the hosts file the managed
  block is written into. Point it at a temp file to dry-run `sync` or `serve`
  without touching the real system file.
- `--port` (serve only, default `80`): port the reverse proxy listens on
  (loopback only).
- `--tls-port` (serve only, default `443`): port the block-page TLS listener
  uses (loopback only).
- `--ca-dir` (serve and `ca` subcommands, default: a `ca` directory beside the
  resolved routes config, e.g. `~/.config/d-man/ca`): directory holding the
  block-page CA's `ca.pem` and `ca-key.pem`. The CA key is root-owned once
  installed; a non-root `serve` run for local testing needs its own
  `--ca-dir`.

## Environment variables

- `DMAN_CONFIG`: overrides the routes file path; see resolution order above.

## Example

```toml
# routes.toml: top-level keys must precede the first [[route]] table.

suffix = "this"

games_dir = "~/.config/d-man/games"

blocklist = [
  "news.ycombinator.com",
  "reddit.com",
  "www.reddit.com",
]

[[route]]
  name = "present"
  port = 7423

[[route]]
  name = "csl"
  port = 7424

[[route]]
  name = "p"          # http://p.this/ -> alias of present.this
  cname = "present"
```
