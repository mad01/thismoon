# ADR-0015: the internal-name guards enumerate public-bound repos

**Date:** 2026-09-08
**Status:** Accepted

**Scope:** `tools/belt`, the write-internal-names and publish-internal-names
guards (MAD-349).

## Context

Both internal-name guards decided "public" by host: a github.com remote
meant the repo was public-bound and internal names were denied there;
every other host was exempt. That rule was derived, never enumerated, and
it held while internal code lived on internal hosts.

It stops holding when the internal world is itself on github.com. A work
org hosted there is where every internal name lives by definition, and
the host rule denies them in their own repos: a push whose commit message
names a sibling service, a PR body, a file edit. The cure under the host
rule is an `allow_repos` org wildcard on every guard in every rendering,
which turns the exemption list into the real policy while the rule keeps
saying the opposite. Private personal repos on github.com have the same
problem in smaller print.

The actual policy is short: internal names may appear anywhere except in
the few repos whose content is public or headed there.

## Decision

A top-level `public_repos` list, purpose-named for the fact it states
(docs/adr/0013), enumerates the public-bound repos: exact
`host/owner/repo` or a trailing `/*` org wildcard. Exactly the two guards
whose subject is that fact read it. A repo on the list is guarded unless
the guard's own `allow_repos` carves it out; a repo not on it allows
internal names. Both guards, and both of the publish guard's events, read
the same list, so a repo is either public-bound everywhere or nowhere.

The host rule survives only as the fallback for a rendering that has no
`public_repos` key at all: there, every github.com repo stays
public-bound. The fallback exists for rollout skew, since the config and
the binary ship from different repos; a new binary meeting an old
rendering tightens rather than disarms. An empty present list guards
nothing and `belt doctor` says so.

Public-bound is enumerated, not derived, for the reason ADR-0013 gives:
the failure mode of a wrongly derived entry is silence. GitHub visibility
is not a usable signal either, since a private repo can be headed public
(this one was).

## Consequences

- The renderings list the public-bound repos and drop the exemptions the
  host rule forced on them. A new public repo must be added to the list;
  until it is, internal names are not blocked there. That is the trade the
  policy asks for: friction in the many private repos was constant, a
  missing public entry is a one-time omission with a visible fix.
- An org wildcard on `public_repos` plus per-guard `allow_repos` for its
  private members recovers the fail-closed stance for one org when wanted.
- The `external-text` hook matcher stops being the public/private line:
  with a list, any MCP server may be routed to it and the target decides.
  Without a list, only the github.com server belongs there.
- Adopting the list flipped every unnamed target from guarded to
  unguarded. A gist, `gh repo create` without an owner, `gh api` without a
  repo in its path, and an MCP call without owner/repo were public-bound
  under the host rule and matched nothing once a list existed. Targets that
  are public by nature are now guarded under both modes: gists, the `gists`
  API endpoints, repository creation without `--private` or `--internal`,
  and their MCP twins (`create_gist`, `update_gist`, and
  `create_repository` unless private). Every other unnamed `gh api` call
  stays allowed with a list, because the whole command text is scanned and
  guarding `gh api orgs/<org>/...` would re-deny the work-org calls the
  list exists to allow.
