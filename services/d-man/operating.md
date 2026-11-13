# operating d-man

d-man is the layer that makes `.this` hostnames work at all: every other
component's `http://<name>.this/` URL resolves and routes through it. One
long-running process, `d-man serve`, does two jobs from one routes file. It
keeps a marker-delimited managed block in /etc/hosts mapping every configured
host to 127.0.0.1, and runs a reverse proxy on 127.0.0.1:80 that picks the
backend port by Host header. A second listener on 127.0.0.1:443 shares the
same handler. It mints a per-host certificate from a local CA, so hosts on the
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
it exists (XDG_CONFIG_HOME relocates that directory), then
/etc/d-man/routes.toml (the fallback for the root daemon, whose HOME points
nowhere useful). Which routes exist is machine-private
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

A machine that switches block lists generates the routes file instead of
editing it. `d-man profile <name>` writes it from routes.base.toml in the
same directory, with blocklist.<name>.toml above it, and records the name in
a file called profile there. Edit those inputs, never the generated file, and
re-apply with `d-man profile "$(d-man profile)"`. The personal profile needs
no block list; any other name without one fails, and a result that does not
validate is never written.

## ui settings

serve answers GET and PATCH /__this/settings.json on every configured route
host. The document holds the webkit theme, palette, font, size and fixation
choices, so a choice made on one .this host shows on the next. It lives in
ui-settings.json beside the routes file (--settings-file moves it), written
by the root daemon with mode 0644 through a temp file and a rename. A missing
file opens empty. A file that does not parse is logged on stderr and serve
starts with no settings, and the next change overwrites it. A route reload
keeps the settings; only a restart rereads the file.

A PATCH must carry an Origin equal to the page's own scheme and host, so only
the page on that host can change the settings. No answer on this path
carries a CORS header. Values outside what the kit accepts are refused with a
400. The kit syncs only on pages served through d-man under the .this
suffix. A page on a localhost port, or under a custom suffix, keeps its
settings in that origin's own browser storage.

## failure modes

Start with `d-man docs` for this doc, then `d-man doctor`: four read-only
checks, one line each. config-loads parses and validates the resolved routes
file. hosts-entries-present confirms the managed /etc/hosts block exists and
names every configured host. proxy-answers fetches d-man's own
/__this/sites.json on 127.0.0.1:80, the one path serve answers itself on any
host. settings-answers fetches /__this/settings.json on 127.0.0.1:80 as the
first configured route's host and wants a version 1 document. Doctor checks
the wiring this machine has, not the wiring it should have.

One host fails while others work: almost always the route, not the daemon.
`d-man list` shows what is routed; a 502 means the route exists but the
backend is down, so restart the backend, not d-man.

Every host fails at once: serve is not running, or something else holds :80.
A proxy-answers FAIL with connection refused means serve is down; an answer
that is not the sites list means another process holds the port.

The command-K site picker is empty on a page not behind d-man: the picker
falls back to fetching /__this/sites.json cross-origin. d-man answers that
only for this machine's own origins, a page whose Origin is loopback or
ends in .this. Any other origin gets no CORS headers and the browser
drops the response. Open the page through its .this host or its localhost
port; there is no setting that widens this.

A blocked host still loads over https: the block-page CA is not trusted yet.
`sudo d-man ca install` fixes it once. The CA key is root-owned by design; a
non-root process cannot read it.

UI settings do not follow between hosts: run `d-man doctor`. A
settings-answers FAIL with a 404, or with a page that is not a settings
document, means the running serve predates the endpoint and sent the request
to the backend. Kill serve and KeepAlive starts the current binary. A FAIL
with a 502 that says the daemon has no route for the host means serve loaded
another routes file than the one doctor read; `d-man config` names doctor's.
A PATCH that gets a 403 sent an Origin other than the page's own. A 500 means
the save failed; the log names the file, and the settings in memory stay as
they were.

The wrong hosts are blocked after a profile switch: `d-man profile` names the
recorded profile, and `d-man config` shows the blocklist the routes file holds.
A hand edit to a generated routes file is lost at the next switch.

## version skew

`d-man version` reports the binary on PATH. The running daemon normally
cannot lag it: serve exits when its binary changes and KeepAlive relaunches
the new build. There is no /version endpoint to compare against, so doctor
carries no general skew check; settings-answers fails on a serve older than
the settings endpoint. If skew is still suspected, kill the serve process;
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
