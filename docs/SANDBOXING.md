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
2. The settings file, `~/.config/srt/<label>.json`. The consuming repo
   ships it as machine-private overlay. This repo ships none. srt fails
   closed: a missing or invalid file exits 1 and the command never runs.
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

t-man services use the same wrapper as their program: `t-man add --name
<svc> -- <wrapper> <args>`.

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

- The settings file lives in the consuming repo's recipe dir and is
  symlinked to `~/.config/srt/<label>.json` by that recipe.
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
- A `run = "always"` recipe hook registers each t-man service with the
  wrapper as its program, never a one-off `t-man add`. The service also gets
  `--extra-log sandbox=$HOME/.local/share/speak/logs/sandbox-denials.log`,
  so `t-man logs <name> --source sandbox` finds the denial ledger.
- Nothing to add to `sandbox-watch`. It attributes denials by the
  `SRT_RUN` label, so a new process is covered the moment it launches
  through `srt-run`. A legacy seatbelt profile is covered once every deny
  rule carries the marker.
- Acid test: a fresh `ralph up` on a clean machine reproduces the posture.

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
