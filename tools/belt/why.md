# why belt

## The problem

An agent session executes tool calls faster than a human reviews them, and
the permission system judges the literal command, not its effect. `bash
cleanup.sh` looks harmless even when the script runs a deny-listed command; a
`git push origin main` on a guarded machine goes straight through; a
Write can drop an internal org name into a public repo long before git sees
it. The concrete trigger is recorded in this component's CLAUDE.md: a July
2026 session retrospective in which a session pushed straight to master and
tried to self-merge, and internal names reached `git commit` twice before the
pre-commit hook caught them. A git hook fires at the last possible moment;
by then the mistake is already made and only barely contained.

## Why its own tool

suspenders already guards the git side, but a git hook cannot see a tool
call. Only a Claude Code PreToolUse hook runs at the point where a session is
about to execute something. That requires a tool Claude Code can invoke with
the payload on stdin and a deny decision on stdout. belt is that layer:
suspenders holds up commits, belt holds up the session before anything
reaches git.

belt stays separate from suspenders because the two run in different
lifecycles, and each owns its own config. The write-internal-names guard
reads the `internal_names` section of belt's own file, plus the shared names
files that section lists under `include`, and never the suspenders config.
What the two share is the derivation: both build their name list through the
same `kit/repofind` discovery. They also share one names file when both list
it, and each points at that file from its own config (docs/adr/0016).
So identical inputs produce identical lists.

## Why this shape

A deny travels as data, never as a failing process. belt exits 0 and puts
the decision in `hookSpecificOutput.permissionDecision`, because a guard hook
must not break tool calls on its own bugs. Every deny reason carries a
`belt[<guard-id>]:` prefix, so a block is always attributable to the guard
that fired.

Config is read live at hook time: belt's own file, rendered per machine class
by the provisioning layer, plus the Claude settings deny list. That list is
shared with the permission system and sits behind its own switch. Every file
is optional and a missing one yields zero values. There is no machine-profile
concept at runtime. A guard that should not exist on a machine class is
simply absent (or disabled) in that class's rendered config.

Machine-private wiring stays out of this repo. The recipe here builds and
installs only, while hook registration and the config overlay with its
exclude paths live in the consuming repo's companion recipe (docs/adr/0006).

## Non-goals

belt is a guardrail against habit, not an adversary-proof sandbox. The
command parser is token-based rather than a full shell parser, so quoting
tricks, subshells, and reordered flags slip through by design; gaps get
patched as `extra_patterns` instead of growing a parser. It does not replace
the permission system or suspenders — it front-runs them, and the git-side
checks still stand behind it. It registers no hooks itself and enables no
guards by default; that wiring belongs to the consuming machine.
