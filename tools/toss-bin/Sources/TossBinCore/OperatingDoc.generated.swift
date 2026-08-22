// Generated from operating.md by scripts/gen-operating-doc.sh. Do not edit.

public let operatingDoc = ##"""
# operating toss-bin

toss-bin is a safe rm replacement for macOS. Instead of deleting, it moves
files to ~/.Trash/my-trash/YYYY-MM-DD/, so anything trashed is one Finder
trip (or one mv) from recovery. Shell setups commonly wire the rm command
to `toss-bin --safe-mode`, a deny-list check in front of every move.

## how it runs

One binary, usually installed at ~/.local/bin/toss-bin; no daemon, no
state beyond the trash directory it writes into. One command:
`toss-bin [flags] <path> ...`. toss-bin silently ignores unknown flags
by design, so an rm wrapper can pass normal rm flags through; do not
read a quiet run as "flag accepted". Short `-h` and `-v` count only as
the sole argument; long `--help` and `--version` work anywhere.

## where config lives

Optional deny-list additions come from ~/.config/toss-bin/config.yaml
(override the path with TOSS_BIN_CONFIG). Two keys: protected_paths
(exact match, the path itself) and protected_trees (the path and
everything under it). Config only extends the built-in list; nothing in
it can unblock a built-in entry. It is read only when --safe-mode or
--validate runs.

## failure modes

"BLOCKED: Protected system path": safe mode refused the path. Built-in
rules block system roots and the home directory plus its standard
folders. Most entries block the directory itself while files inside
still pass, but trees like /System, /usr, and /private are blocked
wholesale, with /usr/local exempt for Homebrew. `toss-bin --validate
<path>` prints the decision without touching anything.

"warning: ignoring ...config.yaml": the config file is malformed YAML.
The run continues on built-ins alone so rm keeps working, but the extra
protections in that file are not active until the YAML parses.

"killed" or "cannot be opened" at launch: macOS rejected the installed
binary's code signature or quarantine state. Clear and re-sign it
(`xattr -c` then `codesign --force --sign -` on the binary), or
reinstall from source with `make install`, which does both.

"is a directory": directories need -r, or -d when empty, same as rm.
"No such file or directory": the path does not exist; -f suppresses the
error like rm -f.

A "deleted" file is not gone: look under ~/.Trash/my-trash/<date>/.
Name conflicts get a counter (file.1.txt, file.2.txt), never an
overwrite. Nothing prunes the trash; emptying the macOS Trash empties
it. Under sudo the process reverts to the invoking user, so files still
land in that user's ~/.Trash.

## version skew

`toss-bin --version` prints a bare semver token like 1.2.0. There is no
JSON build metadata and no HTTP endpoint; no service exists. After an
upgrade, confirm the shell resolves the new binary: `command -v toss-bin`
should print ~/.local/bin/toss-bin, and --version should print the
expected release. A stale copy earlier on PATH shadows the installed one.

## first moves

1. `toss-bin --version` confirms the binary runs at all; codesign
   failures show up here first
2. `toss-bin --validate <path>` shows whether safe mode would block a path
3. `toss-bin --safe-mode --dry-run <path>` previews a real run end to end
4. `ls ~/.Trash/my-trash/` locates anything trashed recently
5. `cat ~/.config/toss-bin/config.yaml` shows machine-local deny
   additions; a missing file is fine, built-ins still apply
"""##
