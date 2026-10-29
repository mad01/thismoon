# ADR-0027: An access manifest beside the code is the source of sandbox policy

**Date:** 2026-10-25
**Status:** Accepted

**Scope:** every process the fleet sandboxes: the t-man services whose srt
settings ship here, and the consuming repo's MCP servers and install steps.
Adds `kit/access` and `tools/srtspec`, changes `recipes/speak/srt-run.sh`,
and makes `catalog validate` require the manifest. Builds on the
2026-10-25 amendments to ADR-0006.

## Context

Each sandboxed process ran under a hand-written srt settings file, the JSON
srt (anthropics/sandbox-runtime) turns into a seatbelt profile. The JSON is
what the kernel enforces, but it doesn't say why. A reviewer couldn't tell
the listening port from a loopback post, the binary's read from a store's,
or a missing host from a deliberately empty list. The fleet rules (no bare
`*` host, `~` read-denied, no write into the policy directory) were
re-checked after the fact by a shell test, one service at a time.

The same JSON also had to serve two audiences. What a process needs is a
property of its code and belongs beside it. What one machine adds (another
code root, a host only it talks to) is a property of that machine. With one
file per process, a machine-specific need meant forking the file into the
consuming repo, and a reader couldn't tell which entries were which.

Nothing answered the question a live machine asks: what does this process
get once srt-run merges `SRT_RUN_ALLOW_DOMAINS` in?

## Decision

The access manifest is the source of truth, and the srt settings file is
derived from it.

- **Where it lives.** A component declares `spec.access` in its
  `service-info.yaml`, the catalog descriptor that already sits beside its
  code. A recipe directory with no component uses a standalone
  `access.yaml` with the same block. With the YAML beside the code, a change
  to what a process does and to what it may reach lands in one diff. The
  catalog can also read and validate the posture without a second file
  format.
- **What it says.** A short, closed vocabulary: `sandbox` (srt, seatbelt,
  none, with a `reason` unless srt), `listen`, `local`, `egress`, `reads`,
  `writes` (with the `workdir` and `temp` symbols), `tls`, `deny`, and `extra`
  for raw srt keys merged last. A machine-specific host is written as the
  literal `via SRT_RUN_ALLOW_DOMAINS`, never a hostname. Decoding is strict.
- **How it becomes JSON.** `srtspec` renders from a fleet default: `~`
  read-denied, writes only to the temp roots, no hosts, and local binding
  only when the manifest listens or names a loopback peer. It checks the
  invariants on the result, `extra` included. Output is deterministic, and
  CI renders every manifest and fails on any diff. A settings file no
  manifest renders is an error.
- **What a machine adds.** `~/.config/srt/<label>.local.json` is merged by
  srt-run at launch with `SRT_RUN_ALLOW_DOMAINS`. It may only widen: add to
  the allow lists and turn on local binding or weaker network isolation.
  Any other key, an allow that undoes a deny, a write into the policy
  directories, or a `*` host refuses the launch. It fails closed, like srt.
- **How to see the result.** `srtspec explain <label>` prints the merged
  policy with the layer each entry came from, using srt-run's rules. A shared
  fixture set holds the accept and refuse cases, and both test suites run it.
- **Who must declare one.** `catalog validate` requires `spec.access` on a
  Component whose recipe launches a process, meaning it runs `t-man add` or
  `srt-run` outside a comment. An unsandboxed process says `sandbox: none`
  and why.

## Consequences

- The five settings files shipped here are renders now. Their content is
  unchanged; key order and layout changed once, to the renderer's canonical
  form.
- A reviewer reads YAML. The JSON diff in a PR is mechanical and CI keeps it
  honest.
- The expansion rules exist twice, as jq in srt-run and as Go in srtspec.
  srt-run has to stay a dependency-free shell script on every launch path.
  The shared fixtures are what keep the two in step; a rule change touches
  both and adds a case.
- The schema can express the consuming repo's MCP and install settings, so
  they can move to `access.yaml` and render with the same tool.
- `generate` relies on the layout convention: a component in a directory
  named `x` renders into `recipes/x/`. A recipe directory with another name
  needs renaming before its component can declare srt access.
- The catalog's detection reads recipe files by convention. An MCP server
  registered only in a consuming repo's server list, with no wrapper beside
  its recipe, isn't detected and isn't required to declare.

## Amendment (2026-10-26)

The expansion rules no longer exist twice. `~/code/bin/srt-run` is a shim
over `srtspec run`: the launcher merges the expansion layer and
`SRT_RUN_ALLOW_DOMAINS` through the same Go package explain reads, and the
jq copy went with the shell launcher and its test. The shared fixtures stay,
as the cases the launch and layers tests both run. The consequence above
that srt-run "has to stay a dependency-free shell script" is withdrawn: the
launch path depends on the srtspec binary, which its recipe builds in wave
0, and every service registration names `packages.srtspec` in `depends_on`
(ADR-0006, 2026-10-26 amendment). `srtspec verify` is the CI gate that keeps
the renders honest.

## Amendment (2026-10-29)

`catalog validate` left with the catalog service (MAD-508), so `srtspec
verify` now decides who must declare a manifest (MAD-509). It looks for
`t-man add` only, and only in the commands a recipe runs, never in a
description or a comment. `srt-run` is no longer a marker. This repo renders
srt settings only from its own manifests, so an `srt-run` exec whose settings
it renders already has one. An MCP wrapper whose manifest the consuming repo
owns, as humanizer's and present's do, is outside what this repo can verify.
