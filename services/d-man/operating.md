# operating d-man

d-man is the layer that makes `.this` hostnames work at all: every other
component's `http://<name>.this/` URL resolves and routes through it. One
long-running process, `d-man serve`, does two jobs from one routes file: it
keeps a marker-delimited managed block in /etc/hosts mapping every configured
host to 127.0.0.1, and it runs a reverse proxy on 127.0.0.1:80 that picks the
backend port by Host header. A second listener on 127.0.0.1:443 shares the
same handler and mints a per-host certificate from a local CA, so hosts on the
block list reach the block page over https too. d-man has no `.this` address
of its own; when it is down, every `.this` host is down at once.

## how it runs

serve runs as a root launchd daemon, registered once with
`sudo t-man --daemon add`: binding :80/:443 and writing /etc/hosts both need
root, and concentrating them here is why no other component ever does. It
never starts or supervises the backends it routes to; a dead backend gets a
502, and fixing that is t-man's job. Upgrades need no sudo: serve watches its
own binary and exits cleanly when the file changes, and launchd's KeepAlive
relaunches the new build. Logs land in /var/log/d-man/.

## config and reload

The routes file resolves in order: --config, DMAN_CONFIG, {{.StorePath}} when
it exists, then /etc/d-man/routes.toml (the fallback for the root daemon,
whose HOME points nowhere useful). Which routes exist is machine-private
overlay config: a freshly installed service has no route until this machine's
overlay adds one. serve watches the file with fsnotify and reloads on content
change or SIGHUP. Two sharp edges:

- The watch resolves symlinks once at startup and watches the real file's
  directory. Editing that file reloads; repointing the symlink at a different
  file does not. Change the content, or send serve a SIGHUP.
- Top-level keys (blocklist, suffix, games_dir) must come before the first
  [[route]] table. TOML reads every key after a [[route]] header as a field
  of that table, where d-man silently ignores it, so a blocklist at the
  bottom of the file blocks nothing.

A bad edit is logged and ignored: the last good routes and /etc/hosts stay
live, so a typo breaks nothing but also changes nothing until fixed.

## failure modes

Start with `d-man docs` for this doc, then `d-man doctor`: three read-only
checks, one line each. config-loads parses and validates the resolved routes
file; hosts-entries-present confirms the managed /etc/hosts block exists and
names every configured host; proxy-answers fetches d-man's own
/__this/sites.json on 127.0.0.1:80, the one path serve answers itself on any
host. Doctor checks the wiring this machine has, not the wiring it should
have.

One host fails while others work: almost always the route, not the daemon.
`d-man list` shows what is routed; a 502 means the route exists but the
backend is down, so restart the backend, not d-man.

Every host fails at once: serve is not running, or something else holds :80.
A proxy-answers FAIL with connection refused means serve is down; an answer
that is not the sites list means another process holds the port.

A blocked host still loads over https: the block-page CA is not trusted yet.
`sudo d-man ca install` fixes it once. The CA key is root-owned by design; a
non-root process cannot read it.

## version skew

`d-man version` reports the binary on PATH. The running daemon normally
cannot lag it: serve exits when its binary changes and KeepAlive relaunches
the new build. There is no /version endpoint to compare against, so doctor
carries no skew check. If skew is still suspected, kill the serve process;
KeepAlive brings it back on the current binary.

## first moves

1. `d-man docs`, then `d-man doctor`: config parses, hosts block present and
   complete, proxy answering, in one pass.
2. Split the fault: `curl -s http://127.0.0.1:<port>/` against the backend
   directly. Backend answers but `<name>.this` does not: it is d-man.
3. `d-man list` and `d-man config`: does the route exist, and which routes
   file (loaded / missing / parse error) produced it.
4. Check /var/log/d-man/ for "reload failed, keeping current routes/hosts":
   a bad edit is being ignored and the file needs fixing.
