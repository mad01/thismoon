# toss-bin, safe rm replacement for macOS

Swift CLI that moves files to `~/.Trash/my-trash/YYYY-MM-DD/` instead of deleting them. The shell wires `rm` to `toss-bin --safe-mode` (via the toss-bin recipe), so everyday deletes become recoverable trash moves with a system-path deny-list in front.

## Module layout

```
tools/toss-bin/
  Sources/
    toss-bin/
      main.swift        Argument parsing, toss(), validatePaths(), main dispatch
      Utilities.swift   CLI helpers (arguments, sudo revert, stderr printing)
    TossBinCore/
      Safety.swift      PathSafety — the deny-list and validate(). Edit built-in protected paths here.
      Config.swift      TossConfig + ConfigLoader — machine-local deny-list additions (YAML via Yams)
      TempOnly.swift    TempOnly: the fixed temp roots and the --temp-only containment check
      TempOnlyMove.swift  TempOnlyMove: the --temp-only move by file descriptor; TrashName
      OperatingDoc.generated.swift  Codegenned from operating.md — never edit by hand
  Tests/TossBinTests/
    SafetyTests.swift   XCTest suite for PathSafety
    TempOnlyTests.swift XCTest suite for TempOnly (fresh scratch dirs only)
    TempOnlyMoveTests.swift  XCTest suite for TempOnlyMove (fresh scratch dirs, scratch trash)
  operating.md          Agent-facing runtime doc, embedded via codegen (`toss-bin docs`)
  scripts/gen-operating-doc.sh  operating.md -> OperatingDoc.generated.swift
  scripts/temp-only-race.sh     races a --temp-only run against a parent swap (run by ./test; skips, not fails, when no swap lands mid-run)
  Package.swift         swift-tools-version 5.9, macOS 10.15+
  Makefile              build / test / test-integration / install / gen-operating-doc
  test                  Shell suite: builds a fresh binary, exercises --validate paths
```

The monorepo's only Swift component. It sits outside the Go module entirely: no Go files, excluded from the Go CI matrix, built and tested on a macOS runner via its Makefile (see `.github/workflows/ci.yml`).

## How it works

Files land in `~/.Trash/my-trash/<YYYY-MM-DD>/`, so recovery is a Finder trip away. toss-bin only creates the dated directory when something actually gets trashed. Name conflicts get a counter appended (`file.1.txt`, `file.2.txt`, ...) rather than overwriting.

### Safe mode

`--safe-mode` runs every path through `PathSafety.validate()` before touching it. The deny-list blocks system roots (`/`, `/System`, `/usr`, ...), macOS metadata paths, and the home directory plus its standard folders (`~/Documents`, `~/Desktop`, ...). Files *inside* an exact-match entry still pass; a subtree entry blocks everything beneath it. `/usr/local` is exempt from the `/usr` tree block for Homebrew. The `rm` shell function always passes `--safe-mode`.

`~/.config/toss-bin/config.yaml` (override with `$TOSS_BIN_CONFIG`) can add machine-local protected paths on top of the built-ins — see Configuration. Extend-only by construction: `PathSafety.ExtraProtections` is additive and nothing in the config can unblock a built-in entry.

### Temp-only mode

`--temp-only` exists so an agent harness can allow exactly `toss-bin --temp-only ...` without a permission prompt. `main.swift` runs `TempOnly.locate()` plus the deny-list over every path and refuses the whole run (nothing trashed, exit 1, each offending path and the allowed roots on stderr) unless all pass. The roots are `/tmp`, `/var/tmp`, `~/.tmp` and their macOS real paths `/private/tmp`, `/private/var/tmp`; a root itself never passes. The check judges the real location: it expands a leading `~` against the account's home from the password database (not `$HOME`), resolves the parent chain with realpath(3) so `..` climbs from a symlink's target the way the kernel does, and keeps the final component as written so a link is judged where it lives. The flag counts anywhere before `--` and implies `--safe-mode`; `--dry-run` and `--validate` work with it.

The move then goes through `tossTempOnly()` and `TempOnlyMove`, not `toss()`. It walks the real directory the check returned from `/` with openat(O_NOFOLLOW | O_DIRECTORY), the root's own real path included, and renames the entry from that directory fd into the trash directory fd with renameatx_np(RENAME_EXCL). A component swapped for a symlink after the check is refused, a name clash counts up instead of overwriting, and a cross-volume entry (EXDEV) is refused rather than copied. The trash directory is reached the same way: the home (as Foundation reports it, so `CFFIXED_USER_HOME` still redirects tests) is the trusted start and is resolved once with realpath(3), then `.Trash`, `my-trash` and the dated directory are created with mkdirat and opened with openat(O_NOFOLLOW | O_DIRECTORY). A symlink or non-directory at any of the three refuses the run before anything moves. Plain `toss()` still creates the trash with `FileManager`.

### rm compatibility

- Unknown flags are silently ignored so the `rm` wrapper can pass normal `rm` flags through without errors.
- `-h`/`-v` act as help/version only when they are the sole argument — `rm -v file` means verbose remove.
- `.` and `..` are refused, same as `rm`.
- Symlinks are trashed as links: the link moves, the target is never touched. Safe mode validates a symlink by where the link lives, not what it points at.

