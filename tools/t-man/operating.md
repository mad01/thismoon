# operating t-man

t-man is a declarative manager for macOS launchd services and the supervisor
behind the platform's local services. `t-man add` writes a launchd plist and
loads it; launchd itself starts the process and keeps it alive. t-man is a
plain CLI with no daemon of its own, so "t-man supervises X" always means
"launchd runs X from a plist t-man wrote".

## how it runs

Every managed service gets RunAtLoad and KeepAlive set true: launchd starts
it at load and relaunches it whenever it exits. t-man shells out to the
legacy launchctl verbs (load -w, unload, start, stop, list, print) and does
no process supervision or health checking itself. `add` is idempotent: a
SHA256 hash of the full definition is stored inside the plist, an unchanged
re-add is a no-op, and a changed one rewrites the plist and bounces the
service.

## where definitions, state, and logs live

The plists are the definitions and the state: {{.StorePath}}/<name>.plist
for user agents (the default), /Library/LaunchDaemons for --daemon services
(every daemon command needs sudo). There is no manifest and no separate
state file; the hash lives inside the plist under TManMetadata, and t-man
ignores plists that carry neither its metadata nor serviceman's marker.

Service logs default to {{.LogPath}}/stdout.log and stderr.log
(/var/log/<name> for daemons) unless the service was added with --logs DIR.
This is where the output of every t-man supervised service lands.
`{{.Bin}} status <name>` prints the exact paths for one service;
`{{.Bin}} logs <name>` tails both streams, and `--source <src>` reads a
named extra log registered at add time (sandbox denial logs by convention:
`{{.Bin}} logs sandbox`).

## failure modes

A service is down and restart does not fix it: KeepAlive means launchd is
already relaunching it, so a service that stays down is exiting the moment
it starts. Read `{{.Bin}} status <name>` for the exact command, workdir, and
log paths, then `{{.Bin}} logs <name> --stderr` (crash loops print why
there). If the logs are empty, ask launchd for the exit reason:
`launchctl print gui/$(id -u)/<name>` and read the `last exit code` and
`state` fields. Then run the plist's command by hand from its workdir; a
command that works in the shell but not under launchd is almost always
missing environment (launchd starts services with a minimal PATH and none
of the shell profile), fixed with `add --env` / `--path`.

t-man reports the service running but its behavior is stale: launchd keeps
the old process across a rebuild and reinstall, so the binary on disk is new
while the process serving is old. Compare the service's `GET /version` (or
its own `version` command) with the freshly installed binary;
`{{.Bin}} restart <name>` picks up the new one.

A service is missing from `{{.Bin}} list`: registration is machine-private,
done at provisioning time with `t-man add`, so a missing entry means that
add never ran on this machine. Also check the mode: `list` shows user
agents by default and daemons only with `sudo {{.Bin}} --daemon list`, and
it skips plists it does not recognise as its own.

## version skew

`{{.Bin}} version -o json` reports the build of the t-man binary on PATH.
Each managed plist records under TManMetadata.Version the build that last
wrote it; a difference only means the service has not been re-added since a
t-man upgrade, and the next `add` refreshes it. t-man runs no long-lived
process of its own, so there is no serve-side build to compare against.

## first moves when a service is down

1. `{{.Bin}} list` to confirm the service exists and read its STATUS
2. `{{.Bin}} status <name>` for the command, workdir, env, and log paths
3. `{{.Bin}} logs <name> --stderr` (add -f to follow a crash loop live)
4. `{{.Bin}} restart <name>`
5. Verify the running build: the service's `GET /version` (or its own
   `version` command) should report the freshly installed binary
