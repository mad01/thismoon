# why t-man

## The problem

Every service on the platform runs under launchd, and raw launchd management
is where a single developer machine rots: hand-written plist XML nobody
re-reads, `launchctl load`/`unload` sequences run from memory, and no way to
tell whether re-running a setup script will needlessly bounce a service that
did not change. The fleet path makes this acute: ralph re-runs recipes on
every `ralph up`, so registering a service has to be a safe no-op when
nothing changed and a clean reload when something did.

## Why its own tool

launchctl is imperative and stateless — it applies whatever you tell it,
with no notion of "already up to date". serviceman, the prior art t-man credits
and stays CLI-compatible with, lacks the change detection: t-man exists to
add true idempotency (hash-based comparison, read-compare-apply) on top of
the same one-line `add` interface. It is not folded into ralph because it is
a platform foundation the whole recipe layer stands on: recipes install and
restart services through t-man-guarded hooks, and docs/adr/0006 allows hard
`depends_on` across recipe sources only on the foundations t-man and d-man.
A standalone tool keeps that interface identical for recipes, scripts, and a
human at the terminal.

Sandboxing is the other reason t-man is its own tool. Every alternative
(launchctl, serviceman, `brew services`) runs the service binary bare;
none can confine what it launches. t-man's `--sandbox-profile` renders the
plist to start the command through `/usr/bin/sandbox-exec` with a seatbelt
profile, and the profile's content feeds the idempotency hash, so editing
the `.sb` file reconciles like any other definition change. Apple ships no
replacement for wrapping an unmodified binary this way (App Sandbox is
opt-in at code-signing time), which makes the service manager the one place
the wrapper can live — and a reason to pick t-man even where `brew services`
would otherwise do.

## Why this shape

Idempotency lives in the plist itself: a SHA256 hash of the full service
definition is stored under `TManMetadata` in the generated plist, so there
is no separate state file and the state cannot desync from the artifact it
describes. Every `add` runs read-compare-apply — read the existing plist
back into a definition, compare hashes, and only on a mismatch write, reload,
and (if the reload fails) restore the old plist. There is no manifest
format: t-man builds the definition entirely from `add` flags, so callers
such as ralph recipes own their desired state and t-man stays a reconciler rather
than another config surface. And t-man only touches plists it recognises,
its own metadata or serviceman's marker, so it coexists with everything else
in `~/Library/LaunchAgents` and `/Library/LaunchDaemons`.

## Non-goals

t-man is launchd-only, matching a platform that is macOS through and through
(docs/adr/0007); there is no cross-platform abstraction waiting behind the
Manager interface. It does no process supervision of its own: `RunAtLoad`
and `KeepAlive` are set true and launchd does the restarting. It does no
health checking: the `--port` probe convention belongs to the surrounding
tooling (the status service reads `ProgramArguments` from the plist), not to
t-man, which neither parses nor enforces it. And it does not adopt or manage
unrelated plists it finds; anything without its metadata is left alone.
