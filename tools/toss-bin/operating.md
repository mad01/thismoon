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

`--temp-only` is the mode for agents cleaning up scratch files. The run
trashes nothing unless every path sits strictly inside /tmp, /var/tmp or
~/.tmp (on macOS /private/tmp and /private/var/tmp). The roots themselves
never pass. Each path is judged by its real location: `~`, `..` and
symlinks in the parent directories are resolved first, while a final
symlink is judged where the link lives. The flag counts anywhere before
`--`, implies --safe-mode, and no flag turns it off. The roots are fixed
in the binary, not config, because an allow rule for `toss-bin
--temp-only` rests on the binary and config is a file an agent can edit.
The move works on the real directory the check approved, opened one
component at a time without following symlinks, so a parent swapped for
a symlink mid-run is refused instead of followed; the trash directory
below the home is opened the same way. With that allow rule
in place, an agent can trash anything under the temp roots with no
prompt, backups a user parked in ~/.tmp included; it all stays
recoverable from ~/.Trash/my-trash/.

## where config lives

Optional deny-list additions come from ~/.config/toss-bin/config.yaml
(override the path with TOSS_BIN_CONFIG). Two keys: protected_paths
(exact match, the path itself) and protected_trees (the path and
everything under it). Config only extends the built-in list; nothing in
it can unblock a built-in entry. It is read only when --safe-mode,
--temp-only or --validate runs. No key changes the --temp-only roots.

## failure modes

"BLOCKED: Protected system path": safe mode refused the path. Built-in
rules block system roots and the home directory plus its standard
folders. Most entries block the directory itself while files inside
still pass, but trees like /System, /usr, and /private are blocked
wholesale, with /usr/local exempt for Homebrew. `toss-bin --validate
<path>` prints the decision without touching anything.

"REFUSED: <path>: --temp-only: ...": the path resolves outside the temp
roots, is a root itself, or has a parent that does not resolve; the
next line lists the allowed roots. Nothing in the run was trashed, the
paths that passed included. $TMPDIR (/var/folders/...) is not a root.

"REFUSED: <path>: <name>, a directory on the checked path, is now a
symlink": a directory was swapped for a symlink after the check, during
the run. That entry stays put; entries already moved stay in the trash.
"it sits on another volume than the trash": a volume is mounted under a
temp root, and --temp-only never falls back to a copy.

"REFUSED: cannot open the trash directory ...: <name>, on the way to the
trash, is a symlink": ~/.Trash, ~/.Trash/my-trash or the dated directory
is a symlink, which --temp-only never follows. Nothing was trashed.
Replace the link with a real directory (`ls -la ~/.Trash/my-trash`
shows it) and run again.

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
3. `toss-bin --safe-mode --dry-run <path>` previews a real run end to end;
   `toss-bin --temp-only --validate <path>` checks the temp-only rule
4. `ls ~/.Trash/my-trash/` locates anything trashed recently
5. `cat ~/.config/toss-bin/config.yaml` shows machine-local deny
   additions; a missing file is fine, built-ins still apply
