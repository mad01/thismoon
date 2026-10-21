# Sandboxing third-party code with seatbelt

Some processes in the fleet run third-party package code: a PyPI engine, a
go-installed binary, a Model Context Protocol (MCP) server. Each one gets
confined with seatbelt (`sandbox-exec`), so a poisoned dependency can't read
secrets or exfiltrate data. This page is the general guide: when seatbelt is the right
tool, the profile patterns that work, how profiles stay reconciled through
ralph, and how to triage denials.

The profiles split along the recipe layering in
[docs/adr/0006](adr/0006-recipe-layering-and-platform-deps.md). A profile
for a component this repo ships can live beside its recipe here. A profile
for a third-party engine or server, and every wrapper that resolves secrets,
lives in the consuming repo's companion recipe. Reference implementations, by
shape:

| Shape | Example | Lives in |
|---|---|---|
| Egress-shaped with a read-write store and a child exec | present MCP | [`recipes/present/present.sb`](../recipes/present/present.sb) and [`present-mcp-sandbox.sh`](../recipes/present/present-mcp-sandbox.sh): outbound `:443` and DNS for `present_share`, plus the local events port |
| Egress-zero, no writes (template) | a sandboxed MCP server | [`examples/dotfiles/recipes/mcp-sandbox/`](../examples/dotfiles/recipes/mcp-sandbox/): profile, wrapper, and the agent-sync drop-in that registers it |
| Egress-zero with a cache (tightest) | humanizer MCP | the consuming repo's humanizer companion recipe |
| Egress-zero with an inbound port and the GPU | speak-tts engine | the consuming repo's speak-tts recipe (see [`recipes/speak/CLAUDE.md`](../recipes/speak/CLAUDE.md)) |
| Egress-shaped (outbound `:443` and DNS) | GitHub MCP servers | the consuming repo's MCP registration recipe |
| Install-time (network open, env scrubbed) | speak engine install | the consuming repo's speak-tts recipe |

## When to reach for seatbelt

Seatbelt confines a process *on the host*. Use it when the code has to run
there:

- Native runtimes that need host hardware. The speak-tts engine needs
  Metal for MLX GPU compute, which a Linux VM can't provide. Seatbelt keeps
  GPU access while cutting network and `$HOME`.
- Long-lived macOS services (launchd agents via t-man) running
  third-party package code.
- MCP servers, which the MCP client spawns on the host and which speak
  over stdio.

For *build steps* that run third-party install code but don't need host
hardware (npm, pip, gem), a throwaway VM is the better tool. webkit's
`make build` is the reference: it runs `npm ci` inside an Apple container VM
and copies only the two bundle files out (see
[`webkit/CLAUDE.md`](../webkit/CLAUDE.md)).

What seatbelt can and can't do, in short: filesystem and network rules are
solid. It can't filter by hostname, so "HTTPS to GitHub only" floors out at
"outbound `:443` to anywhere". It can't filter environment variables
either, so the spawn wrapper must keep secrets out of the process env; the
profile can't.

## Profile patterns

All profiles in the fleet share one skeleton. Start from the closest worked
example above rather than from scratch.

### `(allow default)` baseline

Deny-everything baselines break dyld, system
frameworks, Metal, and trustd in opaque ways. Allow by default, then deny the
two classes that matter (network and filesystem) and re-allow narrowly.
System paths (`/System`, `/usr/lib`) stay readable; they hold no user
secrets.

### Network: deny, then shape

```scheme
(deny network*)
(allow system-socket)              ;; socket creation is separate from connect/bind
;; egress-zero: stop here.
;; egress-shaped (GitHub servers): outbound :443 and DNS only.
(allow network-outbound
  (literal "/private/var/run/mDNSResponder")   ;; libinfo DNS path
  (remote tcp "*:443")
  (remote udp "*:53")
  (remote tcp "*:53"))
;; inbound service (speak-tts): bind and inbound on the one port.
(allow network-bind    (local ip "*:8765"))    ;; "*:" and not "localhost:":
(allow network-inbound (local ip "*:8765"))    ;; "localhost" misses ::1
```

Egress-zero is the goal state. It's reachable whenever the process can be
made offline: speak-tts gets there by pre-fetching its model at install time
(`HF_HUB_OFFLINE=1` at runtime).

### Writes: deny, then allow the state dirs, temp, and `/dev/null`

```scheme
(deny file-write*)
(allow file-write* (literal "/dev/null"))      ;; see gotcha below
(allow file-write*
  (subpath (string-append (param "HOME") "/.config/present"))
  (subpath "/private/tmp")
  (subpath "/private/var/folders"))            ;; per-user temp, shader caches
```

The `/dev/null` literal is load-bearing: `(deny file-write*)` also blocks
shell `>/dev/null` redirects, and bash silently *skips the command* when the
redirect open fails, so warm-ups no-op without erroring.

### `$HOME` reads: default-deny with an allow-list

A secrets blocklist can
never be complete; deny the lot and allow what the process needs (typically
`~/code/bin` for the binary plus its own state dir).

