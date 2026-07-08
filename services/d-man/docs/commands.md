# Command reference

```
d-man [global flags] <command>
```

## Global flags

These apply to every subcommand.

| Flag | Env | Default | Description |
|------|-----|---------|-------------|
| `--config` | `DMAN_CONFIG` | `~/.config/d-man/routes.toml` | Path to the routes file. A leading `~` is expanded. |
| `--hosts-file` | — | `/etc/hosts` | Hosts file to sync the managed block into. Override it to dry-run against a temp file. |

## serve

Run the long-running daemon: sync `/etc/hosts`, then serve the reverse proxy.

```bash
d-man serve --config ~/.config/d-man/routes.toml
```

| Flag | Default | Description |
|------|---------|-------------|
| `--port` | `80` | Port the proxy listens on (loopback only). |
| `--tls-port` | `443` | Port the block-page TLS listener uses (loopback only). |
| `--ca-dir` | `<config dir>/ca` | Directory holding the block-page CA cert and key. |

On start it writes the managed `/etc/hosts` block and builds the proxy. It also
listens on `127.0.0.1:443` to serve the block page for blocked HTTPS hosts,
minting a certificate per host from the local CA (see [`ca`](#ca)). While
running it:

- watches the routes file and re-syncs + reloads when it changes,
- reloads on `SIGHUP`,
- watches its own binary and exits when it changes, so launchd relaunches the
  new build.

Binding `:80` and writing `/etc/hosts` need root, so `serve` runs as a root
launchd daemon (see [getting started](getting-started.md)). For local testing
without root, use a high `--port` and a temp `--hosts-file`:

```bash
d-man serve --port 18080 --hosts-file /tmp/hosts --config ./routes.toml
```

## sync

Write the managed `/etc/hosts` block once from the routes file, then exit.

```bash
sudo d-man sync
```

This is a manual fallback. When the daemon is running it syncs automatically on
every routes change, so you rarely need `sync`. It validates before writing,
backs up to `/etc/hosts.d-man.bak`, and replaces the file atomically. Writing
`/etc/hosts` needs root; without it the command reports a permission error and
suggests `sudo`.

Output when the file changed:

```
synced 3 hosts to /etc/hosts (backup: /etc/hosts.d-man.bak)
```

When already current:

```
/etc/hosts already up to date
```

## ca

Manage the local certificate authority that lets blocked HTTPS hosts show the
block page with a trusted certificate.

```bash
sudo d-man ca install     # generate the CA if absent, trust it in the system keychain
sudo d-man ca uninstall   # remove it from the keychain
d-man ca path             # print the CA directory
```

| Flag | Default | Description |
|------|---------|-------------|
| `--ca-dir` | `<config dir>/ca` | Directory holding the CA cert (`ca.pem`) and key (`ca-key.pem`). |

`install` and `uninstall` write the system keychain, which needs root; without
it they report a permission error and suggest `sudo`. Run `install` once per
machine — `serve` reuses the same CA to sign per-host leaf certificates on the
fly. A host reaches the block page only when it is in the config `blocklist`.

## list

Print the resolved host-to-backend routes from the config.

```bash
d-man list
```

```
suffix: this
  present.this                 -> 127.0.0.1:7423
  csl.this                     -> 127.0.0.1:7424
  p.this                       -> 127.0.0.1:7423 (cname present.this)
```

A `cname` route shows the backend it resolves to and the alias target.

## version

Print the git commit the binary was built from.

```bash
d-man version
d-man version -o json     # {"version":"<sha>"}
```

| Flag | Default | Description |
|------|---------|-------------|
| `-o`, `--output` | `text` | Output format: `text` or `json`. |

The JSON form is the cross-tool convention sibling tools follow, so `ralph` can
probe any of them for the build they are running.

## Exit codes

`d-man` exits non-zero on a config or hosts error: an invalid routes file
(bad hostname, unknown `cname` target, `cname` cycle, port out of range), or a
hosts write that would corrupt the file or lacks permission. The error prints to
stderr.
