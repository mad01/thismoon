# ADR-0016: the internal-name lists have one source both guard tools include

**Date:** 2026-09-09
**Status:** Accepted

**Scope:** `tools/belt`, `tools/suspenders`, `kit/internalnames`. Decided
after a five-reviewer panel on belt's guard structure and its consuming
configs.

## Context

belt and suspenders block the same internal names at different moments, and
both derive the blocked set the same way: repos discovered under
`workspace_dirs` plus `blocked_words`, minus `allowlist`. ADR-0010 made each
tool read only its own config and said the two name sections would stay in
step through "shared code and shared shape", with the provisioning layer
rendering one authored block into both files.

The render step was never built. Instead the consuming repos carried the
block four times: belt personal, belt work, suspenders personal, suspenders
work, each with a comment asking the editor to keep the others identical.
Measured at this decision, the four allowlists held 70, 46, 67, and 49
entries, every difference one-directional, the signature of editing one
file and forgetting the rest. Nothing checked, because nothing could: the
copies live in two repos, and any one machine sees only its own class.

Two shapes were on the table. A provision-time render (the ADR-0010 plan)
keeps both tools standalone but turns the installed configs into generated
files rather than symlinks to versioned ones, adds a render step to debug,
and needs marker-line templating because the provisioning scripts have no
YAML library. A read-time shared file has none of that, but it must not
become the cross-read ADR-0010 banned: the maintainer's constraint for this
decision was that belt and suspenders never read each other's config.

## Decision

The name lists move to a purpose-built file set, `~/.config/internal-names/
*.yaml`, that is neither tool's config. A names file carries exactly three
keys, `blocked_words`, `allowlist`, and `allow_phrases`, and nothing else.
Each tool names the files it reads in its own config: belt under
`internal_names.include`, suspenders under `guard.include`. The lists in a
named file append to the section's own lists; `workspace_dirs` stays in each
tool's rendering, because the directories legitimately differ per tool and
per machine class.

The read is strict and fail-closed in the same way the tool's own config is.
An unknown key in a names file is an error, since a misspelled key would
parse cleanly and guard nothing. A file the config names but that is missing
or does not parse is a config error: belt denies every guarded call naming
the file, suspenders fails to load. An absent `include` key means no
includes, not a fallback. Both doctors print each named file with its status
and counts.

One parser, `kit/internalnames`, reads the file for both tools, so the
format has one implementation and ADR-0010's "shared code" clause holds.
suspenders gains `allow_phrases` in the same change so every reader honors
every key in the shared file.

ADR-0010's rule stands: neither tool reads the other's config, and neither
file is a fallback for the other. What changes is the count of explicit,
non-own surfaces belt reads: the Claude settings deny list and now the names
files it lists, each named in its config, each reported by `belt doctor`.

## Consequences

- The renderings carry `workspace_dirs` and `include` and nothing else in
  their name sections. The consuming repo installs `common.yaml` on every
  machine class; a work overlay adds a second file and one include line when
  a work-only name appears.
- Rollout order matters. The binary must reach a machine before the
  rendering that drops its inline lists: an older belt ignores `include`,
  and the result is an empty name set, which `belt doctor` reports as a
  warning rather than a deny. Merging the binary first and running the
  provisioning tool before the config change removes the window.
- Minimum length and wildcard expansion still differ between the two
  derivations (the MAD-328 follow-up); the shared file removes the drift in
  inputs, not in matching semantics.
- A names file is a third place a guard's behavior comes from. Both doctors
  show it, both `config` commands show it, and a missing file is loud.
