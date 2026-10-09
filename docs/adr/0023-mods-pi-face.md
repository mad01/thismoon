# ADR-0023: a mod may carry a pi face loaded from the same directory

**Date:** 2026-10-09
**Status:** Accepted

**Scope:** `mods/`, `mods/package.json`, and how the pi coding agent loads
mods. Amends the Claude-only clause of ADR-0021.

## Context

ADR-0021 made mods a Claude-only layer: Codex and pi saw nothing of them.
pi now runs on every machine beside Claude Code, with the same belt guards
and hints wired in through its extension API. The signals the mods keep on
screen (belt denials, humanizer findings on prose leaving the session) are
the same in both harnesses. Keeping them Claude-only would mean a
second copy of each mod's logic somewhere under the pi recipe.

The Claude face cannot be loaded by pi as it is. A Claude hook sits in a
chain and reads the decision `next(e)` hands back; pi fires separate
`tool_call` and `tool_result` events and has no chain. What the two share
is `lib/`: pure functions with no harness API.

pi loads extensions from a package: a directory with a `package.json`
whose `pi.extensions` lists the files, loaded in place with no copy. The
consuming machine declares the package in pi's `settings.json`, with
per-file include filters, the way it enables Claude mods through
`enabledPlugins`.

## Decision

A mod may add `pi/index.ts`, its pi face: `export default (pi:
ExtensionAPI) => { ... }`. The face owns pi's API and imports `lib/` only.
The Claude face stays `hooks/register.tsx` and is unchanged. `lib/`
imports neither API. A mod with no pi face is still a complete mod.

`mods/package.json` makes the `mods/` directory one pi package with
`pi.extensions: ["./*/pi/index.ts"]`. Adding a face never edits it, the
way the marketplace lists every mod up front. The consuming dotfiles
declares the package from the ralph sources cache and names the faces it
enables; a merge to main is the deploy, as for the Claude side.

The pi face follows the rules the Claude face already follows: additive,
fails open, denies only on the person's own Cancel, guards every dialog
and status line with `hasUI`. Where a Claude hook reads a decision off the
chain, the pi face reads a split event or the extension bus. The dotfiles
permission gate announces each belt denial as `belt:deny` for that reason.

## Consequences

- `mods/<name>/pi/index.ts` lives inside a Claude plugin directory. The
  Claude engine ignores it (`claude plugin validate` passes). The tsconfig
  the engine writes beside a loaded mod type-checks it against the Claude
  API, so the face carries `// @ts-nocheck` like every pi extension that
  relies on pi's runtime types.
- CI's `node --test mods/*/test/*.test.mjs` covers the pi faces through
  `test/pi.test.mjs` files driven by a fake `pi` object, and the manifest
  check parses `mods/package.json` with the other manifests.
- Codex still loads no mod; anything it needs stays a skill.
- Loading is machine-private as before (ADR-0006): which faces a machine
  enables is the consuming repo's settings, never this repo's.