```scheme
(deny file-read* (subpath (param "HOME")))
(allow file-read*
  (subpath (string-append (param "HOME") "/code/bin")))
```

### Parent-spine metadata

`realpath()` and exec lstat every parent component
of an allowed path, and `subpath` allows don't cover parents. Allow
metadata-only reads on the spine:

```scheme
(allow file-read-metadata
  (literal (param "HOME"))
  (literal (string-append (param "HOME") "/code")))
```

### `HOME` arrives as a param

The wrapper passes `-D HOME="$HOME"`, and t-man supplies it for
`--sandbox-profile` services, so profiles stay machine-portable.

## Spawn wrappers (MCP servers)

An MCP server can't be sandboxed from its registration entry alone.
agent-sync expands only a leading `~` in `command` and args, so there is no
`-D HOME=$HOME` param. The client also spawns servers from the session cwd,
which may sit inside a read-denied subtree. `getcwd()` there fails with
EPERM (operation not permitted), which kills the process before main. So the registered `command` is a wrapper
script that:

1. Runs `cd /` to leave any read-denied cwd.
2. Resolves secrets *before* `sandbox-exec`. A GitHub MCP wrapper, for
   example, runs `gh auth token` outside the sandbox, so the keychain and
   `~/.config/gh` stay unreachable inside it.
3. Launches through `env -i` with an explicit allow-list (`HOME` plus
   whatever the server actually needs). Seatbelt can't filter env; the
   wrapper is the only place to keep shell secrets out.
4. Execs `sandbox-exec -D HOME="$HOME" -f <profile> <binary> ...`.

[`recipes/present/present-mcp-sandbox.sh`](../recipes/present/present-mcp-sandbox.sh)
is the full version, with config passed through and secrets read from the
secrets file. [`examples/dotfiles/recipes/mcp-sandbox/`](../examples/dotfiles/recipes/mcp-sandbox/)
is the minimal one, with the registration as an agent-sync drop-in. Profiles
are read at spawn time: after editing a `.sb` or a wrapper, restart the
server (a new agent session, or `/mcp` reconnect in Claude Code).

## t-man services

Long-lived services get the profile from t-man itself
([`tools/t-man/config.md`](../tools/t-man/config.md) has the flag reference):

```bash
t-man add --name speak-tts --workdir $HOME/.local/share/speak \
  --sandbox-profile $HOME/path/to/speak-tts.sb -- <command>
```

- The profile path *and a content SHA-256* feed t-man's idempotency hash, so
  editing the `.sb` and re-running `ralph up` reconciles and bounces the
  service.
- Set `--workdir` inside a read-allowed subtree (same `getcwd()` EPERM as
  above; launchd defaults cwd to `/`, but don't rely on it).
- Never pass secrets via `--env`: the plist env is exactly what the
  sandboxed process sees.

## Install-time sandboxing

Install time is when a poisoned package's setup code first runs, so installs
get their own profile. It keeps the same filesystem posture and opens the
network (the package index and model hosts). The install launches through
`env -i` with an allow-list, so shell secrets never enter it. The speak-tts engine's
install script and its install profile in the consuming repo are the worked
example. Pre-fetch whatever the runtime needs (models, packs) here; that is
what lets the runtime profile deny all egress.

## Everything through ralph: the persistence checklist

`ralph up` reconciles every sandbox artifact; nothing is manual-only. For a
new sandboxed target, confirm:

- The profile (`.sb`) and wrapper live in the recipe dir, referenced only by
  `~`/`$HOME`-portable paths.
- A recipe chmod hook (`run = "always"`, idempotent) keeps the wrapper
  executable.
- MCP registration flows from the consuming repo's agent-sync server set
  (the base `servers.json` or a `servers.d/` drop-in) through
  `agent-sync mcp sync`, never a hand-run `claude mcp add`.
- A `run = "always"` recipe hook registers each t-man service that takes a
  `--sandbox-profile`, never a one-off `t-man add`.
- The process name is in `SANDBOX_WATCH_PROCESSES` in
  [`recipes/speak/recipe.toml`](../recipes/speak/recipe.toml). One watcher
  covers the whole fleet.
- t-man services with a `--sandbox-profile` also get
  `--extra-log sandbox=$HOME/.local/share/speak/logs/sandbox-denials.log`,
  so `t-man logs <name> --source sandbox` finds the denial ledger.
- Acid test: a fresh `ralph up` on a clean machine reproduces the posture.
  Known gaps live outside ralph: t-man and uv are guarded skips (hooks no-op
  loudly without them), and Apple's `container` runtime is a manual pkg
  install.

## Observability: best-effort, not an audit trail

Enforcement is solid: the access *is* blocked. Telemetry is lossy:

- Denials only stream reliably *live*, with the predicate
  `eventMessage CONTAINS "deny(1)"` at DEFAULT level. `sender == "Sandbox"`
  matches nothing, and `--level debug` drops the events.
- The kernel rate-limits and dedupes repeated identical denials, and
  `log show` after the fact is unreliable for them. Persistent denials from
  long-lived services (the real threat) surface; one-shot probes can be
  missed. A guaranteed ledger would need Apple's Endpoint Security framework.

Two taps exist:

- `sandbox-watch` (a t-man agent, registered by the
  [speak recipe](../recipes/speak/CLAUDE.md)) watches the live stream for the
  fleet's process names. It batches per 60s window, raises a Notification
  Center banner per batch, and appends every denial to
  `~/.local/share/speak/logs/sandbox-denials.log` tagged `ALERT` or
  `IGNORED`. Each banner is mirrored to `sandbox-notifications.log` (same
  dir): a `NOTIFIED` header plus the batch's unique denials. Banners can't
  be copied from Notification Center, so when one fires, read this file to
  see exactly what triggered it. Both files are registered as t-man extra
  logs under the sandbox naming convention (`sandbox`, `sandbox-*`):
  `t-man logs sandbox` shows the whole fleet's sandbox logs in one place,
  and `t-man logs sandbox speak-tts` scopes to one service. Sandboxed t-man
  services carry `--extra-log sandbox=<denials log>` so the scoped view
  works too.
- [`sandbox-audit.sh`](../recipes/speak/sandbox-audit.sh) (`stream` /
  `show [24h]`) is the manual tap for working a specific session. Prefer
  `stream` in one terminal while exercising the service; `show` inherits the
  after-the-fact unreliability above.

## Triaging a denial

A denial line reads `process(pid) · operation · target`, e.g.
`python3.14(123) · file-read-data · /Users/x/.aws/credentials`. Three
outcomes:

1. **Real breakage.** The tool's actual function fails (synthesis errors, a
   tool call returns a sandbox error) and the denied path is something it
   legitimately needs. Add the *minimal* allow to the profile (a `subpath`
   or `literal`, not a parent directory) and apply it: `ralph up` bounces
   t-man services via the content hash, and MCP servers need a respawn.
   Confirm the function works. Never add allows preemptively.
