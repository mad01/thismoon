# Agents and daemons

t-man manages two kinds of launchd services. The difference comes down to who
owns the service and where its plist lives. You pick one with a global flag,
and that choice decides the plist directory, the log directory, and whether you
need root.

## The two modes

| | Agent (default) | Daemon (`--daemon`) |
|---|---|---|
| launchd type | LaunchAgent | LaunchDaemon |
| Plist directory | `~/Library/LaunchAgents/` | `/Library/LaunchDaemons/` |
| Default log directory | `~/Library/Logs/<name>/` | `/var/log/<name>/` |
| Runs as | your user | root |
| Needs sudo | no | yes |
| Lifecycle | runs while you are logged in | runs at boot, before login |

`--agent` is the default and is implied if you pass nothing. `--daemon` and
`--agent` are mutually exclusive.

## When to use which

Use an **agent** for anything that only needs to run while you are logged in
and only needs your user's permissions. Most local services fit here: a web
server on a high port, a background worker, a TTS engine. Agents are the safe
default because they never need root.

Use a **daemon** only when the service must run before you log in, or must bind
a privileged port (below 1024), or must edit root-owned files. A reverse proxy
that listens on port 80 and rewrites `/etc/hosts` is the classic case — it
cannot do its job as your user, so it has to be a root LaunchDaemon.

If you are not sure, start with an agent. You can remove it and re-add as a
daemon later.

## Daemon mode needs root for every command

`--daemon` is guarded by a root check, and the check runs on every command, not
just `add`. `list`, `status`, `logs`, `start`, `stop`, `restart`, and `remove`
all require `sudo` when you pass `--daemon`, because the plists they read and
the services they control live under `/Library/LaunchDaemons` and run as root.

```bash
sudo t-man --daemon list
sudo t-man --daemon status my-daemon
sudo t-man --daemon restart my-daemon
```

Without `sudo`, t-man exits with `system daemon mode requires sudo/root
privileges` rather than failing partway through.

## The one-time daemon setup

Registering a daemon is a single privileged `add`. It is the one place where a
service needs root, and you do it once per machine:

```bash
sudo t-man --daemon add --name my-daemon \
  --desc "Root service that runs at boot" \
  -- /usr/local/bin/my-daemon --flag value
```

This writes `/Library/LaunchDaemons/my-daemon.plist`, creates the log directory
under `/var/log/my-daemon/`, and loads the service as root. After that, the
daemon comes back on its own at every boot — you do not repeat this step. You
only run `sudo t-man --daemon add` again if you change its configuration, and
then the hash check means it only reloads if something actually changed.

## The `--port` convention

t-man does not have a `--port` flag, and it does not care what ports your
services listen on. But there is a convention worth knowing, because the
surrounding tooling depends on it.

When a service command takes `--port N` as one of its own arguments, that
argument ends up in the plist's `ProgramArguments`:

```bash
t-man add --name my-api -- /usr/local/bin/my-api serve --port 7777
#                                                        ^^^^^^^^^^^
#                          this is an argument to my-api, not to t-man
```

A status page that watches t-man services reads each plist's
`ProgramArguments`. If it finds `--port`, it HTTP-probes the service on that
port to decide whether it is healthy. If there is no `--port`, it falls back to
checking that launchd has a live PID for the service.

So the practical rule is: if you want a service to be health-checked over HTTP,
pass it a `--port N` argument and have it actually listen there. If it is not an
HTTP service, leave `--port` off and it is monitored by PID instead. Either way
this is a convention the consumers agree on, not behavior t-man enforces.

## Inspecting what t-man wrote

The plist t-man generates is plain XML. You can read it directly:

```bash
# Agent
cat ~/Library/LaunchAgents/my-service.plist

# Daemon
sudo cat /Library/LaunchDaemons/my-daemon.plist
```

You will see the `ProgramArguments`, the log paths, `RunAtLoad`, `KeepAlive`,
and the `TManMetadata` dict with the stored hash. Do not hand-edit these files
— the marker comment says as much, and the next `add` would overwrite your
changes anyway. Change the service by re-running `add` with new flags.
