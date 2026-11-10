# toss-bin

A safe `rm` replacement for macOS that moves files to `~/.Trash/my-trash/YYYY-MM-DD/` instead of deleting them.

## How it works

Files end up in the macOS Trash folder under a dated subdirectory, so recovery is straightforward. The trash directory only gets created when you actually trash something. Name conflicts get a counter appended (`file.1.txt`, `file.2.txt`, ...) rather than overwriting.

With the toss-bin recipe applied, the shell function `rm` calls `toss-bin --safe-mode` when the binary is on `PATH` and falls back to real `rm` otherwise. The `--safe-mode` flag keeps the system-path deny-list active for every shell delete.

## Install

```sh
make install
```

Builds a release binary (`swift build --configuration=release`) and installs it to `~/.local/bin/toss-bin`. Make sure `~/.local/bin` is in your `PATH`.

Fleet machines get it through ralph: the `recipes/toss-bin` recipe builds from the thismoon sources cache, installs the binary, and defines the `rm` shell function.

Requires macOS 10.15 or later and a Swift toolchain on `$PATH`.

## Usage

```sh
toss-bin [flags] <path> [...]
```

### Flags

| Flag | Description |
|---|---|
| `--safe-mode` | Block trashing protected system paths and home directory |
| `--temp-only` | Refuse the whole run unless every path sits inside `/tmp`, `/var/tmp` or `~/.tmp`; implies `--safe-mode` |
| `--dry-run` | Print what would happen without doing anything |
| `--validate` | Check whether paths would be blocked; no file operations |
| `-r`, `-R`, `--recursive` | Allow trashing directories |
| `-d` | Allow trashing empty directories |
| `-f` | Suppress errors for nonexistent files (matches `rm -f` behavior) |
| `--help`, `-h` | Show usage (`-h` only when it is the sole argument) |
| `--version`, `-v` | Show version (`-v` only when it is the sole argument) |

Combined short flags work: `-rf` expands to `-r -f`. Unknown flags are silently ignored so the `rm` function passes through normal `rm` flags without errors. That includes `-h` and `-v` when mixed with paths: `rm -v file` means verbose remove, so the short forms only act as help/version on their own.

### rm-compatible behavior

- `.` and `..` are refused, same as `rm`. Trashing them would move the shell's working directory out from under it.
- Symlinks are trashed as links: the link itself moves to trash, the target is never touched. This includes broken symlinks, and a symlink to a directory needs no `-r`.
- Safe-mode validates a symlink by where the link lives, not what it points at. A link in `~/code` pointing at `~/Documents` is trashable (only the link moves); the deny-listed paths that are themselves symlinks (`/var`, `/etc`, `/tmp`) stay blocked by their own path.

### Temp-only mode

`--temp-only` lets an agent clean up its own scratch files without a permission prompt. The run trashes nothing, exits 1, and names each offending path and the allowed roots unless every path sits strictly inside one of the built-in temp roots:

- `/tmp` (on macOS, `/private/tmp`)
- `/var/tmp` (on macOS, `/private/var/tmp`)
- `~/.tmp`

The roots themselves are never valid targets. A path is judged by its real location: toss-bin expands a leading `~` and resolves `..` and symlinks in every parent directory before comparing, so `/tmp/link-to-home/file` and `/tmp/../Users/x` are refused. The final component follows the usual symlink rule: a link is trashed as a link and judged by where it lives. `$TMPDIR` (`/var/folders/...` on macOS) is not a root.

The move works on the real directory the check approved, not on the path as typed: toss-bin opens it one directory at a time without following symlinks and renames the entry from there. Another process that swaps a parent directory for a symlink during the run gets that entry refused, not redirected. An entry on another volume than the trash is refused too; temp-only mode never copies. The trash is reached the same way: below your home, `.Trash`, `my-trash` and the dated directory must be real directories, so a symlink planted at any of them refuses the run before anything moves.

The flag counts wherever it appears before `--`, implies `--safe-mode`, and nothing switches it off. Combine it with `--dry-run` or `--validate` to check first.

The roots are fixed in the binary and no config key changes them. An allow rule for this one invocation is safe only because the binary refuses everything else, and a config file is something an agent could edit. For Claude Code, the rule is:

```json
{ "permissions": { "allow": ["Bash(toss-bin --temp-only *)"] } }
```

With that rule in place, an agent can trash anything under the temp roots with no prompt, including backups you parked in `~/.tmp`. All of it stays recoverable from `~/.Trash/my-trash/`.

### Examples

