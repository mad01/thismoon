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
Tests/TossBinTests/     XCTest suite over PathSafety
test                    shell suite: fresh build, --validate over the deny-list
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
and exits), then `toss()`.

For each path, `toss()` refuses `.` and `..`, runs `PathSafety.validate()`
when safe mode is on, and stats the path without following a final symlink —
so broken symlinks count as existing and a link to a directory moves as a
link. Directories require `-r` (or `-d`); `--dry-run` prints the would-be
destination and stops there. The dated trash directory is created on the
first real move; name conflicts append `.1`, `.2`, ... before the
extension; the move itself is `FileManager.moveItem`. Any per-path error is
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
`--dry-run`, `--validate`, `-r`/`-R`/`--recursive`, `-d`, `-f`, and
help/version. The consuming shell interface is the `rm` function shipped by
`recipes/toss-bin`: it routes to `toss-bin --safe-mode "$@"` when the
binary is on `PATH` and falls back to real `rm` otherwise. The version
string is the `VERSION` constant in `main.swift`, managed by
release-please's generic updater (`// x-release-please-version`); there is
no ldflags injection.
