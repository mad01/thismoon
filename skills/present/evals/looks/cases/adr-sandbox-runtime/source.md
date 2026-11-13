# ADR-0028: sandbox-runtime replaces hand-written seatbelt profiles

**Date:** 2026-10-26
**Status:** Accepted

**Scope:** every process the fleet confines: the t-man services whose
settings ship here, and the MCP servers and install steps a consuming repo
registers. Decided 2026-10-25 (MAD-461, MAD-464). The record lived in a
consuming repo until now and moves here because the decision is the
fleet's. ADR-0027 builds on it, and the 2026-10-25 amendments to ADR-0006
say where each settings file lives.

## Context

Every sandboxed MCP server and install step ran under a hand-written
seatbelt profile through `sandbox-exec`, one `.sb` file per process. Each
started from `(allow default)` and subtracted, and each repeated the same
scaffolding: a `HOME` parameter, parent-path metadata allows, `/dev/null`,
DNS sockets.

Seatbelt cannot filter egress by hostname. Every profile that needed HTTPS
allowed outbound port 443 to any host. So the argument the sandboxes were
there for did not hold: a poisoned dependency could read whatever the
profile allowed and send it anywhere.

The watcher attributed denials by process name. It carried a list of names
to match, and every new sandboxed process meant another entry.

[anthropics/sandbox-runtime](https://github.com/anthropics/sandbox-runtime)
(srt) generates a default-deny seatbelt profile from a short JSON settings
file. It routes network traffic through local HTTP and Socket Secure
(SOCKS) proxies that allow only the listed hosts and deny direct
connections. It is the sandbox Claude Code uses for its own shell, so macOS
changes that break it get fixed upstream.

## Decision

srt replaces the hand-written profiles.

- **Installed through mise**, as `npm:@anthropic-ai/sandbox-runtime`,
  pinned to an exact version. It is a security boundary, so a bump is a
  deliberate edit that re-checks every settings file. The consuming repo
  declares the pin; `srt-run` finds the binary through the mise shims.
- **One settings file per process**, at `~/.config/srt/<label>.json`, the
  same on every machine. It holds the static policy only. A file that is
  the same everywhere ships beside the service's recipe here; one a
  consuming repo owns ships from there (ADR-0006). No sandbox may write
  `~/.config/srt`, so no confined process can rewrite its own policy, and
  the files can be symlinks into a checkout.
- **One launch path.** `recipes/speak/srt-run.sh`, linked to
  `~/code/bin/srt-run`, runs
  `srt --settings <path> -- env SRT_RUN=<label> <argv>`. The label is the
  settings file's name. A wrapper in front of it resolves secrets outside
  the sandbox, rebuilds the environment with an `env -i` allow-list, and
  runs `cd /` first.
- **Egress by hostname, with per-machine hosts chosen at launch.** The
  settings name the hosts every machine uses. A host that differs per
  machine (an LLM proxy, a GitHub Enterprise instance, a shared present)
  arrives from the wrapper as `SRT_RUN_ALLOW_DOMAINS`, which `srt-run`
  merges in before srt starts. No machine's hostname lands in a committed
  file.
- **`*` is never allowed.** srt rejects a bare `*` in a settings file, and
  `srt-run` refuses one in `SRT_RUN_ALLOW_DOMAINS`.
- **Reads: deny `~`, carve back what the process needs.** srt globs cannot
  express "prose and Go files only", so a process that reads source gets
  the code roots back, and nested denies keep secret-shaped files (`.env`,
  `*.pem`, `*.key`, `.git/config`) unreadable inside them. The most
  specific rule wins.
- **The watcher stays ours.** The srt CLI records no violations (upstream
  issue #582), so `sandbox-watch` keeps streaming the unified log. Each
  srt denial carries the base64 of the command, and the watcher decodes
  the `SRT_RUN` label from it. There is no process list to maintain.
- **A process srt cannot confine is left out, not forced.** The speak-tts
  runtime needs `iokit-open` for Metal, which srt has no key for, so it
  keeps a seatbelt profile under t-man's `--sandbox-profile`. Every deny
  rule in that profile is tagged `(with message "SRT_RUN=speak-tts")`, so
  the watcher attributes it the same way. csl-web (ssh pulls the proxies
  cannot carry), sandbox-watch (the monitor itself) and d-man (a root
  daemon) stay outside; their manifests say `sandbox: none` and why.

The first-party Go services run under it too: events, keeper-of-facts,
moondb, catalog-web and present serve first, then deps, status and
speak-web. Their risk is module dependencies rather than code they
execute; the gain is egress default-deny and writes confined to the
service's own directory.

## Consequences

- A compromised dependency reaches only the hosts its settings name.
  Readable files can leave only through those hosts.
- A process that reads source can read any file under the code roots that
  isn't secret-shaped, a token in a YAML config included. Accepted: egress
  is pinned, and the calling agent reads the same files.
- Go binaries need `enableWeakerNetworkIsolation` to reach trustd for TLS
  verification, which srt documents as a possible exfiltration channel.
  Manifests say `tls: true` for it.
- The egress policy lives in two places: the settings and the wrapper that
  names the per-machine hosts. A wrapper change is a policy change and
  gets the same review. The merged copy a running process got is in
  `~/.config/srt/merged/<label>.json`.
- Settings must never sit in a directory their own sandbox can write.
  srt's `denyWrite` protects a regular file but only the target of a
  symlink, so a writable directory would let a confined process swap the
  link and widen its next launch.
- Seatbelt checks a symlink's resolved path, so a link inside an allowed
  directory is readable only when its target is allowed too.
- srt hands the child `TMPDIR` from `CLAUDE_CODE_TMPDIR`, else
  `/tmp/claude`, which it never creates. `srt-run` sets and creates it,
  and every settings file lists the temp roots in `allowWrite`.
- Each sandboxed process costs one srt parent process beside it, under bun
  when one is installed and node otherwise.
- Settings are read at spawn time: after a change, restart the process. A
  missing srt or settings file stops the launch; nothing runs unconfined.
- Reverting means restoring the deleted `.sb` files from git and pointing
  the wrappers back at `sandbox-exec`.
