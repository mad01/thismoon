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
      OperatingDoc.generated.swift  Codegenned from operating.md — never edit by hand
  Tests/TossBinTests/
    SafetyTests.swift   XCTest suite for PathSafety
  operating.md          Agent-facing runtime doc, embedded via codegen (`toss-bin docs`)
  scripts/gen-operating-doc.sh  operating.md -> OperatingDoc.generated.swift
  Package.swift         swift-tools-version 5.9, macOS 10.15+
  Makefile              build / test / test-integration / install / gen-operating-doc
  test                  Shell suite: builds a fresh binary, exercises --validate paths
```

The monorepo's only Swift component. It sits outside the Go module entirely: no Go files, excluded from the Go CI matrix, built and tested on a macOS runner via its Makefile (see `.github/workflows/ci.yml`).

## How it works

Files land in `~/.Trash/my-trash/<YYYY-MM-DD>/`, so recovery is a Finder trip away. toss-bin only creates the dated directory when something actually gets trashed. Name conflicts get a counter appended (`file.1.txt`, `file.2.txt`, ...) rather than overwriting.

### Safe mode

`--safe-mode` runs every path through `PathSafety.validate()` before touching it. The deny-list blocks system roots (`/`, `/System`, `/usr`, ...), macOS metadata paths, and the home directory plus its standard folders (`~/Documents`, `~/Desktop`, ...). Files *inside* a protected directory still pass. `/usr/local` is exempt from the `/usr` tree block for Homebrew. The `rm` shell function always passes `--safe-mode`.

`~/.config/toss-bin/config.yaml` (override with `$TOSS_BIN_CONFIG`) can add machine-local protected paths on top of the built-ins — see Configuration. Extend-only by construction: `PathSafety.ExtraProtections` is additive and nothing in the config can unblock a built-in entry.

### rm compatibility

- Unknown flags are silently ignored so the `rm` wrapper can pass normal `rm` flags through without errors.
- `-h`/`-v` act as help/version only when they are the sole argument — `rm -v file` means verbose remove.
- `.` and `..` are refused, same as `rm`.
- Symlinks are trashed as links: the link moves, the target is never touched. Safe mode validates a symlink by where the link lives, not what it points at.

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
| `--dry-run` | Print what would happen without doing anything |
| `--validate` | Check whether paths would be blocked; no file operations |
| `-r`, `-R`, `--recursive` | Allow trashing directories |
| `-d` | Allow trashing empty directories |
| `-f` | Suppress errors for nonexistent files (matches `rm -f`) |
| `--help`, `-h` | Usage (`-h` only as the sole argument) |
| `--version`, `-v` | Version (`-v` only as the sole argument) |
| `docs` | Print the embedded operating doc (only as the sole argument, so a file named `docs` stays trashable) |

## Configuration

Optional; toss-bin runs with the built-in deny-list when no file exists.

```yaml
# ~/.config/toss-bin/config.yaml
protected_paths:        # exact match — the path itself, contents still pass
  - /Volumes/backup
protected_trees:        # the path and everything under it
  - ~/code/archive
```

Entries take a leading `~`, trailing slashes are stripped, duplicates (including of built-ins) are harmless. Config-added trees get no `treeAllowList` exemptions — those carve-outs exist for built-in system trees only. A malformed config prints a warning to stderr and the run continues on built-ins alone, so a YAML typo never breaks `rm`. The config is read only when safe mode or `--validate` runs.

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
- **The rm wrapper lives in the recipe, not here.** `recipes/toss-bin/recipe.toml` ships `shell.functions.rm`; consuming machines get it through ralph.

## See also

- `README.md`: usage reference, full protected-path listing, develop loop.
- `recipes/toss-bin/`: build/install recipe plus the `rm` shell function.
