# ADR-0013: repo scoping is purpose-named, never a generic global exclude

**Date:** 2026-08-31
**Status:** Accepted

**Scope:** `tools/belt`. Decided after a four-reviewer critical panel on the
repo-scoping config design (MAD-333 follow-up work).

## Context

The same three repos appeared on multiple lists in the rendered config:
`guards.git-push-main.allow_repos` and `hints.commit-policy.exclude_repos`
both named the dotfiles repo and the single-writer store clones, with a
"keep the two lists in step" comment doing the synchronization. Two fixes
were on the table: a generic top-level `exclude_repos` that every
exemption-style check reads, and a rename of the guard key so one spelling
(`exclude_repos`) covers guards and hints alike.

The panel broke both. A generic global list merges a hint-silencer with a
guard-disarmer: the maintainer edits it to quiet an advisory in a
prototyping repo and thereby switches off git-push-main there (the guard
whose failure motivated this whole work item), with nothing in the config
signaling the tier crossing. And the one-key rename buys less than it
promises: rule-level keys (`git_identity[].repos`,
`commit_guards[].always_allow`) keep their own spellings regardless, while
`exclude_repos` on a deny guard reads inverted: "excluded" from the guard
means the push is *allowed*.

The panel also rejected deriving exemptions the way `internal_names`
derives blocked names: derivation is safe where an error announces itself
(an over-derived blocked name produces a visible deny) and unsafe where the
error is the absence of an event (a wrongly derived exemption is a guard
that silently never fires). Every concrete signal fails on the real
config: the `dotfiles-*` prefix covers repos that are name-exempt but
deliberately not push-exempt, and push-history derivation would have
exempted the very repo whose direct push started this.

## Decision

The duplicated lists collapse into one top-level key named for the workflow
fact they all encode: `direct_main_repos` (repos whose workflow is
direct-to-main). Exactly two checks read it, the two whose subject is that
workflow: the git-push-main guard (the push is allowed) and the
commit-policy hint (the branch + PR advice is silenced). No other check
consults it, so editing it cannot reach a check the fact is irrelevant to.

Everything else keeps its kind-scoped key: guards exempt repos with
`allow_repos`, hints opt out with `exclude_repos`, and validation rejects
each on the wrong kind. There is no alias and no deprecation cycle.

The criterion, for the next list someone wants to add: **reversibility**. A
wrong exemption from git-push-main or commit-policy is a visible,
recoverable push; a wrong exemption from write-internal-names is an
internal name permanently in a public repo, and from git-identity a wrong
email after push. Shared lists get edited for the cheap reason and silently
grant the expensive one, so a shared list may only reach checks whose
wrong-exemption cost is the cheap kind. Naming the list after the workflow
fact, not the mechanism, is what keeps its reach self-limiting.

Exemptions stay enumerated, never derived.

## Consequences

- The rendered config states each repo's direct-main workflow once.
  Per-check `allow_repos`/`exclude_repos` remain as escape hatches for the
  rare exemption that is not about the direct-main workflow.
- Repo identity for exclusion matching is canonical `host/owner/repo`,
  resolved from the origin remote, everywhere a working tree is reachable,
  including prefer-csl, whose index-side shard name is filesystem-derived
  and would let a worktree under a different parent directory dodge a
  path-based match. Only kof-assertions, which sees index-side search
  results with no path, matches the pattern tail (host dropped,
  case-insensitive), and that tail matching is confined to its own
  per-hint list.
- commit-guard keeps `commit_guards[].always_allow` and takes no toggle
  key; a `direct_main_repos` entry does not exempt it, since being a
  direct-main repo says nothing about work-hours discipline.
- A future shared list must name its workflow fact and pass the
  reversibility test before any check reads it.
