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

**Requirements:** macOS 10.15 or later, Swift toolchain on `$PATH`.

## Usage

```sh
toss-bin [flags] <path> [...]
```

### Flags

| Flag | Description |
|---|---|
| `--safe-mode` | Block trashing protected system paths and home directory |
| `--dry-run` | Print what would happen without doing anything |
| `--validate` | Check whether paths would be blocked; no file operations |
| `-r`, `-R`, `--recursive` | Allow trashing directories |
| `-d` | Allow trashing empty directories |
| `-f` | Suppress errors for nonexistent files (matches `rm -f` behavior) |
| `--help`, `-h` | Show usage (`-h` only when it is the sole argument) |
| `--version`, `-v` | Show version (`-v` only when it is the sole argument) |

Combined short flags work: `-rf` expands to `-r -f`. Unknown flags are silently ignored so the `rm` function passes through normal `rm` flags without errors. That includes `-h` and `-v` when mixed with paths — `rm -v file` means verbose remove, so the short forms only act as help/version on their own.

### rm-compatible behavior

- `.` and `..` are refused, same as `rm` — trashing them would move the shell's working directory out from under it.
- Symlinks are trashed as links: the link itself moves to trash, the target is never touched. This includes broken symlinks, and a symlink to a directory needs no `-r`.
- Safe-mode validates a symlink by where the link lives, not what it points at. A link in `~/code` pointing at `~/Documents` is trashable (only the link moves); the deny-listed paths that are themselves symlinks (`/var`, `/etc`, `/tmp`) stay blocked by their own path.

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
```

### Protected paths

When `--safe-mode` is active, the following paths are blocked. Files *inside* a protected directory are still allowed (e.g. `~/Documents/myfile.txt` passes).

**System paths (exact match — blocked):**
`/`, `/Library`, `/Applications`, `/Users`, `/Volumes`, `/etc`, `/var`, `/tmp`, `/opt`, `/Developer`

**System subtrees (entire tree blocked):**
`/System`, `/bin`, `/sbin`, `/usr`, `/private`, `/cores`, and several macOS metadata paths (`.Spotlight-V100`, `.fseventsd`, etc.)

**Exempt from tree block:**
`/usr/local` (Homebrew) is allowed even though `/usr` is blocked.

**Home directory paths (exact match — blocked):**
`~` (home itself), `~/.Trash`, `~/Library`, `~/Documents`, `~/Desktop`, `~/Downloads`, `~/Pictures`, `~/Music`, `~/Movies`, `~/Applications`, `~/Public`

## Configuration

Optional. Without a config file toss-bin uses its built-in deny-list; with one, `~/.config/toss-bin/config.yaml` adds machine-local protected paths on top:

```yaml
protected_paths:        # exact match — the path itself is blocked, contents still pass
  - /Volumes/backup
protected_trees:        # the path and everything under it is blocked
  - ~/code/archive
```

The config can only extend the deny-list, never shrink it: there is no way to unblock a built-in path from config. Entries accept a leading `~`, trailing slashes are ignored, and listing a path that is already built in is harmless. Additions apply whenever `--safe-mode` (or `--validate`) is active. A malformed config prints a warning and the run continues with the built-in list, so a YAML typo never breaks `rm`. Set `$TOSS_BIN_CONFIG` to read a different file.

## Where things live

- `~/.Trash/my-trash/<YYYY-MM-DD>/`: trashed files, one directory per day, created on first use.
- `~/.local/bin/toss-bin`: the installed binary.
- `~/.config/toss-bin/config.yaml`: optional deny-list additions (read, never written).

## Develop

**Key files:**

| File | What it does |
|---|---|
| `Sources/TossBinCore/Safety.swift` | `PathSafety` — the deny-list and `validate()` function. Edit built-in protected paths here. |
| `Sources/TossBinCore/Config.swift` | `TossConfig` + `ConfigLoader` — the optional YAML config with deny-list additions. |
| `Sources/toss-bin/main.swift` | Argument parsing, `toss()`, `validatePaths()`, and the main dispatch. |
| `Sources/toss-bin/Utilities.swift` | CLI helpers: arguments, sudo revert, stderr printing. |
| `Tests/TossBinTests/SafetyTests.swift` | XCTest suite for `PathSafety`. |
| `Package.swift` | swift-tools-version 5.9, macOS 10.15+. |
| `test` | Shell test suite that runs `--validate` against known-blocked and known-allowed paths. |

**Run the tests before touching the deny-list** — the shell suite builds a fresh binary and exercises every blocked and allowed path:

```sh
make test               # XCTest unit suite
make test-integration   # shell suite (./test)
```

**Change-and-see loop:**

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
