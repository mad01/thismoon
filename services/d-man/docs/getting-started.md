# Getting started

This walks you from a fresh checkout to reaching a local service at
`http://present.this/`. It assumes macOS and [t-man](https://github.com/mad01/thismoon/tree/main/tools/t-man)
on your `PATH`.

## 1. Build and install

```bash
make install
```

This builds `d-man`, copies it to `~/code/bin/d-man`, and codesigns it on
macOS. Via ralph, `ralph up` does the same; the routes file is symlinked into
place by the consuming machine's dotfiles overlay recipe.

## 2. Write the routes file

`d-man` reads `~/.config/d-man/routes.toml` by default. Create it (the dotfiles
recipe symlinks this from `recipes/d-man/routes.toml`):

```toml
suffix = "this"

[[route]]
name = "present"
port = 7423

[[route]]
name = "csl"
port = 7424

[[route]]
name = "p"
cname = "present"
```

Check that it parses and see the resolved routes:

```bash
d-man list
```

```
suffix: this
  present.this                 -> 127.0.0.1:7423
  csl.this                     -> 127.0.0.1:7424
  p.this                       -> 127.0.0.1:7423 (cname present.this)
```

## 3. Register the daemon (one-time, needs root)

`d-man serve` binds `127.0.0.1:80` and writes `/etc/hosts`, both of which need
root, so it runs as a launchd daemon registered through t-man:

```bash
sudo t-man --daemon add --name d-man -- \
  $HOME/code/bin/d-man serve --config $HOME/.config/d-man/routes.toml
```

This is the only command that needs `sudo`. It writes
`/Library/LaunchDaemons/d-man.plist` (start on boot, restart on exit), syncs the
`/etc/hosts` block, and starts the proxy. Logs go to `/var/log/d-man/`.

## 4. Verify

```bash
t-man status d-man
curl http://present.this/      # resolved via /etc/hosts, proxied to :7423
curl http://p.this/            # the alias, same backend
open http://csl.this/          # works in Safari too, not only Chrome
```

## 5. Add or change a route

Edit `routes.toml` and save. The running daemon watches the file, re-validates,
re-syncs `/etc/hosts`, and rebuilds the proxy on its own. No `sudo`, no restart.

```toml
[[route]]
name = "grafana"
port = 3000
```

Reach it at `http://grafana.this/` within a second.

To add an alias for an existing route, point `cname` at its name:

```toml
[[route]]
name = "g"
cname = "grafana"
```

If an edit is invalid (bad hostname, unknown `cname` target, a `cname` cycle),
the daemon logs the error and keeps the last good routes and `/etc/hosts`
untouched. Check `t-man logs d-man` to see what it rejected.

## Upgrading

Run `make install` (or `ralph up`). The daemon notices its own binary changed,
exits, and launchd relaunches the new build. No `sudo`, no manual restart.

## Next steps

- [Command reference](commands.md) for every subcommand and flag.
- [README](../README.md) for the design rationale (why `/etc/hosts` and not DNS
  on macOS 26).
