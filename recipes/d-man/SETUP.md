# d-man setup (one-time)

`d-man` runs as a **root launchd daemon** because it binds `127.0.0.1:80` and
writes `/etc/hosts` — both need root. Everything *after* this one-time step is
sudo-free: edit `routes.toml` and the running daemon applies it; `ralph up`
rebuilds the binary and the daemon self-restarts via launchd KeepAlive.

## 1. Build + install (via ralph, or by hand)

```bash
ralph up                       # builds d-man from the thismoon source cache
# or, by hand:
make -C ~/code/src/github.com/mad01/thismoon/services/d-man install
```

This installs `~/code/bin/d-man`. The routes file
(`~/.config/d-man/routes.toml`) is machine-personal and comes from the
consuming repo's overlay recipe, which symlinks it into place — write one by
hand if you are not using an overlay (format: `services/d-man/README.md`).

## 2. Register the daemon (the only sudo)

```bash
sudo t-man --daemon add --name d-man -- \
  $HOME/code/bin/d-man serve --config $HOME/.config/d-man/routes.toml
```

This writes `/Library/LaunchDaemons/d-man.plist` (`RunAtLoad` → starts on
reboot, `KeepAlive` → restarts on kill), syncs the managed `/etc/hosts` block,
and starts the `:80` reverse proxy. Logs land in `/var/log/d-man/`.

## 3. Verify

```bash
t-man status d-man
t-man logs d-man
curl http://present.this/        # resolves via /etc/hosts, proxied to the backend
open http://csl.this/            # also works in Safari (system resolver path)
```

## Daily use

- **Add / change a route:** edit the `routes.toml` your daemon watches (in the
  overlay repo, tracked in git). The daemon notices and re-applies it — no
  sudo, no restart. A route is either port-backed (`port = 7423`) or a CNAME
  alias (`cname = "present"`) that resolves to another route's backend.
- **Upgrade the binary:** `ralph up` (or `make install`). The daemon self-exits
  and launchd relaunches the new build — no sudo.
- **Manual one-shot sync** (rarely needed): `sudo d-man sync`.
- **Remove the daemon:** `sudo t-man --daemon remove d-man`.

## Why not a real DNS server?

macOS 26 (Tahoe) `mDNSResponder` hijacks queries for non-IANA TLDs like `.this`
and never forwards them to a local nameserver, so the classic
dnsmasq + `/etc/resolver/this` approach silently fails. `*.localhost` only works
in Chrome/Firefox (not Safari/`curl`). `/etc/hosts` is read by `getaddrinfo`
*before* `mDNSResponder`, so it resolves in every client and keeps the `.this`
suffix — at the cost of explicit names (no wildcards), which matches the
explicit route list anyway.
