# ADR-0021: mods are an additive, Claude-only interface layer

**Date:** 2026-10-05
**Status:** Accepted

**Scope:** `mods/`, the root `.claude-plugin/marketplace.json`, and how
consuming machines load them.

## Context

Claude Code can load a plugin whose hooks are TypeScript functions running
in-process. They draw a band above the prompt or open a pane, and they
observe events such as `turn.complete` and `tool.call`. This repo already
produces belt denies on the events service, worklog keys, work-on phase
markers, and present page ids. A session sees them only when it reads them
back through tools. A mod can keep them on screen. The same mechanism
could also carry policy, which is the risk. A module hook is skipped on a
throw or a timeout, so a guard written as a mod fails open. Codex and pi
never load one at all.

## Decision

Mods live at `mods/<name>/`, one complete plugin per directory, listed by the
root marketplace `thismoon` with a relative `source`. They are additive only.
No skill, hook, or service may require a mod, and belt stays the fail-closed
guard. A mod that probes a localhost service does so under a short budget and
stays silent when the service is down. Mods are not components: they have no
Makefile, no release-please entry, and no tag line of their own.
`plugin.json` carries the version and a merge to main is the deploy. Loading
is machine-private, as ADR-0006 already
says of recipes: the consuming repo registers the marketplace from the ralph
sources cache and enables mods through `enabledPlugins`. All four planned
mods are listed from the start, three of them as valid stubs, so a later
builder adds a mod without editing the marketplace file.

## Consequences

- Skills, hooks, and services are unchanged; a machine with no mods enabled
  behaves exactly as before.
- belt remains the only guard layer. A mod never denies, rewrites a tool
  call, or edits the transcript.
- Codex and pi see nothing of this layer. Anything they need stays a skill.
- CI runs the mods' `node --test` suites and checks every mod is in the
  marketplace. `claude plugin validate` stays a local gate: the runner has
  no Claude Code binary, so a hook the engine would refuse is caught on the
  author's machine, not in CI.
- The plugin API is early access and moves between releases. The generated
  `.claude-plugin/types/` beside a loaded mod is the authority; a release
  that renames an event breaks the band silently until someone runs it with
  `--debug`.
