# why belt

## The problem

An agent session executes tool calls faster than a human reviews them, and
the permission system judges the literal command, not its effect. `bash
cleanup.sh` looks harmless even when the script runs a deny-listed command; a
`git push origin main` on a work-profile machine goes straight through; a
Write can drop an internal org name into a public repo long before git sees
it. The concrete trigger is recorded in this component's CLAUDE.md: a July
2026 session retrospective in which a session pushed straight to master and
tried to self-merge, and internal names reached `git commit` twice before the
pre-commit hook caught them. A git hook fires at the last possible moment;
by then the mistake is already made and only barely contained.

## Why its own tool

suspenders already guards the git side, but a git hook cannot see a tool
call. Only a Claude Code PreToolUse hook runs at the point where a session is
about to execute something, and that requires a tool Claude Code can invoke
with the payload on stdin and a deny decision on stdout. belt is that layer:
suspenders holds up commits, belt holds up the session before anything
reaches git. It stays separate from suspenders because the two run in
different lifecycles, and each owns its own config: the write-internal-names
guard reads the `internal_names` section of belt's own file and no other.
What the two share is the derivation, not the file — both build their name
list through the same `kit/repofind` discovery, so identical configs produce
identical lists.

## Why this shape

A deny travels as data, never as a failing process: belt exits 0 and puts
the decision in `hookSpecificOutput.permissionDecision`, because a guard hook
must not break tool calls on its own bugs. Config is read live at hook time:
belt's own file, with the ralph machine profile as the one fallback and the
Claude settings deny list shared with the permission system behind its own
switch; every file is optional and a missing one yields zero values. Machine-private wiring
stays out of this repo: the recipe here builds and installs only, while hook
registration and the config overlay with its exclude paths live in the
consuming repo's companion recipe (docs/adr/0006). Every deny reason carries
a `belt[<guard-id>]:` prefix, so a block is always attributable to the guard
that fired.

## Non-goals

belt is a guardrail against habit, not an adversary-proof sandbox. The
command parser is token-based rather than a full shell parser, so quoting
tricks, subshells, and reordered flags slip through by design; gaps get
patched as `extra_patterns` instead of growing a parser. It does not replace
the permission system or suspenders — it front-runs them, and the git-side
checks still stand behind it. It registers no hooks itself and enables no
guards by default; that wiring belongs to the consuming machine.
