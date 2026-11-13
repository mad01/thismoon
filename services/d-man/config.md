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

Step 3 honors `XDG_CONFIG_HOME`: when that variable holds an absolute path,
the per-user routes file is `$XDG_CONFIG_HOME/d-man/routes.toml` instead.
Everything else about the order is unchanged.

With none of those present, the per-user path is reported as missing and
d-man runs on built-in defaults: no routes, no block list, suffix `this`. A
leading `~` in any resolved path is expanded before the file is opened. A
home directory that cannot be resolved at all is an error rather than a path
relative to the working directory, and the `--config` default falls back to
`/etc/d-man/routes.toml`.

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

## Generated routes files (profiles)

A machine that switches block lists can generate the routes file with
`d-man profile <name>` instead of editing it. The inputs sit beside the routes
file: `routes.base.toml` holds the routes, `blocklist.<name>.toml` holds one
profile's top-level `blocklist`, and a file called `profile` records the active
name. The command writes the block list above the base, which keeps the
ordering constraint above, and rewrites a leading `~/` in `games_dir` to the
invoking user's home. Edit the inputs, not the generated file. Details and
failure behavior: [`docs/commands.md`](docs/commands.md#profile).

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
- `--settings-file` (serve only, default: `ui-settings.json` beside the
  resolved routes config, e.g. `~/.config/d-man/ui-settings.json`): where
  serve keeps the UI settings document. A leading `~` is expanded. See
  [UI settings](#ui-settings).

## Environment variables

- `DMAN_CONFIG`: overrides the routes file path; see resolution order above.
- `XDG_CONFIG_HOME`: relocates the per-user config directory (step 3 above)
  and, with it, the default `--ca-dir` and `--settings-file`.
- `EVENTS_BASE_URL`: where route-reload events are archived (default
  `http://127.0.0.1:7430`). Emitting is best effort: an unreachable events
  service drops the event and never affects a reload.

## Cross-origin access

`GET /__this/sites.json` is answered by d-man itself, on every configured
route host and no other: an unknown host gets the 502 and a blocked host the
block page. So a page that points its own name at 127.0.0.1 cannot read the
list as a same-origin request. Pages behind the front door fetch it
same-origin and need no CORS header at all. For the localhost-port case (a
page opened at `http://127.0.0.1:<port>` rather than through `<name>.this`),
d-man reflects the request `Origin` in `Access-Control-Allow-Origin`
alongside `Vary: Origin`. It does so only when that origin is an
`http`/`https` URL whose host is loopback (`localhost`, `127.0.0.0/8`,
`::1`, on any port) or ends in `.this`. Any other origin gets no CORS
headers, so a page from the internet cannot read the list of services
running on this machine.

The allowlist is fixed: no config key or flag widens it.

`/__this/settings.json` has no allowlist at all. It never sends a CORS
header, so only a page on the same host can read it. A PATCH whose `Origin`
is not the request's own scheme and host gets a 403.

## UI settings

serve keeps the webkit UI settings (theme mode, palette per mode, font, size,
fixation reading) in one JSON document, so a choice made on one `.this` host
shows on every other. The file is `ui-settings.json` beside the routes file,
or wherever `--settings-file` points:

```json
{
  "v": 1,
  "settings": {
    "webkit-size": "18",
    "webkit-theme": "dark"
  }
}
```

The file is state the daemon writes, not config to edit: serve reads it once
at start and rewrites it on every change. It holds only keys that were set,
each with a value the kit accepts:

| Key | Values |
|-----|--------|
| `webkit-theme` | `light`, `dark`, `system` |
| `webkit-palette-light`, `webkit-palette-dark` | `default` or a palette family the kit ships |
| `webkit-font` | `Fira Code`, `Inter`, `Lexend`, `Work Sans` |
| `webkit-size` | a whole number from `12` to `24` |
| `webkit-fixation` | `true`, `false` |

A missing file means no settings. A file that does not parse is logged on
stderr and serve starts with no settings; entries with an unknown key or a
refused value are dropped with a warning.

Settings sync only on pages served through d-man under the `.this` suffix. A
page on a localhost port, or a machine with a custom `suffix`, keeps its
settings in that origin's own browser storage.

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