## Configuration

Optional; toss-bin runs with the built-in deny-list when no file exists.

```yaml
# ~/.config/toss-bin/config.yaml
protected_paths:        # exact match — the path itself, contents still pass
  - /Volumes/backup
protected_trees:        # the path and everything under it
  - ~/code/archive
```

Entries take a leading `~`, trailing slashes are stripped, duplicates (including of built-ins) are harmless. Config-added trees get no `treeAllowList` exemptions — those carve-outs exist for built-in system trees only. A malformed config prints a warning to stderr and the run continues on built-ins alone, so a YAML typo never breaks `rm`. The config is read only when safe mode (including `--temp-only`) or `--validate` runs.

## Build / install / test

```bash
make build              # swift build --configuration=release
make test               # swift test (XCTest suite for TossBinCore)
make test-integration   # ./test — shell suite over --validate paths
make install            # install to ~/.local/bin/toss-bin + codesign resign
```

Installs to `~/.local/bin/` (not `~/code/bin/` like the Go tools): that is the path the shell `rm` wrapper has always resolved `toss-bin` from.

## Commands

Single command: `toss-bin [flags] <path> [...]`

| Flag | What it does |
|------|---------------|
| `--safe-mode` | Block trashing protected system paths and home directory |
| `--temp-only` | Refuse the whole run unless every path sits strictly inside `/tmp`, `/var/tmp` or `~/.tmp`; implies `--safe-mode` |
| `--dry-run` | Print what would happen without doing anything |
| `--validate` | Check whether paths would be blocked; no file operations |
| `-r`, `-R`, `--recursive` | Allow trashing directories |
| `-d` | Allow trashing empty directories |
| `-f` | Suppress errors for nonexistent files (matches `rm -f`) |
| `--help`, `-h` | Usage (`-h` only as the sole argument) |
| `--version`, `-v` | Version (`-v` only as the sole argument) |
| `docs` | Print the embedded operating doc (only as the sole argument, so a file named `docs` stays trashable) |

## Gotchas

- **Runtime debugging lives in `operating.md`** (embedded via codegen,
  printed by `toss-bin docs`): safe-mode block messages, codesign/quarantine
  kills, trash recovery paths, version-skew checks. Keep those facts there,
  not here. Swift has no `go:embed`, so `scripts/gen-operating-doc.sh`
  regenerates `Sources/TossBinCore/OperatingDoc.generated.swift` before
  every `make build`/`make test`; the generated file is committed, and an
  XCTest asserts the constant carries the doc's heading.
- **Run both test suites before touching the deny-list.** `make test` covers `PathSafety` at unit level; `make test-integration` builds a fresh binary and exercises every blocked and allowed path end to end.
- **Don't hand-bump `VERSION` in `main.swift`.** The `// x-release-please-version` annotation lets release-please's generic updater manage it (`extra-files` in `release-please-config.json`); there is no ldflags injection for Swift.
- **Silently-ignored unknown flags are deliberate.** They keep the `rm` wrapper transparent. Don't "fix" argument parsing to reject them.
- **Config extends, never weakens.** There is deliberately no way to unblock a built-in deny-list entry from config; loosening the default net is a code change in `Safety.swift`, not a config edit. The config file itself is machine wiring and ships from the consuming repo (docs/adr/0006) when a machine wants one.
- **The temp-only roots stay in code, never config.** An allow rule for `toss-bin --temp-only` is safe only because the binary refuses everything else, and config is a file an agent could edit. Don't add a config key or env var that widens them.
- **Temp-only containment is physical, not lexical.** Don't swap realpath(3) for `standardizingPath` or `URL.resolvingSymlinksInPath()`: a lexical `..` passes `/tmp/link/../x` when the link points outside, and `resolvingSymlinksInPath()` strips `/private`. The all-or-nothing refusal is deliberate too: a caller never gets a half-done cleanup.
- **The temp-only move never takes a path string.** `moveItem`, `rename(2)` or any path-based call resolves the parents again at move time, and a process that can write /tmp can swap one for a symlink between the check and the move (`scripts/temp-only-race.sh` reproduces it: the old path-string move trashed nearly every outside file). Keep the fd walk and renameatx_np, and never add a copy fallback for EXDEV. The same holds for the destination: opening the trash by path, or creating it with `createDirectory`, follows a link planted at `~/.Trash/my-trash` or the dated directory, and the allow rule then creates files wherever the link points.
- **The integration suite never trashes into the real `~/.Trash`.** Every `./test` case that really moves a file runs through `toss_scratch` (a scratch home via `CFFIXED_USER_HOME`). The `--validate` cases for the real `$HOME` deny-list call `$TOSS` directly, because the redirect moves the home the deny-list sees.
- **The rm wrapper lives in the recipe, not here.** `recipes/toss-bin/recipe.toml` ships `shell.functions.rm`; consuming machines get it through ralph.

## See also

- `README.md`: usage reference, full protected-path listing, develop loop.
- `recipes/toss-bin/`: build/install recipe plus the `rm` shell function.
