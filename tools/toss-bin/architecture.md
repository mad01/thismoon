# toss-bin architecture

## Overview

toss-bin is a tool: one Swift binary installed to `~/.local/bin/toss-bin`,
invoked per delete and exiting immediately; nothing stays running and
nothing persists between runs. Its entire boundary is moving the paths it is given into
`~/.Trash/my-trash/`, plus a validate-only mode that touches nothing. It is
the monorepo's only Swift component and sits outside the Go module; CI
builds and tests it through its Makefile on a macOS runner.

## Structure

```
Sources/toss-bin/       executable target
  main.swift            argument parsing, toss(), validatePaths(), dispatch
  Utilities.swift       CLI helpers: arguments, sudo revert, print-to-stderr
Sources/TossBinCore/    library target
  Safety.swift          PathSafety — deny-list tables, ExtraProtections, validate()
  Config.swift          TossConfig + ConfigLoader — YAML config (Yams), path normalization
  TempOnly.swift        TempOnly: fixed temp roots, --temp-only containment check
  TempOnlyMove.swift    TempOnlyMove: the --temp-only move by file descriptor; TrashName
Tests/TossBinTests/     XCTest suites over PathSafety, TempOnly and TempOnlyMove
test                    shell suite: fresh build, --validate over the deny-list
scripts/temp-only-race.sh  races a --temp-only run against a parent swap
```

`TossBinCore` exists so the safety logic is importable: the XCTest suite
links the library directly, while the shell suite exercises the same
deny-list end to end through the built binary. `Utilities.swift` carries the
small CLI plumbing — `CLI.arguments`, `CLI.revertSudo()` (drops back to
`SUDO_UID` so a sudo'd invocation still uses the user's trash), and a
`print(..., to: .standardError)` overload.

## Data flow

`parseArguments` splits argv into short flags, long flags, and paths:
combined short flags expand (`-rf` becomes `-r -f`), `--` ends flag
parsing, and unknown flags are dropped so the `rm` wrapper can pass
anything through. Dispatch order: help/version (short forms only when they
are the sole argument), then `--validate` (prints `OK:`/`BLOCKED:` per path
and exits), then either `tossTempOnly()` under `--temp-only` or `toss()`.
Under `--temp-only` every path must pass `TempOnly.locate()` and the
deny-list before anything moves; one failure refuses the whole run. The
check resolves the parent chain with realpath(3) and compares the result
against roots fixed in code (`/tmp`, `/var/tmp`, `~/.tmp` and their
`/private` real paths). It returns a `Location`: the real parent directory
and the final name. `tossTempOnly()` never moves by path string, because
the kernel would resolve the parents again and a process that can write
/tmp could swap one for a symlink after the check. `TempOnlyMove` walks
the `Location` directory from `/` with openat(O_NOFOLLOW | O_DIRECTORY),
the root's own real path included (so `/tmp` is reached as
`/private/tmp`, never through its symlink), and refuses any component
that is a symlink at move time. It then renames the entry from that open
directory into the open trash directory with renameatx_np(RENAME_EXCL),
counting up the name on a clash, and refuses a cross-volume entry
(EXDEV) instead of copying it. The trash directory is opened the same way
(`TempOnlyMove.openTrash`): the home is resolved once with realpath(3) and
walked from `/`, then `.Trash`, `my-trash` and the dated directory are each
created with mkdirat and opened with openat(O_NOFOLLOW | O_DIRECTORY). A
symlink or non-directory at any of them refuses the run before the first
move, so a planted link cannot pick where the entries land.

For each path, `toss()` refuses `.` and `..`, runs `PathSafety.validate()`
when safe mode is on, and stats the path without following a final symlink.
So broken symlinks count as existing and a link to a directory moves as a
link. Directories require `-r` (or `-d`); `--dry-run` prints the would-be
destination and stops there. The dated trash directory is created on the
first real move; name conflicts append `.1`, `.2`, ... before the
extension (`TrashName`); the move itself is `FileManager.moveItem`. Any per-path error is
reported to stderr and the run exits 1 after processing all paths.

`PathSafety.validate()` checks three built-in tables: exact-match system
paths, blocked subtrees (with `/usr/local` exempted from `/usr`), and
exact-match home-directory paths. Paths inside a protected directory pass;
the protected path itself does not. When safe mode or `--validate` runs,
`ConfigLoader` reads `~/.config/toss-bin/config.yaml` (if present) into an
`ExtraProtections` value — additional exact and subtree entries checked
alongside the built-ins. The extras are additive only, config-added trees
get no exemptions, and a malformed config warns to stderr and yields the
empty extras so the built-in net stays intact.

## Storage

`~/.Trash/my-trash/<YYYY-MM-DD>/` — one directory per day, created lazily,
holding whatever was trashed that day under conflict-suffixed names. That
is the only thing toss-bin writes. `~/.config/toss-bin/config.yaml`
(override: `$TOSS_BIN_CONFIG`) is read, never written; there is no cache
and no state directory.

## Interfaces

A single CLI command: `toss-bin [flags] <path> [...]` with `--safe-mode`,
`--temp-only`, `--dry-run`, `--validate`, `-r`/`-R`/`--recursive`, `-d`, `-f`, and
help/version. The consuming shell interface is the `rm` function shipped by
`recipes/toss-bin`: it routes to `toss-bin --safe-mode "$@"` when the
binary is on `PATH` and falls back to real `rm` otherwise. The version
string is the `VERSION` constant in `main.swift`, managed by
release-please's generic updater (`// x-release-please-version`); there is
no ldflags injection.
