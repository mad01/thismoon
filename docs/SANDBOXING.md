# Sandboxing third-party code with srt

Some processes in the fleet run third-party package code: a PyPI engine, a
go-installed binary, a Model Context Protocol (MCP) server. Each one runs
under [anthropics/sandbox-runtime](https://github.com/anthropics/sandbox-runtime)
(srt), so a poisoned dependency can't read secrets or exfiltrate data. This
page covers why srt, the launch pattern, how a process declares its access,
how it stays reconciled through ralph, and how to triage denials.

## Sandbox a new process

The whole job, in the order it happens. Each step links to the section
that explains it.

1. **Declare what it may reach.** A component adds `spec.access` to its
   `service-info.yaml`. A recipe directory with no component, such as an
   MCP server a consuming repo registers, adds a standalone `access.yaml`.
   Start minimal: `sandbox: srt`, `listen` with the port for a service,
   `reads: [~/code/bin, workdir]`, `writes: [workdir, temp]`, no `egress`.
   Add `local: [events]` if it posts to events, `tls: true` for a Go
   binary that makes TLS calls, `egress` only for hosts every machine
   uses, and the literal `via SRT_RUN_ALLOW_DOMAINS` for a host that
   differs per machine. Comment each root with why the process needs it.
   ([Declaring access](#declaring-access))
2. **Render the settings.** `make sandbox-settings` here, or
   `srtspec render <manifest> -o recipes/<dir>/<label>.srt.json` in a
   consuming repo. The renderer refuses a `*` host, a lifted `~` deny and
   a write into `~/.config/srt`. Never edit the JSON; `srtspec verify`
   fails CI on drift.
   ([Render, check, generate, verify](#render-check-generate-verify))
3. **Link it into place.** A `[dotfiles.<name>_srt_settings]` item
   symlinks `<label>.srt.json` to `~/.config/srt/<label>.json`, with no
   profile gate: one file serves every machine. The file name is the label
   sandbox-watch attributes denials by.
   ([Everything through ralph](#everything-through-ralph))
4. **Launch through srt-run.** A t-man service names
   `~/code/bin/srt-run <settings> <command...>` as its program in a
   `run = "always"` hook, with `--extra-log sandbox=...` so `t-man logs`
   finds the denial ledger. An MCP server gets a wrapper: resolve secrets
   outside the sandbox, derive the per-machine host into
   `SRT_RUN_ALLOW_DOMAINS`, `cd /`, `env -i` with an allow-list, exec
   `srt-run`. Register the wrapper and keep it executable with a chmod
   hook. ([The pattern](#the-pattern),
   [Sandboxing a t-man service](#sandboxing-a-t-man-service))
5. **Probe, then widen only what a denial shows.** Start it, exercise every
   path (a request, a write, the egress call), and read the ledger with
   `sandbox-audit.sh ledger <label>` or `t-man logs <name> --source
   sandbox`. Each ALERT line names the operation and the target. A read
   through a symlink fails until its target is in `reads` too.
   `srtspec explain <label>` prints the effective policy with the layer
   each entry came from. ([Triaging a denial](#triaging-a-denial))

Rules every manifest and wrapper follow, each explained further down:

- Hosts that differ per machine come from the wrapper, never the manifest
  ([Machine-specific hosts](#machine-specific-hosts)).
- Name hosts. `*` and `*:port` are refused everywhere.
- No sandbox may write `~/.config/srt`, and no settings file sits under
  any sandbox's `allowWrite`
  ([Settings live outside every allowWrite](#settings-live-outside-every-allowwrite)).
- Secrets never enter the sandbox: the wrapper resolves them and passes
  only what the process needs ([The pattern](#the-pattern)).
- A symlink reads only when its target is allowed too. A target only one
  machine has goes in that machine's expansion layer
  ([The machine-local expansion layer](#the-machine-local-expansion-layer)).
- List `temp` in `writes` for anything that writes temp files
  ([Gotchas](#gotchas)).
- A process srt can't confine is left out, never forced, and says so with
  `sandbox: none` or `sandbox: seatbelt` plus a reason
  ([The legacy path](#the-legacy-path-a-seatbelt-profile-under-t-man)).

[`examples/dotfiles/recipes/mcp-sandbox/`](../examples/dotfiles/recipes/mcp-sandbox/)
is a complete worked example for a consuming repo: the manifest, its
render, the wrapper, and the agent-sync drop-in that registers the server.
The mise pins for srt and bun sit beside it in
[`examples/dotfiles/recipes/mise/`](../examples/dotfiles/recipes/mise/).

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
   ([`recipes/speak/srt-run.sh`](../recipes/speak/srt-run.sh)): a shim
   that execs `srtspec run` ([`tools/srtspec`](../tools/srtspec/)). Usage:
   `srt-run <settings-file> <argv...>`. It expands a leading `~` in the
   settings path, merges the machine's layers into a copy srt reads, finds
   srt through the mise shims dir, sets the child's `TMPDIR`, and runs
   `srt --settings <abs> -- env SRT_RUN=<label> <argv...>`, with srt itself
   under bun when one is installed ([bun or node](#bun-or-node)). The label
   is the settings file's name (`~/.config/srt/present.json` gives
   `present`); `SRT_RUN_LABEL` overrides it. Exit 2 is a usage error, 127 a
   missing srt or runtime, 1 a refused layer.
2. The settings file, `~/.config/srt/<label>.json`, rendered from the
   process's access manifest (see [Declaring access](#declaring-access)).
   A file that is the same on every machine ships beside its service's
   recipe here. One that differs per machine ships from the consuming repo
   as a machine-private overlay
   ([docs/adr/0006](adr/0006-recipe-layering-and-platform-deps.md)). srt
   fails closed: a missing or invalid file exits 1 and the command never
   runs.
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
is the consuming-repo version for a third-party server: the manifest, its
render, the wrapper that hands the GitHub host to `srt-run`, and the
agent-sync drop-in that registers it.

t-man services run `srt-run` as their program, with or without a wrapper
in front of it. See [Sandboxing a t-man service](#sandboxing-a-t-man-service).

### bun or node

srt is a node CLI, and its process stays up beside the command for the
command's whole life. It hosts the HTTP and SOCKS proxies that enforce
`allowedDomains` and forwards signals, so each sandboxed process costs one
extra runtime process. bun runs the same JS in less memory. Resident size
of the srt parent, with srt 0.0.81 on macOS, for one sandboxed `sleep`
after five proxied requests:

| Runtime | Parent RSS |
|---------|------------|
| node 24 | about 83 MB |
| bun 1.4.4 | about 48 MB |

`SRT_RUN_RUNTIME` picks `bun` or `node`. Unset, `srt-run` uses bun when it
finds one, in mise's installs dir first and then on `PATH`, and node
otherwise. bun, like srt, is a mise tool the consuming repo declares. Each
launch prints `srt-run: <label> runtime <bun|node>` to stderr, so a
service's stderr log names the runtime it started under.

Under bun, `srt-run` runs srt's `dist/cli.js` itself, because the npm bin
wrapper execs node. It asks `mise which srt` for the bin the pin selects
and follows it to the package. When it can't find `cli.js`, the launch
stops with exit 127, and `SRT_RUN_RUNTIME=node` goes back to the wrapper.
bun runs with `--no-env-file` and `--config=/dev/null`. By default it loads
`.env` and `bunfig.toml` from the cwd, and a `bunfig.toml` preload would run
a project's code in the unsandboxed parent. `--no-install` keeps it from
fetching a package that is missing.

The policy and the denial tag don't depend on the runtime, since srt's own
JS builds both. Under bun, an allowed host answers 200 through the proxy and
any other host gets nothing. Denials keep their `SRT_RUN` label in the
ledger, a SIGTERM to the parent ends the command, and exit codes and stdio
pass through.

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
own inputs at launch instead. Its manifest says so with the literal egress
entry `via SRT_RUN_ALLOW_DOMAINS`, which renders nothing. The wrapper exports `SRT_RUN_ALLOW_DOMAINS`
(comma-separated `host[:port]`), and `srt-run` merges it into
`network.allowedDomains`. The merged copy goes to
`~/.config/srt/merged/<label>.json` (mode 600, rewritten every launch) and
srt reads that. A host the launcher cannot parse, or a failed merge, stops
the launch. present's wrapper derives the host from `PRESENT_SHARED_URL`, so
its static file keeps `allowedDomains` empty. Never use `*`: srt 0.0.81
rejects a bare `*` or `*:443`, and `srt-run` refuses one in the variable.
Anything else a machine adds goes in the
[expansion layer](#the-machine-local-expansion-layer), which srt-run merges
in the same step.

### The legacy path: a seatbelt profile under t-man

srt's settings have no key for some sandbox classes. speak-tts needs
`iokit-open` to reach Metal for its MLX engine, so it keeps a hand-written
seatbelt profile run by t-man's `--sandbox-profile` flag
([`tools/t-man/config.md`](../tools/t-man/config.md)). Use this path only
when srt can't express what the service needs, and say so in the manifest
with `sandbox: seatbelt` and the reason. Tag every deny rule in the
profile with `(with message "SRT_RUN=<label>")`. The log then appends the
marker to each denial, and `sandbox-watch` attributes it like an srt
process. t-man hashes the profile's content, so editing it and re-running
`ralph up` re-renders the plist and restarts the service.

## Declaring access

Every sandboxed process declares what it may reach in an access manifest
beside its code. The srt settings file is rendered from the manifest and
never edited by hand: the YAML is what a reviewer reads, the JSON is what
srt enforces
([docs/adr/0027](adr/0027-access-manifest-source-of-truth.md)). The
[`srtspec`](../tools/srtspec/CLAUDE.md) tool renders it, and the schema
lives in [`kit/access`](../kit/access/access.go).

### The manifest

A component declares it as `spec.access` in its `service-info.yaml`. events
serve, for example:

```yaml
spec:
  access:
    sandbox: srt
    workdir: ~/.local/share/events
    listen: 7430
    reads: [~/code/bin, workdir]
    writes: [workdir, temp]
```

A recipe directory with no component, such as an MCP server's settings in
the consuming repo, holds the same block as a standalone `access.yaml`:

```yaml
# recipes/claude-mcp/access.yaml
sandbox: srt
label: github-mcp
egress: [via SRT_RUN_ALLOW_DOMAINS]
reads: [~/code/bin]
writes: [temp]
tls: true
```

| Field | Meaning | Renders to |
|---|---|---|
| `sandbox` | `srt`, `seatbelt` or `none`. Required. | Only `srt` renders a file. |
| `reason` | Why the process isn't under srt. Required for `seatbelt` and `none`. | Nothing. |
| `label` | The srt-run label. Defaults to the component name, or to the directory name for an `access.yaml`. | The file name, `<label>.srt.json`. |
| `workdir` | The process's own state directory. | The `workdir` symbol's value. |
| `listen` | The loopback port it serves on. | `network.allowLocalBinding: true` |
| `local` | Loopback peers it calls, by name (`events`). | `network.allowLocalBinding: true` |
| `egress` | External hosts: `host`, `*.domain` or `host:port`. A host that differs per machine is the literal `via SRT_RUN_ALLOW_DOMAINS`. | `network.allowedDomains`. The literal renders nothing. |
| `reads` | Paths carved out of the `~` read deny. | `filesystem.allowRead` |
| `writes` | Paths it may write. | `filesystem.allowWrite` |
| `tls` | It checks TLS certificates in-process, as Go binaries do. | `enableWeakerNetworkIsolation: true` |
| `deny` | Extra read denies under the `reads` roots. | `filesystem.denyRead`, after `~` |
| `extra` | Raw srt settings for anything the fields can't say. | Merged last: objects merge, lists union, scalars override. |

`reads` and `writes` take two symbols: `workdir`, and `temp` for
`/private/tmp` and `/private/var/folders`. Paths start with `/` or `~/`;
globs work. Write `temp` wherever the process writes temp files, even
though the render adds both roots anyway: the manifest is documentation
first. `extra` is the escape hatch for keys such as `allowAppleEvents`,
`allowPty` or `network.allowMachLookup`.

### The fleet default

Every render starts from the same floor: `denyRead: ["~"]`, both temp roots
in `allowWrite`, and empty `allowedDomains`, `deniedDomains` and
`denyWrite`. `allowLocalBinding` is on only when `listen` or `local` is
set. After `extra`, the renderer checks the result still holds three
invariants. No `*` or `*:port` host, `~` stays read-denied, and no write
reaches `~/.config/srt` or the ralph sources cache. A manifest that breaks
one doesn't render. The decoder is strict, so a misspelled field fails
instead of silently dropping a deny. A `~/` deny that sits under no `reads`
root is refused as a likely typo, since `~` already covers it.

### Render, check, generate, verify

```bash
srtspec render services/events/service-info.yaml        # JSON to stdout
srtspec check services/events/service-info.yaml recipes/events/events.srt.json
make sandbox-settings                                    # render every manifest in the repo
make sandbox-verify                                      # the CI gate; srtspec verify . elsewhere
```

`make sandbox-settings` runs `srtspec generate`. It renders a component's
manifest to `recipes/<component dir>/<label>.srt.json`, and an
`access.yaml` into the directory it sits in. Output is deterministic:
sorted keys, list entries in manifest order, two-space indent.

`make sandbox-verify` runs `srtspec verify`, the gate CI runs. It writes
nothing and prints one line per check. It fails when a committed settings
file is not its manifest's render byte for byte. It also fails when a
`recipes/*/*.srt.json` has no manifest, and when a `<label>.local.json` kept
beside a recipe does more than widen. So neither a hand edit to a settings
file nor a manifest change without its render can merge. A consuming repo runs `srtspec verify .`
over its own recipes the same way. `check` compares one pair by meaning,
ignoring key and list order, and exits 1 with a diff on drift.

### The machine-local expansion layer

Some machines need more than the shipped policy: another code root, a
cache, a host only that machine talks to. That goes in
`~/.config/srt/<label>.local.json`, never in the manifest. srt-run merges it
at launch, together with `SRT_RUN_ALLOW_DOMAINS`, into
`~/.config/srt/merged/<label>.json`. The layer may only widen. It can:

- add to `network.allowedDomains`, `network.allowMachLookup`,
  `network.allowUnixSockets`, `filesystem.allowRead` and
  `filesystem.allowWrite`;
- set `network.allowLocalBinding` or `enableWeakerNetworkIsolation` to
  `true`.

```json
{
  "_comment": "kof on this machine also checks pins in ~/notes",
  "filesystem": { "allowRead": ["~/notes"] }
}
```

Anything else makes srt-run exit 1 before srt starts, naming each problem.
That covers any other key, and an allow that undoes a deny: one equal to a
deny entry, or `~` itself. It also covers a write that reaches
`~/.config/srt` or the sources cache, and a `*` or `*:port` host. Invalid
JSON or a failed merge stops the launch as well. The layer lives in
`~/.config/srt`, which no sandbox may write.

A config file reached through a machine-specific symlink is the common
case. srt matches resolved paths, so the manifest's path alone does not
cover a link whose target only one machine has. That target goes in the
machine's layer. speak-web is the worked example: its manifest reads
`~/.config/speak/config.yaml`, and a consuming repo that links that file
from its own checkout also ships `speak-web.local.json` with the target:

```json
{
  "_comment": "~/.config/speak/config.yaml links into the dotfiles checkout",
  "filesystem": { "allowRead": ["~/code/dotfiles/speak/config.yaml"] }
}
```

Without it speak-web starts, but every read of the config fails with
`operation not permitted`. speak logs that error at startup together with
the layer file to add.

### Explain

`srtspec explain <label>` prints the policy a launch would get. Each row is
one entry with the layer it came from: `base` (the settings file), `local`
(the expansion layer) or `env` (`SRT_RUN_ALLOW_DOMAINS`, read from the
calling shell). Where `srtspec run` would refuse to launch, explain says
why and exits 1. The two commands merge through one package, and both run
the cases in
[`recipes/speak/testdata/srt-local`](../recipes/speak/testdata/srt-local).

### srt keys

What the renderer writes, for reading a render or writing `extra`. srt
requires all four `filesystem` lists and both domain lists, even when empty.
`~` expands inside the file. A `_comment` key is accepted.

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

The temp roots are in every render because `srt-run` points the child's
`TMPDIR` at the caller's temp dir or `/private/tmp`, and srt doesn't allow
that dir on its own.

Settings are read at spawn time. After a manifest change, render, then
restart the process: a new agent session or `/mcp` reconnect for an MCP
server, `t-man restart` for a service. After editing an expansion layer,
only the restart is needed.

## Reads policy

Deny `~`, then allow what the process needs. A secrets blocklist can never
be complete, so the default is the whole home directory off. A typical
allow-list is `~/code/bin` for the binary plus the process's own state
directory.

A process that reads source (humanizer scanning docs, say) gets the code
roots back in `reads`. Secret files live inside those roots too, so add
nested denies under them:

```yaml
reads: [~/code]
deny:
  - ~/code/**/.env
  - ~/code/**/.env.*
  - ~/code/**/*.pem
  - ~/code/**/*.key
```

They render after `~` in `denyRead`. The nested denies are more specific
than the `~/code` allow, so they win.
Anchor them under `~/`: a bare `**/.env` resolves against the working
directory, and the wrapper's `cd /` makes that a different tree from the
one you meant.

## Everything through ralph

`ralph up` reconciles every sandbox artifact; nothing is manual-only. For a
new sandboxed process, confirm:

- The process declares an access manifest, and the settings file is its
  render, committed in the recipe dir that links it to
  `~/.config/srt/<label>.json`. That is the service's recipe here when the
  file is the same on every machine, and the consuming repo's otherwise.
  `make sandbox-verify` passes. `catalog validate` fails a
  component whose recipe registers a t-man service or execs `srt-run`
  without `spec.access`.
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

events, keeper-of-facts, moondb, catalog-web, present serve, deps, status
and speak-web run this way. What carries over to the next one:

- **The label is the service name.** The settings file's name gives the
  label, and it matches the t-man name. present serve is the exception: its
  label is `present-serve`, because the MCP server already uses `present`.
- **Generic settings ship beside the recipe.** When nothing in them differs
  per machine, the service declares `spec.access` in its `service-info.yaml`
  and `make sandbox-settings` renders `recipes/<svc>/<label>.srt.json`,
  linked to `~/.config/srt/<label>.json` by a `[dotfiles.<svc>_srt_settings]`
  item. `recipes/speak/srt-settings-test.sh`, part of `make test-recipes`,
  checks every shipped file and its wiring.
- **A wrapper only when there is something to resolve.** launchd starts an
  agent in `/` with a small environment, so the cwd and env reasons for an
  MCP wrapper don't apply. A service that needs a secret or a
  machine-specific host keeps a launcher that resolves them outside the
  sandbox and then execs `srt-run`. `recipes/present/present-serve.sh` reads
  the sharing config and hands the shared host to `srt-run` that way.
  `recipes/speak/speak-serve.sh` exports only the provider's key from the
  secrets file and hands over the provider's host, and
  `recipes/deps/deps-serve.sh` puts mise's go on the child's `PATH`.
- **Ordering.** ralph links every dotfiles item before any build or
  package runs, so the settings link and `~/code/bin/srt-run` exist before
  the registration hook. ralph's `depends_on` accepts only `builds.` and
  `packages.` keys, so the hook depends on `builds.speak_chmod_scripts`,
  which keeps `srt-run.sh` executable, and on `packages.srtspec`, the
  binary the shim execs; the srtspec recipe sits in wave 0 with the
  services for that reason.
- **Start minimal.** `listen` with the port (and `local: [events]` if it
  posts to events), `reads: [~/code/bin, workdir]`, `writes: [workdir,
  temp]`, no `egress`. Add only what a probe shows the service needs.

Three services stay outside srt: csl-web, sandbox-watch and d-man. The ADR
amendment in [docs/adr/0006](adr/0006-recipe-layering-and-platform-deps.md)
records why. Their components say so with `sandbox: none` and the reason.

### git inside the sandbox

A service that runs git under `denyRead: ["~"]` needs two things:

- **A global config git can skip.** git dies on a read-denied
  `~/.gitconfig` (`fatal: unable to access '~/.gitconfig': Operation not
  permitted`). keeper-of-facts and moondb only run read-only commands
  (`rev-parse`, `blame`), which need no identity or credential helper, so
  they get `--env GIT_CONFIG_GLOBAL=/dev/null`. The repo's own `.git/config`
  has to stay readable, because git reads it on every command.
- **A readable developer directory.** `srt-run` puts `/usr/bin` ahead of the
  caller's `PATH`, so git resolves to `/usr/bin/git`, an xcrun shim that
  loads libxcrun from the active developer directory. A Homebrew git earlier
  in t-man's `--path` doesn't change that. An Xcode under `$HOME`, such as a
  beta in `~/Downloads`, is unreadable inside the sandbox, and every git call
  fails with `xcrun: error: invalid active developer path`. The Command Line
  Tools under `/Library` are always readable, so the keeper-of-facts and
  moondb hooks add `--env DEVELOPER_DIR=/Library/Developer/CommandLineTools`
  when that directory exists. Machines without them keep `xcode-select`'s
  choice.

Note: the machine can fix the developer directory for every process instead,
with `sudo xcode-select -s /Library/Developer/CommandLineTools` or by keeping
Xcode under `/Applications`. The hooks don't depend on it.

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
- Ledger format. A line is tab-separated
  `time  status  summary  [reason  [×N]]`. Each `ALERT` is written as it
  arrives. `IGNORED` lines are held for the 60s window, or until the next
  `ALERT`, and written once per distinct denial. Repeats with the same
  label, process name, operation, target and reason, under any pid, become
  a trailing `×N` on the first sighting's line, which keeps that
  sighting's time and pid. A process that execs the same helper every few
  seconds then costs a few lines a minute, not one per denial.
- Rotation. The watcher rotates both files itself at a window boundary. A
  file past 5 MiB moves to `.1`, older ones shift up, three generations
  are kept, and the live file starts again empty under the same path.
  `sandbox-audit.sh ledger [label]` reads every generation, oldest first.
  Follow the live file with `tail -F` or `t-man logs -f`, which both
  reopen it by path.
- Events. Each banner is also posted to events.this with source
  `sandbox-watch` and tags `command` (the label), `process`, `op`,
  `unique`, and `status`.

Three outcomes:

1. **Real breakage.** The tool's function fails and the denied path is
   something it legitimately needs. Add the narrowest allow (a path, not a
   parent directory) to its manifest and render, or to the machine's
   expansion layer when only that machine needs it. Restart the process and
   confirm it works. Never add allows preemptively.
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
tail -F ~/.local/share/speak/logs/sandbox-denials.log &
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
- One srt parent per process. srt stays running beside each sandboxed
  process to run its proxies, so every MCP server costs one extra bun
  process, or node without bun ([bun or node](#bun-or-node)).
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
  only if its target is allowed too. Name the target in `allowRead`: in the
  manifest when every machine links the same target, and in the
  machine-local expansion layer when the target is machine-specific. The
  same holds for `allowWrite`.
- Keep the launch machinery outside every `allowWrite`: srt's install,
  bun's install, the mise shims, `srt-run` and `srtspec`, and the settings.
  Under `SRT_DEBUG`, srt warns when its own install sits in a writable path.
