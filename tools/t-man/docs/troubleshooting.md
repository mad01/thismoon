# Troubleshooting

Most problems with a t-man service are launchd problems wearing a t-man hat.
This page walks through the common ones, starting with t-man's own commands and
dropping down to `launchctl` when you need more detail.

## A service will not stay up

This is the most common failure: you `add` a service, it appears in `list`, but
it keeps restarting or shows as stopped. `KeepAlive` is always on, so launchd
relaunches the process every time it exits. If the process exits immediately,
you get a tight crash loop.

Work through it in this order:

**1. Confirm it exists and check its status.**

```bash
t-man list
t-man status my-service
```

`status` shows the exact command, working directory, environment, and log
paths t-man recorded. Check that the command path is right and the working
directory exists.

**2. Read stderr.** A crash loop almost always prints why on stderr:

```bash
t-man logs my-service --stderr
```

Common causes you will see here: the binary is missing or not executable, a
required env var is unset, the working directory does not exist, or the port is
already in use.

**3. Follow the logs while it crashes** so you catch the restart in real time:

```bash
t-man logs my-service -f
```

If the logs are empty, the process may be dying before it writes anything (for
example, a bad interpreter path or a missing shared library). Drop to
launchctl in that case.

**4. Ask launchd directly.** launchctl knows things t-man does not surface,
including the last exit code and the reason it was killed:

```bash
# Agent
launchctl print gui/$(id -u)/my-service

# Daemon
sudo launchctl print system/my-daemon
```

Look at the `last exit code` and `state` fields. A non-zero exit code points at
the program itself; `state = spawn scheduled` flapping means the crash loop is
active.

**5. Check the command by hand.** Run exactly what the plist runs, from the
working directory it sets, with the same environment:

```bash
cat ~/Library/LaunchAgents/my-service.plist   # read ProgramArguments + env
cd /the/workdir
/the/command --the --args
```

If it fails in your shell too, the problem is the command, not t-man. If it
works in your shell but not under launchd, the difference is almost always the
environment: launchd starts with a minimal `PATH` and none of your shell
profile. Add what the service needs with `--env` and `--path` and re-run `add`.

## Logs are empty

`t-man logs` reads the `stdout.log` and `stderr.log` files in the service's log
directory. If they are empty:

- Confirm the service is actually running: `t-man status my-service`.
- Confirm the log directory exists: `ls ~/Library/Logs/my-service/`.
- Some programs buffer stdout when it is not a terminal. If the program has an
  unbuffered or line-buffered flag, use it, or log to stderr.

## A change to the config did nothing

If you edited the service and `add` printed "already up to date," the hash did
not change. t-man hashes the full definition, so a change only registers if it
alters a field t-man tracks (command, args, env, workdir, log paths, sandbox
profile, extra logs). Re-check the flags you passed. Running with `--dryrun`
shows what t-man would do without applying it:

```bash
t-man --dryrun add --name my-service --env NEW=value -- /usr/local/bin/my-service
```

## A sandboxed service is failing

If a service launched with `--sandbox-profile` dies or misbehaves, the seatbelt
profile is probably denying something it needs (a file read, a network
connection, a process spawn). Register the denial log at `add` time with
`--extra-log` and read it:

```bash
t-man logs my-service --source sandbox
# or, across every sandboxed service:
t-man logs sandbox
t-man logs sandbox -f          # follow new denials live
```

Each denial names the operation and the path or resource that was blocked. Add
the corresponding allow rule to the `.sb` profile, then re-run `add` — editing
the profile changes the hash, so t-man re-renders the plist and bounces the
service automatically.

## Editing the plist directly does not stick

Do not hand-edit the generated plist. The next `add` rewrites it from the
definition, so your edits disappear, and in the meantime t-man's stored hash no
longer matches what is on disk. Always change a service through `add` with new
flags.

## Removing a stuck service cleanly

If a service is wedged and you want a clean slate:

```bash
t-man remove my-service       # unloads the plist and deletes the file
```

If launchd still has a stale registration after that (rare), unload it by hand:

```bash
launchctl unload ~/Library/LaunchAgents/my-service.plist 2>/dev/null
launchctl bootout gui/$(id -u)/my-service 2>/dev/null
```

Then re-add the service.

## Permission denied on a daemon

Every `--daemon` command needs root. If you get `system daemon mode requires
sudo/root privileges`, prefix the command with `sudo`:

```bash
sudo t-man --daemon status my-daemon
sudo t-man --daemon restart my-daemon
```