2. **Expected or benign noise.** The tool works fine; the denial is
   discovery probing. Dotenv loaders, AWS/GCP SDK credential chains, and
   `~/.netrc` lookups all scan every candidate path they never use. The
   block is correct; the banner is noise. Add a tight extended-regex (ERE)
   line to `~/.config/sandbox-watch/ignore.conf`, with a comment saying *why
   it's benign*. Fleet-wide defaults ship in
   [`recipes/speak/sandbox-ignore.conf`](../recipes/speak/sandbox-ignore.conf);
   add those through a commit to this repo. A matched denial keeps logging
   as `IGNORED` but stops notifying. `(with no-report)` on the profile rule
   is the heavier variant: it stops the denial reaching the log entirely, so
   reserve it for denials so frequent they flood the stream.
3. **Compromise indicator.** An unexpected read of a secrets path
   (`~/.ssh`, keychains), or any network attempt from an egress-zero
   process, especially right after a pin bump or dependency change. Don't
   allow or silence it. Diff the venv or module graph against the previous
   pins, find what introduced the access, and treat it as a potential
   supply-chain compromise until explained.

Rule of thumb: never silence or allow a denial you can't explain. The
safe default is to leave it alerting until you understand it. A silenced
credential read you didn't expect is exactly what an attacker wants ignored.

### Mapping a denial back to the profile

The operation names the rule class: `file-read-data` maps to the
`file-read*` allows, `file-write-create` to `file-write*`, and
`network-outbound` to the network block. The target tells you which
`subpath` or `literal` would cover it. If the target is under an allow
you expected to match, check the parent spine: missing `file-read-metadata`
on a parent component shows up as denials on the *parent*, not the leaf.

### After every pin bump

Run the real workload with
`sandbox-audit.sh stream` open (or check `sandbox-denials.log` after) and
explain anything new before moving on.

### Ratcheting tighter

The reverse loop also works. Flip a suspect allow to
a deny in a staging copy of the profile, run the real workloads, and keep the
deny if the stream stays quiet.

## Known gotchas, collected

- `getcwd()` EPERMs when cwd is inside a read-denied subtree: `cd /` in
  wrappers, `--workdir` on services.
- `(deny file-write*)` silently breaks `>/dev/null` redirects: allow the
  literal.
- In a seatbelt profile, `localhost` doesn't reliably match `::1`, so bind
  allows use `*:port` (the app itself should bind loopback).
- Hand-rolled JSON-RPC probes against a stdio MCP server must hold stdin
  open (sleep between frames). "server is closing: EOF" is the probe's
  fault, not a sandbox failure.
- `present_open`-style child execs of `/usr/bin/open` work under the
  seatbelt: `open` only sends a LaunchServices mach message, and the browser
  launches via launchd *outside* the sandbox.
- Notification banners from launchd agents are attributed to "Script
  Editor". Allow them once in System Settings → Notifications, or they
  silently never appear.
