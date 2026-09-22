# present configuration

present has no config file. Everything is set with flags or environment
variables, resolved once at process start by `present serve` and
`present mcp` independently.

## Where config lives

There is no config file to locate or parse. `present serve` and
`present mcp` each resolve `--workdir`/`PRESENT_WORKDIR` and
`--port`/`PRESENT_PORT` on their own; because they must agree to work
together (the MCP tools write pages into the workdir, and the URLs they
return point at the port `serve` listens on), a divergence between the two
processes is a configuration error to check for, not a bug. Both processes
log their resolved `workdir=… port=… base-url=…` to stderr at startup.

A leading `~` in `--workdir` is expanded before use. Flags take precedence
over environment variables, which take precedence over the built-in
defaults. A `~` that cannot be expanded — no resolvable home directory,
which happens under a launchd agent with a stripped environment — is an
error at startup. Earlier releases stripped the `~` and used a path relative
to the working directory instead, which quietly served an empty store.

## Flags

These are persistent flags, seen by every subcommand (`present serve`,
`present mcp`, `present share`, `present unshare`):

- `--workdir` (string; env `PRESENT_WORKDIR`): directory holding `pages/`,
  the page store both processes read and write. The default is
  `~/.config/present` while that directory holds a `pages/` subdirectory,
  otherwise `$XDG_STATE_HOME/present` (or `~/.local/state/present` when
  that variable is unset or not absolute).
- `--port` (int, default `7423`; env `PRESENT_PORT`): port `present serve`
  listens on. Also used to compute the default `--base-url`, and by
  `present mcp` to build the page URLs it returns.