```sh
# Trash a file
toss-bin somefile.txt

# Preview without doing it
toss-bin --dry-run --safe-mode somefile.txt

# Check if a path would be blocked
toss-bin --validate /System ~/Documents /tmp/safe.txt

# Trash a directory
toss-bin -r ./old-build

# Trash scratch files, refusing anything outside the temp roots
toss-bin --temp-only -rf /tmp/build-cache ~/.tmp/notes.txt
```

### Protected paths

When `--safe-mode` is active, the following paths are blocked. Files *inside* an exact-match entry are still allowed (e.g. `~/Documents/myfile.txt` passes); a subtree entry blocks everything beneath it.

System paths (blocked on exact match):
`/`, `/Library`, `/Applications`, `/Users`, `/Volumes`, `/etc`, `/var`, `/tmp`, `/opt`, `/Developer`

System subtrees (entire tree blocked):
`/System`, `/bin`, `/sbin`, `/usr`, `/private`, `/cores`, and several macOS metadata paths (`.Spotlight-V100`, `.fseventsd`, etc.)

Exempt from tree block:
`/usr/local` (Homebrew) is allowed even though `/usr` is blocked.

Home directory paths (blocked on exact match):
`~` (home itself), `~/.Trash`, `~/Library`, `~/Documents`, `~/Desktop`, `~/Downloads`, `~/Pictures`, `~/Music`, `~/Movies`, `~/Applications`, `~/Public`

## Configuration

Optional. Without a config file toss-bin uses its built-in deny-list; with one, `~/.config/toss-bin/config.yaml` adds machine-local protected paths on top:

```yaml
protected_paths:        # exact match — the path itself is blocked, contents still pass
  - /Volumes/backup
protected_trees:        # the path and everything under it is blocked
  - ~/code/archive
```

The config can only extend the deny-list, never shrink it: there is no way to unblock a built-in path from config. Entries accept a leading `~`, trailing slashes are ignored, and listing a path that is already built in is harmless. Additions apply whenever `--safe-mode`, `--temp-only` or `--validate` is active. No config key changes the `--temp-only` roots. A malformed config prints a warning and the run continues with the built-in list, so a YAML typo never breaks `rm`. Set `$TOSS_BIN_CONFIG` to read a different file.

## Where things live

- `~/.Trash/my-trash/<YYYY-MM-DD>/`: trashed files, one directory per day, created on first use.
- `~/.local/bin/toss-bin`: the installed binary.
- `~/.config/toss-bin/config.yaml`: optional deny-list additions (read, never written).

## Develop

Key files:

| File | What it does |
|---|---|
| `Sources/TossBinCore/Safety.swift` | `PathSafety`: the deny-list and `validate()` function. Edit built-in protected paths here. |
| `Sources/TossBinCore/Config.swift` | `TossConfig` + `ConfigLoader`: the optional YAML config with deny-list additions. |
| `Sources/TossBinCore/TempOnly.swift` | `TempOnly`: the fixed temp roots and the `--temp-only` containment check. |
| `Sources/TossBinCore/TempOnlyMove.swift` | `TempOnlyMove`: the `--temp-only` move by file descriptor, plus `TrashName` for clash names. |
| `Sources/toss-bin/main.swift` | Argument parsing, `toss()`, `validatePaths()`, and the main dispatch. |
| `Sources/toss-bin/Utilities.swift` | CLI helpers: arguments, sudo revert, stderr printing. |
| `Tests/TossBinTests/SafetyTests.swift` | XCTest suite for `PathSafety`. |
| `Tests/TossBinTests/TempOnlyTests.swift` | XCTest suite for `TempOnly`, run in fresh scratch directories. |
| `Tests/TossBinTests/TempOnlyMoveTests.swift` | XCTest suite for `TempOnlyMove`, with a scratch home and its trash. |
| `Package.swift` | swift-tools-version 5.9, macOS 10.15+. |
| `test` | Shell test suite that runs `--validate` against known-blocked and known-allowed paths. |
| `scripts/temp-only-race.sh` | Races a `--temp-only` run against a parent directory swapped for a symlink; `test` runs it, and a run where no swap lands mid-run is a skip, not a failure. |

**Run the tests before touching the deny-list.** The shell suite builds a fresh binary and exercises every blocked and allowed path:

```sh
make test               # XCTest unit suite
make test-integration   # shell suite (./test)
```

Change-and-see loop:

```sh
# 1. Edit Sources/TossBinCore/Safety.swift (deny-list) or Sources/toss-bin/main.swift
# 2. Run both test suites
make test test-integration

# 3. Build and install
make install

# 4. Try it live
toss-bin --validate /path/you/care/about
toss-bin --dry-run --safe-mode /tmp/testfile.txt
```

## Docs

- [architecture](architecture.md): internal structure and data flow
- [operating](operating.md): runtime behavior, failure modes, first moves
- [why](why.md): why this component exists
- [config](config.md): configuration reference
