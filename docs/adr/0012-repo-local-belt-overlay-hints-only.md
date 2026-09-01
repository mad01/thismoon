# ADR-0012: repo-local belt overlay is hints-only and can never deny

**Date:** 2026-08-31
**Status:** Accepted

**Scope:** `tools/belt`. Decided while building the commit-policy hint
(MAD-333, follow-up to PR #46).

## Context

The commit-policy hint made a repo property configurable only in the wrong
place. Whether a repo's main is committed to directly or via PR is decided
by the repo (thismoon went PR-only, dotfiles stays direct), but the
machine-rendered belt config (ADR-0010) is the only surface, so every
machine's rendering must enumerate other repos' policies, and a repo cannot
version its own policy next to the code it applies to.

The obvious fix, a repo-root config file belt reads, collides with what
belt is. A checked-out repo is untrusted input: any clone could carry a
config that disables `write-internal-names` or allowlists itself for main
pushes. And belt's own fail-closed rule cuts the other way: a
present-but-broken machine config denies every guarded call, so if a repo
file inherited that semantic, a malformed file in any cloned repo would
shut down the machine's tooling.

## Decision

belt reads an optional `.belt.yaml` at the root of the repo a tool call
touches, with two hard rules:

- **Hints only.** The overlay can configure hints about that repo. For
  commit-policy that is an `exclude` opt-out (or opt-in overriding the
  machine's `exclude_repos`), a replacement `protected_branches` set, and a
  `message` appended to the advice. The worst a malicious or wrong file can do is
  silence or reword a nudge. Guards never read repo-controlled files;
  loosening or tightening enforcement stays with the machine rendering.
  The loader lives in `internal/hint`, which `internal/guard` cannot import
  without a cycle, so the boundary is structural, not conventional.
- **Never deny.** A missing file is defaults, a foreign or empty file is a
  no-op, and a file that fails to parse degrades to one line of advisory
  text naming the file, the fail-visible channel a hint already has. The
  machine config's broken-config-denies-everything semantic explicitly does
  not apply to repo files.

Precedence: the machine's `hints.<id>.enabled` gate stays supreme (a
disabled hint never runs, so its overlay is never consulted). Within an
enabled hint, the repo file wins over the machine's repo lists for that
repo: local overrides global, in both directions, because both directions
are advisory.

## Consequences

- A repo declares its own commit policy in-tree and it travels with every
  clone; machine renderings keep only the machine-scoped lists.
- Repos can silence hints about themselves. Accepted: a hint suppressed by
  a repo file costs exactly what an ignored hint costs, and the guard tier
  is unaffected.
- Unknown hint ids and unknown keys in `.belt.yaml` are ignored rather
  than rejected, because a repo may target a newer belt than the machine runs,
  and nagging on every commit about keys the binary does not know would
  make upgrades noisy in the wrong direction. This inverts the machine
  config's validate-loudly rule on purpose: that rule protects the
  operator's beliefs about enforcement; a repo file carries no enforcement.
- Every hint that adopts the overlay must define which keys it reads;
  `commit-policy` is the only consumer at this decision's date.
