# why toss-bin

## The problem

`rm` is irreversible. One mistyped path, one glob that expands wider than
intended, one `-rf` in the wrong directory, and the files are gone with no
recovery path. The shell is exactly where those mistakes happen: fast,
habitual typing with no confirmation step. An earlier setup used
`rcmdnk/trash` for this; it was replaced with a purpose-built tool that
matches `rm`'s semantics closely enough to sit behind an alias without
friction.

## Why its own tool

Replacing a shell habit only works if the replacement is transparent. The
binary has to accept `rm`'s flags without erroring (silently dropping the
ones it doesn't know), refuse `.` and `..` the way `rm` does, treat symlinks
as links rather than following them, and demand `-r` for directories. Any
visible difference and muscle memory fights the wrapper until it gets
removed. That contract is a program, not an alias trick.

Swift because the tool is macOS-only by definition (it targets the macOS
Trash) and Foundation's `FileManager` gives atomic same-volume moves and
symlink-aware stat calls without any runtime dependency. The result is one
small binary with nothing to install underneath it.

## Why this shape

Trashed files land in `~/.Trash/my-trash/<YYYY-MM-DD>/`: inside the real
Trash so Finder shows them, in a namespaced subtree so toss-bin never
collides with Finder's own entries, and grouped by day so "the file I
deleted on Tuesday" is a directory listing. Name conflicts append a counter
instead of overwriting — trash must never destroy what it holds. The dated
directory appears on first use; an empty day leaves no residue.

The deny-list lives in its own library target (`TossBinCore`) so safety
validation is unit-testable without running the binary; a shell suite then
exercises the same list end to end through `--validate`. Safe mode is an
explicit flag rather than the default: the shell `rm` function (shipped by
`recipes/toss-bin`) always passes `--safe-mode`, so the protection contract
lives in the wrapper while direct `toss-bin` invocations keep full control.
`--dry-run` and `--validate` exist so deny-list changes can be tested
against live paths without moving anything.

## Non-goals

No retention policy: toss-bin only ever adds to the trash; emptying it is
the user's call, via Finder or by deleting dated directories. No Linux or
Windows support — the tool is defined by the macOS Trash. Not a full `rm`
reimplementation: no `-i` prompting, no verbose mode; unknown flags are
tolerated, not implemented. And configuration only extends: the optional
config file adds machine-local protected paths, but the built-in deny-list
is code, and there is deliberately no config knob that weakens it —
loosening the default net is a code change that runs through both test
suites.
