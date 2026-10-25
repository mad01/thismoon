# Sandboxing third-party code with srt

Some processes in the fleet run third-party package code: a PyPI engine, a
go-installed binary, a Model Context Protocol (MCP) server. Each one runs
under [anthropics/sandbox-runtime](https://github.com/anthropics/sandbox-runtime)
(srt), so a poisoned dependency can't read secrets or exfiltrate data. This
page covers why srt, the launch pattern, the settings file, how it stays
reconciled through ralph, and how to triage denials.

## Why srt

srt replaced hand-written seatbelt profiles (`*.sb` files run with
`sandbox-exec`). On macOS it still enforces through seatbelt: it generates a
profile from a JSON settings file and runs the command under it. What
changed:

- Egress by hostname. srt routes network traffic through local HTTP and
  SOCKS proxies that check `network.allowedDomains`. A raw seatbelt profile
  can only filter by port, so "HTTPS to GitHub" used to floor out at
  "outbound `:443` to anywhere".
- Generated profiles. The settings file says what to allow. srt writes
  the profile language, including the system baseline that dyld and the
  system frameworks need.
- One JSON file per process. Each sandboxed process gets one settings
  file. There is no shared profile to keep in step.
- Maintained upstream. srt is the sandbox Claude Code uses for its own
  shell, so a macOS change that breaks it gets fixed there, not in our
  profiles.

The srt CLI comes from mise (`npm:@anthropic-ai/sandbox-runtime`, declared
in the consuming repo's mise config).

## The pattern

Three pieces, split along the recipe layering in
[docs/adr/0006](adr/0006-recipe-layering-and-platform-deps.md):

1. `srt-run`, shipped here by the speak recipe as `~/code/bin/srt-run`
   ([`recipes/speak/srt-run.sh`](../recipes/speak/srt-run.sh)). Usage:
   `srt-run <settings-file> <argv...>`. It expands a leading `~` in the
   settings path, finds srt through the mise shims dir, sets the child's
   `TMPDIR`, and runs `srt --settings <abs> -- env SRT_RUN=<label> <argv...>`.
   The label is the settings file's name (`~/.config/srt/present.json` gives
   `present`); `SRT_RUN_LABEL` overrides it.
2. The settings file, `~/.config/srt/<label>.json`. A file that is the same
   on every machine ships beside its service's recipe here. One that
   differs per machine ships from the consuming repo as a machine-private
   overlay ([docs/adr/0006](adr/0006-recipe-layering-and-platform-deps.md)).
   srt fails closed: a missing or invalid file exits 1 and the command
   never runs.
3. A wrapper script that the MCP registration or t-man service runs. It
   resolves secrets outside the sandbox, runs `cd /`, scrubs the environment
   with `env -i` and an allow-list, and execs `srt-run`.

The wrapper exists because neither an MCP registration nor a launchd plist
can express the whole launch:

- The MCP client spawns servers from the session cwd, which may sit inside
  a read-denied subtree. `getcwd()` there fails with EPERM (operation not
  permitted) before `main` runs, so the wrapper does `cd /` first.
- The sandbox can't filter environment variables, so the wrapper is the
  only place to keep shell secrets out. It resolves secrets before the
  launch: a GitHub MCP wrapper runs `gh auth token` outside the sandbox, so
  the keychain stays unreachable inside it. Then `env -i` passes only what
  the process needs.

[`recipes/present/present-mcp-sandbox.sh`](../recipes/present/present-mcp-sandbox.sh)
is the full version, with config and sharing secrets passed through.
[`examples/dotfiles/recipes/mcp-sandbox/`](../examples/dotfiles/recipes/mcp-sandbox/)
is the minimal one, with the settings file, the wrapper, and the agent-sync
drop-in that registers it.

t-man services run `srt-run` as their program, with or without a wrapper
in front of it. See [Sandboxing a t-man service](#sandboxing-a-t-man-service).

### Settings live outside every allowWrite

A settings file must never sit where a sandbox can write, its own or any
other. present may write its page store under `~/.config/present`; a policy
file there could be rewritten by the process it confines, and it would load
on the next launch. A symlinked policy is no safer in a writable directory,
since the link itself can be swapped. So every settings file lives in
`~/.config/srt/`, which no fleet sandbox may write, and so does the link
target if the consuming repo symlinks it from a checkout.

### Machine-specific hosts

The settings file holds the static policy. A host that differs per machine,
such as present's shared instance or an LLM proxy, comes from the wrapper's
own inputs at launch instead. The wrapper exports `SRT_RUN_ALLOW_DOMAINS`
(comma-separated `host[:port]`), and `srt-run` merges it into
`network.allowedDomains` with jq. The merged copy goes to
`~/.config/srt/merged/<label>.json` (mode 600, rewritten every launch) and
srt reads that. With the variable set, a missing jq or a failed merge stops
the launch. present's wrapper derives the host from `PRESENT_SHARED_URL`, so
its static file keeps `allowedDomains` empty. Never use `*`: srt 0.0.81
rejects a bare `*` or `*:443`, and `srt-run` refuses one in the variable.

### The legacy path: a seatbelt profile under t-man

srt's settings have no key for some sandbox classes. speak-tts needs
`iokit-open` to reach Metal for its MLX engine, so it keeps a hand-written
seatbelt profile run by t-man's `--sandbox-profile` flag
([`tools/t-man/config.md`](../tools/t-man/config.md)). Use this path only
when srt can't express what the service needs. Tag every deny rule in the
profile with `(with message "SRT_RUN=<label>")`. The log then appends the
marker to each denial, and `sandbox-watch` attributes it like an srt
process. t-man hashes the profile's content, so editing it and re-running
`ralph up` re-renders the plist and restarts the service.

## A worked settings file

The present MCP server's settings, as the consuming repo would ship them
at `~/.config/srt/present.json`:

```json
{
  "network": {
    "allowedDomains": [],
    "deniedDomains": [],
    "allowLocalBinding": true
  },
  "filesystem": {
    "denyRead": ["~"],
    "allowRead": ["~/code/bin", "~/.config/present", "~/.local/state/present"],
    "allowWrite": [
      "~/.config/present",
      "~/.local/state/present",
      "/private/tmp",
      "/private/var/folders"
    ],
    "denyWrite": []
  }
}
```

`allowedDomains` stays empty because the shared host arrives at launch (see
Machine-specific hosts above). The two temp entries are there because
`srt-run` points the child's `TMPDIR` at the caller's temp dir or
`/private/tmp`, and srt doesn't allow that dir on its own.

srt requires all four `filesystem` lists and both domain lists, even when
empty. `~` expands inside the file. A `_comment` key is accepted, which is
the only way to annotate JSON. The keys that come up most:

| Key | Effect |
|---|---|
| `network.allowedDomains` | Hosts the proxies let through, as `host` or `host:port`, wildcards allowed. Empty means no network. |
| `network.allowLocalBinding` | Allows binding local ports and connecting to localhost, which the fire-and-forget POST to events on `127.0.0.1:7430` needs. |
| `network.allowMachLookup` | Extra mach services to reach. A trailing `*` matches a prefix. |
| `filesystem.denyRead` / `allowRead` | Reads are allowed except under `denyRead`; `allowRead` carves exceptions back out. The most specific rule wins. Globs work. |
| `filesystem.allowWrite` / `denyWrite` | Writes are denied except under `allowWrite`, minus `denyWrite`. |
| `enableWeakerNetworkIsolation` | Adds `com.apple.trustd.agent`, which Go binaries need for TLS certificate checks. |
| `allowAppleEvents` | Lets the process send Apple Events. Off by default. |
| `allowPty` | Lets the process open a pseudo-terminal. |

Settings are read at spawn time. After editing one, restart the process: a
new agent session or `/mcp` reconnect for an MCP server, `t-man restart` for
a service.

## Reads policy

Deny `~`, then allow what the process needs. A secrets blocklist can never
be complete, so the default is the whole home directory off. A typical
allow-list is `~/code/bin` for the binary plus the process's own state
directory.

A process that reads source (humanizer scanning docs, say) gets the code
roots back in `allowRead`. Secret files live inside those roots too, so
add nested denies under them:

```json
"denyRead": [
  "~",
  "~/code/**/.env",
  "~/code/**/.env.*",
  "~/code/**/*.pem",
  "~/code/**/*.key"
],
"allowRead": ["~/code"]
```

The nested denies are more specific than the `~/code` allow, so they win.
Anchor them under `~/`: a bare `**/.env` resolves against the working
directory, and the wrapper's `cd /` makes that a different tree from the
one you meant.

## Everything through ralph

`ralph up` reconciles every sandbox artifact; nothing is manual-only. For a
new sandboxed process, confirm:

- The settings file lives in the recipe dir that links it to
  `~/.config/srt/<label>.json`. That is the service's recipe here when the
  file is the same on every machine, and the consuming repo's otherwise.
- Neither `~/.config/srt` nor the link's target is under any settings
  file's `allowWrite`. This prints every entry that covers the directory,
  and should print nothing:
  `jq -r '.filesystem.allowWrite[]' ~/.config/srt/*.json | grep -xE '~/?|~/\.config(/srt.*)?/?'`
- The wrapper lives beside it, referenced only by `~`/`$HOME`-portable
  paths, and a chmod hook (`run = "always"`, idempotent) keeps it
  executable.
- MCP registration flows from the consuming repo's agent-sync server set
  (the base `servers.json` or a `servers.d/` drop-in) through
  `agent-sync mcp sync`, never a hand-run `claude mcp add`.
- A `run = "always"` recipe hook registers each t-man service with
  `srt-run`, or a wrapper that execs it, as its program, never a one-off
  `t-man add`. The service also gets
  `--extra-log sandbox=$HOME/.local/share/speak/logs/sandbox-denials.log`,
  so `t-man logs <name> --source sandbox` finds the denial ledger.
- Nothing to add to `sandbox-watch`. It attributes denials by the
  `SRT_RUN` label, so a new process is covered the moment it launches
  through `srt-run`. A legacy seatbelt profile is covered once every deny
  rule carries the marker.
- Acid test: a fresh `ralph up` on a clean machine reproduces the posture.

## Sandboxing a t-man service

A first-party service under t-man runs through the same launcher. The
registration names `srt-run` as the program, then the settings path, then
the original command:

```bash
t-man add --name events \
  --extra-log sandbox=$HOME/.local/share/speak/logs/sandbox-denials.log \
  -- $HOME/code/bin/srt-run $HOME/.config/srt/events.json \
  $HOME/code/bin/events serve --port 7430 --workdir $HOME/.local/share/events
```

events, keeper-of-facts, moondb, catalog-web and present serve run this
way. What carries over to the next one:

- **The label is the service name.** The settings file's name gives the
  label, and it matches the t-man name. present serve is the exception: its
  label is `present-serve`, because the MCP server already uses `present`.
- **Generic settings ship beside the recipe.** When nothing in them differs
  per machine, they live in this repo as `recipes/<svc>/<label>.srt.json`,
  linked to `~/.config/srt/<label>.json` by a `[dotfiles.<svc>_srt_settings]`
  item. `recipes/speak/srt-settings-test.sh`, part of `make test-recipes`,
  checks every shipped file and its wiring.
- **A wrapper only when there is something to resolve.** launchd starts an
  agent in `/` with a small environment, so the cwd and env reasons for an
  MCP wrapper don't apply. A service that needs a secret or a
  machine-specific host keeps a launcher that resolves them outside the
  sandbox and then execs `srt-run`. `recipes/present/present-serve.sh` reads
  the sharing config and hands the shared host to `srt-run` that way.
- **Ordering.** ralph links every dotfiles item before any build or
  package runs, so the settings link and `~/code/bin/srt-run` exist before
  the registration hook. ralph's `depends_on` accepts only `builds.` and
  `packages.` keys, so the hook depends on `builds.speak_chmod_scripts`,
  which keeps `srt-run.sh` executable.
- **Start minimal.** `allowedDomains: []`, `allowLocalBinding: true` (the
  listening port, and loopback posts to events), `denyRead: ["~"]`,
  `allowRead` for `~/code/bin` and the workdir, `allowWrite` for the workdir
  plus `/private/tmp` and `/private/var/folders`. Add only what a probe
  shows the service needs.

Three services stay outside srt: csl-web, sandbox-watch and d-man. The ADR
amendment in [docs/adr/0006](adr/0006-recipe-layering-and-platform-deps.md)
records why.

### git inside the sandbox

A service that runs git under `denyRead: ["~"]` needs two things:

- **A global config git can skip.** git dies on a read-denied
  `~/.gitconfig` (`fatal: unable to access '~/.gitconfig': Operation not
  permitted`). keeper-of-facts and moondb only run read-only commands
  (`rev-parse`, `blame`), which need no identity or credential helper, so
  they get `--env GIT_CONFIG_GLOBAL=/dev/null`. The repo's own `.git/config`
  has to stay readable, because git reads it on every command.
- **A readable developer directory.** `srt-run` puts `/usr/bin` ahead of the
  caller's `PATH`, so git resolves to `/usr/bin/git`, the xcrun shim. A
  Homebrew git earlier in t-man's `--path` doesn't change that. The shim
  loads its library from the active developer directory (`xcode-select -p`).
  An Xcode under `~`, such as a beta in `~/Downloads`, is unreadable inside
  the sandbox, and every git call fails with `xcrun: error: invalid active
  developer path`.

The developer directory is a machine fix, not a recipe one. Point
`xcode-select` at the Command Line Tools, or keep Xcode under
`/Applications`:

```sh
sudo xcode-select -s /Library/Developer/CommandLineTools
```

### Probing on an alternate port

Probe a settings change without touching the live service: no `ralph up`,
no restart, no re-registration. Copy the store to a scratch directory under
`~`, and write a copy of the settings with the workdir swapped for it. Then
run the installed binary on another port, with the environment launchd
gives an agent. Keep the copy named `<label>.json`, so ledger lines carry the
service's label, and keep it outside every `allowWrite`:

```bash
mkdir -p ~/srt-probe ~/.local/state/srt-probe
cp -R ~/.local/share/events ~/.local/state/srt-probe/events
jq '.filesystem[] |= map(if . == "~/.local/share/events"
      then "~/.local/state/srt-probe/events" else . end)' \
  recipes/events/events.srt.json > ~/srt-probe/events.json
cd / && env -i HOME="$HOME" USER="$USER" TMPDIR="$TMPDIR" \
  PATH=/usr/bin:/bin:/usr/sbin:/sbin \
  ~/code/bin/srt-run ~/srt-probe/events.json \
  ~/code/bin/events serve --port 17430 \
  --workdir ~/.local/state/srt-probe/events > /private/tmp/events-probe.log 2>&1 &
```

Hit `/version` and the main read endpoints, exercise one write, then read
the ledger lines for the label:

```bash
grep -F "$(printf '\tevents · ')" ~/.local/share/speak/logs/sandbox-denials.log | tail
```

A scratch directory under `~` exercises the same carve-out as the real
workdir. Send the probe's output outside `~`: srt's launch bash reads the
metadata of its own stdout and stderr, and a log file under `~` turns that
into an alert. The fleet ignore list covers only t-man's own
`~/Library/Logs/<name>/stdout.log` and `stderr.log`. Kill the probe when
done.

## Triaging a denial

`sandbox-watch` (a t-man agent, registered by the
[speak recipe](../recipes/speak/CLAUDE.md)) tails the live log for denials.
srt tags every deny rule with the base64 of the command it ran, which holds
the `SRT_RUN=<label>` marker. The watcher decodes it and writes each fleet
denial as `label · process(pid) · operation · target`, e.g.
`present · present(123) · file-read-data · /Users/x/.aws/credentials`:

- Banner. One Notification Center banner per 60s batch of `ALERT`
  lines.
- Ledger. Every fleet denial goes to
  `~/.local/share/speak/logs/sandbox-denials.log`, tagged `ALERT`, or
  `IGNORED` with a reason (`ignore-list`). Denials from an agent's own bash
  sandbox carry srt's tag but no marker; they land there as
  `IGNORED ... foreign` and never notify. Each banner is mirrored to
  `sandbox-notifications.log` beside it. `t-man logs sandbox` shows both.
- Events. Each banner is also posted to events.this with source
  `sandbox-watch` and tags `command` (the label), `process`, `op`,
  `unique`, and `status`.

Three outcomes:

1. **Real breakage.** The tool's function fails and the denied path is
   something it legitimately needs. Add the narrowest allow to its settings
   file (a path, not a parent directory), restart the process, and confirm
   it works. Never add allows preemptively.
2. **Expected noise.** The tool works and the denial is discovery probing:
   dotenv loaders, AWS/GCP SDK credential chains and `~/.netrc` lookups all
   scan paths they never use. Add a tight extended-regex (ERE) line to
   `~/.config/sandbox-watch/ignore.conf` with a comment saying why it's
   benign. Fleet-wide defaults ship in
   [`recipes/speak/sandbox-ignore.conf`](../recipes/speak/sandbox-ignore.conf)
   through a commit to this repo. A pattern can scope to one process with
   `^<label> · `.
3. **Compromise indicator.** An unexpected read of a secrets path (`~/.ssh`,
   keychains) or a network attempt from a process with no allowed domains,
   especially right after a pin bump. Don't allow or silence it. Diff the
   dependency graph against the previous pins and treat it as a potential
   supply-chain compromise until explained.

Never silence or allow a denial you can't explain. A silenced credential
read you didn't expect is exactly what an attacker wants ignored.

### Probing a process by hand

Run the binary under `srt-run` from a shell and watch the ledger:

```bash
tail -f ~/.local/share/speak/logs/sandbox-denials.log &
srt-run ~/.config/srt/present.json ~/code/bin/present mcp
```

`SRT_DEBUG=1` before `srt-run` makes srt print its debug log to stderr: the
command it wrapped, the allowed hosts, and the restrictions it applied.
Probe with a fresh path and a binary that hasn't violated recently. The
kernel reports a denial once per operation and target and folds repeats
into "N duplicate reports". It also throttles a binary that keeps
violating. After one burst from `gcat`, its later denials went unreported
for minutes, new paths included; `/bin/cat` on the same paths was reported
at once. The access is still blocked either way; only the report is
missing.
[`sandbox-audit.sh stream`](../recipes/speak/sandbox-audit.sh) shows every
denial on the machine, the system's own included, with the decoded label
under each srt-tagged line. After every pin bump, run the real workload
with one of these open and explain anything new.

## Limits

- Live telemetry is lossy. Denials only stream reliably live, with the
  predicate `eventMessage CONTAINS "deny(1)"` at default level;
  `--level debug` drops them. The kernel reports each operation and target
  once, folds repeats, throttles a binary that keeps violating, and
  `log show` after the fact is unreliable. Enforcement is solid;
  observability isn't a complete audit trail.
- srt records nothing itself. The CLI prints no violations (upstream
  issue #582), so `sandbox-watch` is the only tap.
- Apple Events are off unless the settings set `allowAppleEvents`.
- One Node parent per process. srt stays running beside each sandboxed
  process to run its proxies, so every MCP server costs one extra Node
  process.
- Launch noise. srt starts commands through bash, and its base profile
  denies `sysctl-read kern.iossupportversion` for each process in the exec
  chain. The fleet ignore list silences it.
- Resolver noise. A Go binary resolves the proxy's `localhost` through
  libSystem first. Each API call therefore logs a denied connect to
  mDNSResponder, a denied UDP query, and a denied bind before the lookup
  succeeds from `/etc/hosts`. The fleet ignore list silences those three
  shapes; a TCP connect to a real host still shows its target and still
  alerts.

## Gotchas

- `TMPDIR` needs a home. srt sets the child's `TMPDIR` from
  `CLAUDE_CODE_TMPDIR`, else `/tmp/claude`, which it allows but never
  creates, so Go's `os.CreateTemp` fails with "no such file or directory".
  `srt-run` sets `CLAUDE_CODE_TMPDIR` to the caller's `TMPDIR` (else
  `/private/tmp`) and creates it. srt does not add that dir to the write
  allow-list, so a process that writes temp files lists `/private/tmp` and
  `/private/var/folders` in `allowWrite`.
- Symlinks resolve before the check. Seatbelt matches the resolved path, so
  a ralph-managed symlink under an `allowRead` path (a config file linked
  from a repo checkout or from `~/.config/ralph/sources/thismoon`) reads
  only if its target is allowed too. Name the target in `allowRead`. The
  same holds for `allowWrite`.
- Keep the launch machinery outside every `allowWrite`: srt's install, the
  mise shims, `srt-run`, and the settings. Under `SRT_DEBUG`, srt warns when
  its own install sits in a writable path.