- `--base-url` (string, default `http://localhost:<port>`; env
  `PRESENT_BASE_URL`): base URL used when building page links, e.g. in
  `present_create`'s returned URL. Only needs overriding when pages are
  reached through something other than plain `localhost:<port>` (for example
  the local domain front door's `present.this`).
- `--shared-url` (string; env `PRESENT_SHARED_URL`): the shared present
  instance local pages can be pushed to, e.g. `https://present.example.com`.
  Sharing is on only when `--author-key` is set as well.
- `--author-key` (string; env `PRESENT_AUTHOR_KEY`): the author key sent to
  the shared instance as a bearer token on every push; mint one with
  `present key new`. Prefer the environment variable over the flag: an
  argument list is visible to other processes on the machine, the
  environment is not. With only one of the two set, the process warns once
  on stderr and sharing stays off.

These two belong to `present serve` alone:

- `--bind` (string, default `127.0.0.1`; env `PRESENT_BIND`): interface the
  server listens on. Local mode refuses anything but loopback, because it
  authenticates nothing. Shared mode refuses to start unless the bind is
  given explicitly (`0.0.0.0` inside a container, `127.0.0.1` to try it on
  one machine), so exposure is always a stated choice.
- `--shared` (bool, default `false`; env `PRESENT_SHARED`): run as a shared
  instance: no index or listing, pages by id only, an author key on every
  write, and the MCP tools over HTTP at `/mcp`. `--base-url` then acts as a
  display override for the URLs writes return; left empty, each URL is
  derived from the request's `X-Forwarded-Proto` and `X-Forwarded-Host`
  headers, or from its `Host` when nothing in front forwarded them.
- `--store` (string, default `fs`; env `PRESENT_STORE`): where pages live.
  `fs` is the workdir. `k8s` keeps each page as a `Page` custom resource
  (`present.thismoon.mad01.dev/v1alpha1`) in `--namespace`, reached through
  the in-cluster credentials when running in a pod and the default
  kubeconfig otherwise; it is the store a shared instance with several
  replicas uses and is refused without `--shared`.
- `--namespace` (string; env `PRESENT_NAMESPACE`): namespace the `k8s`
  store keeps pages in. Unset, it is the pod's own namespace, else the
  kubeconfig context's, else `default`.
- `--sweep-interval` (duration, default `10m`; env `PRESENT_SWEEP_INTERVAL`):
  how often the `k8s` store deletes expired ephemeral pages. Expired pages
  already read as missing between sweeps; the interval only bounds how long
  their objects linger.

## Where the page store lives

The store moved out of `~/.config` — pages are state, not configuration —
but **existing pages are never migrated**. The resolution is deliberately
sticky: a machine that has published pages keeps reading them where they
are, so upgrading never orphans a page or breaks the URLs handed out for it.

What makes a machine count as "has published pages" is a `pages/` directory
inside `~/.config/present`, not the directory itself. Provisioning puts the
render template and assets there on every machine, so the directory alone
says nothing about whether anything was ever published — a machine with the
template but no `pages/` lands in the XDG state directory like any fresh
install.

Nothing changes for a supervised install: the fleet recipe passes
`--workdir` to `present serve` explicitly, and the MCP's seatbelt wrapper
sets `PRESENT_WORKDIR`, so both processes agree regardless of which branch
the default would take. To move an existing store, move the directory and
set `PRESENT_WORKDIR` (or `--workdir`) to the new path on both processes;
there is no automatic migration to undo.

## Environment variables

- `PRESENT_WORKDIR`: default for `--workdir`.
- `PRESENT_PORT`: default for `--port`. An unparseable value warns once on
  stderr, and the built-in default (`7423`) is used instead.
- `PRESENT_BASE_URL`: default for `--base-url`.
- `PRESENT_SHARED_URL`: default for `--shared-url`.
- `PRESENT_AUTHOR_KEY`: default for `--author-key`, and the recommended way
  to set it: the environment is private to the process, the argument list
  is not.
- `PRESENT_BIND`: default for `--bind`. Setting it counts as an explicit
  bind for shared mode.
- `PRESENT_SHARED`: default for `--shared`; `1`, `true`, `0`, `false` in any
  case. An unparseable value warns once on stderr and the default (`false`)
  is used instead.
- `PRESENT_STORE`: default for `--store`.
- `PRESENT_NAMESPACE`: default for `--namespace`. The Kubernetes manifests
  set it from the pod's own namespace.
- `PRESENT_SWEEP_INTERVAL`: default for `--sweep-interval`, in
  `time.ParseDuration` syntax (`10m`, `1h30m`). An unparseable value warns
  once on stderr and the default is used instead.

To see what a given environment actually resolved to, run `present doctor`;
an agent with no shell gets the same report from the `present_doctor` MCP
tool. It checks the page store first, since the MCP tools write it directly
and keep working while `serve` is down. With a shared instance configured
it also calls that instance's `GET /api/whoami` with the key, so a wrong URL
or a refused key shows up before the first share.

## Example

```bash
present serve --workdir ~/.config/present --port 7423
present mcp --workdir ~/.config/present --port 7423
```

Equivalent via environment variables:

```bash
export PRESENT_WORKDIR=~/.config/present
export PRESENT_PORT=7423
present serve
```

A shared instance inside a container, and the same thing tried on one
machine:

```bash
present serve --shared --store k8s --bind 0.0.0.0 --port 7423 --workdir /var/lib/present
present serve --shared --bind 127.0.0.1 --port 17423 --workdir /tmp/present-shared
```

The first is what the container runs: pages in the pod's namespace, the
workdir holding only the baked assets. The second tries shared mode on one
machine with the filesystem store, which is fine for a single process. From
a developer machine, `--store k8s --namespace present` on top of the second
line uses the kubeconfig's current context, which is how you exercise a
kind cluster by hand.

A local present pointed at a shared instance, so its pages can be shared
from the page view, the CLI, or the `present_share` tool:

```bash
present key new                                   # once; keep the output somewhere safe
export PRESENT_SHARED_URL=https://present.example.com
export PRESENT_AUTHOR_KEY=<the key you minted>
present serve                                     # the page view gains a Share button
present share <id>                                # the same push from a shell
```

Both variables have to reach every process that shares: `present serve` for
the Share button, `present mcp` for `present_share`, and the shell that runs
`present share`. On a ralph-provisioned machine one place covers all three:
the recipe's serve launcher and MCP wrapper read exactly these two exports
from the ralph-managed secrets file, and the login shell sources it. A
shared instance never sets them; it is where pages land, not where they
come from.
