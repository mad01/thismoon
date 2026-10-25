# ADR-0025: present terminal sessions live in serve behind an opt-in switch

**Date:** 2026-10-19
**Status:** Accepted

**Scope:** `services/present`: the `terminal` Doc block, `internal/terminal`,
the terminal routes in `internal/server`, the `present terminal` command,
and the page shell's xterm.js client. Decided in MAD-414.

## Context

A talk about agent tooling wants the agent on a slide. The presenter starts
Claude on one slide, talks through the next few while it works, and comes
back to it later in the deck. The page cannot hold that state. Every update
reloads the open tab, edits to the deck mid-talk included, so a terminal the
tab owned would die on the first edit. The process has to outlive the tab,
and a later slide has to find it again by name.

Only one present process can hold it. `present mcp` runs inside a seatbelt
profile that denies most of `$HOME`, and it exits with its client.
`present serve` is the t-man agent: long-running, unsandboxed, with the
user's own rights. A process serve starts can do anything the user can. And
page content reaches the browser through innerHTML with no Content Security
Policy, so anything that gets HTML into a page can run script in the page
view.

A shared instance serves readers who are not on the machine. It has no
business running a process for them.

## Decision

Serve owns the sessions. A terminal session is a process in a
pseudo-terminal (PTY), keyed by page id and session name, running
`zsh -l -i` or `zsh -l -i -c <cmd>`. Serve keeps its last 1 MiB of output
and replays it to every tab that attaches. A reload, or a later slide
naming the same session, shows what happened and takes input again. Serve
starts a session once, so every block naming it gives the same command,
directory, and prefill, within a Doc and across a page's brief and deck.

The feature is off by default, behind the file `terminal.enabled` in the
workdir. `present terminal on` and `off` create and remove it. It is a file
rather than a flag, so it works with serve up or down and needs no restart.
Serve reads it on every terminal request and checks it every 2 s. A file it
cannot read counts as off, and off kills every session.

The browser names a session and nothing else. The socket's query carries
the terminal size only. Serve reads the command, directory, and prefill
from the stored page's block of that name, and refuses a session the page
does not declare. Script injected into a page can at most attach to a
session the author declared; it cannot choose what runs. The socket checks
the Origin without consulting the Host, against DNS rebinding, and allows
loopback, `localhost`, and `present.this`. The last is named because d-man
proxies the upgrade and rewrites the Host on the way.

The routes exist in local mode only. A shared instance builds no session
manager and registers no terminal route. It accepts pages holding terminal
blocks and shares them as usual. Its shell marks the shared mode, and each
block shows a placeholder.

The browser side is xterm.js and its fit addon, vendored and pinned under
ADR-0022. A block attaches the first time the reader sees it, or on Run,
and reconnects with backoff after a drop.

## Consequences

- Escape leaves browser fullscreen, and a terminal program wants it too
  (Claude interrupts on it). A deck holding a terminal asks for the
  Keyboard Lock API in fullscreen, so a focused terminal keeps Escape. Only
  Chromium has it, and only in a secure context. Over
  `http://present.this`, and in Safari and Firefox, Escape still leaves
  fullscreen.
- A `ralph up` that rebuilds present restarts serve, which kills every
  session, mid-demo if one runs. Accepted for the first version.
- A session no tab has watched for 2 h is killed, so a forgotten demo does
  not run for days. A kill sends SIGHUP to every process group in the
  session and SIGKILL after a 2 s grace. A process that called setsid
  outlives it.
- A running session keeps the command it started with. An edited block
  changes what the next start runs, after Kill.
- Shell startup files run in every session. A hook that waits for a
  terminal reply xterm.js never sends stalls the session, so every session
  exports `PRESENT_TERMINAL=1` for such a hook to test.
- The seatbelt profile lets `present mcp` write the workdir, so the switch
  file is within its reach. No tool writes it; the CLI is the only writer.
- Accepted gaps: the browser code in `app.js` has no unit tests and is
  checked by hand in a browser. Kill is held per tab in `sessionStorage`,
  so another tab, or a new one, starts the session again on its next
  attach.

## Amendment (2026-10-25)

The Context and Consequences name a seatbelt profile as the mechanism that
confines `present mcp`. That profile (`recipes/present/present.sb`) is gone:
`present mcp` now runs under anthropics/sandbox-runtime (srt) through
`srt-run`, with `~/.config/srt/present.json` from the consuming repo. The
posture this decision relies on is unchanged: most of `$HOME` is denied, the
workdir is writable, and the process exits with its client. So the switch
file is still within `present mcp`'s reach, and the CLI is still its only
writer.

## Amendment (2026-10-25, sandboxed serve)

The Context says `present serve` runs unsandboxed, with the user's own
rights. That changed: serve now runs under srt through `srt-run`, with
`~/.config/srt/present-serve.json` from the present recipe, and those
settings grant no pseudo-terminal. A terminal session fails to start,
because openpty is denied. The switch file, the page-declared sessions, the
origin check and the local-only routes are unchanged. Turning sessions back
on is a new decision rather than a settings tweak. `allowPty` alone would
start the shell inside serve's confinement, with `$HOME` reads denied and no
egress beyond loopback, and that isn't the demo this ADR set out to enable.
