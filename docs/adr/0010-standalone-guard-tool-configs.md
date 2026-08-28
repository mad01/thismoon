# ADR-0010: guard tools read only their own config

**Date:** 2026-08-28
**Status:** Accepted

**Scope:** `tools/belt`, `tools/suspenders`, and any future guard tool.
Decided after a code audit of every exemption mechanism in both tools.

## Context

belt and suspenders guard the same risk at different moments: belt denies an
agent's tool call before it runs, suspenders blocks a commit. Both derive a
blocked-name set from local checkouts via `kit/repofind`, and their name
config sections share one shape.

Until this decision, belt also read two configs it does not own. When its
own `internal_names` section was absent, it loaded the `guard:` section of
the suspenders config as a fallback, and when the section was present it
replaced that fallback wholesale, so a partial section (say, only
`allow_phrases`) silently blanked the rest of the name set while suspenders
kept blocking at commit time. The read was one-directional: suspenders never
saw belt-only keys, so an exemption written on the belt side did nothing at
commit time, and the shipped example config even described the belt section
as feeding the suspenders guard, which it never did. The coupling promised a
unification it could not deliver — the two tools' name derivations had
already drifted (case handling, minimum length, wildcard expansion) while
the shared-file story suggested they agreed. belt's second foreign read, the
Claude settings deny lists, was unconditional and undocumented as a config
surface of its own.

## Decision

Guard tools are config-standalone. belt and suspenders each read exactly one
guard config: their own file. Neither file is a fallback for the other, in
either direction, and an absent belt `internal_names` section means an empty
name set — honest emptiness over a hidden read.

Reads of non-guard platform surfaces are the narrow exception, and every one
must be explicit: named in the tool's `Paths`, switchable where a switch
makes sense, and reported by `doctor`. belt has exactly two — the ralph
machine config as a profiles fallback, and the Claude settings files
(`permissions.deny` Bash entries for `script-deny-list`), now behind the
`claude_settings.enabled` key. That key defaults to true so the script guard
keeps enforcing the shared deny list on machines that never wrote it;
setting it false stops belt opening the Claude settings at all, leaving the
guard with `extra_patterns` only. Nothing else may be read: a new foreign
surface needs a new decision here.

Agreement between the two name guards lives in shared code and shared
shape, not shared files: both call the same `kit/repofind` derivation, and
the config sections stay shape-compatible so the provisioning layer (the
consuming repo's recipes, ADR-0006) can render one authored block into both
files. Single-point authoring is a provision-time concern, not a read-time
one.

## Consequences

An exemption now has to be present in both tools' configs to cover both
moments; the divergence failure mode is a blocked commit, never a leak, and
templating both files from one block removes the double bookkeeping. A
machine that relied on the old fallback loses its belt name set until its
belt config carries `internal_names` — `belt doctor` states plainly that
the guard has nothing to match. Removing the fallback is a breaking config
change for belt, released as a major bump. What stays open, tracked as
follow-up to the same audit: moving the derivation and matching semantics
both tools duplicate into one shared package so their behavior cannot drift
apart again.
