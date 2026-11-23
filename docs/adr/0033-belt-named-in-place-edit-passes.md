# ADR-0033: belt lets a named in-place edit pass

**Date:** 2026-11-23
**Status:** Accepted

**Scope:** `tools/belt`, the file-discard guard.

## Context

file-discard judges a `sed -i` or `perl -i` with no backup the way it
judges an overwrite. Each file it rewrites must be in a temp or cache
folder, missing or empty, ignored, or committed. Judged like that, any
`sed -i` on a file with changes would deny, and that is the form agents
use most to edit a file they just changed.

## Decision

An in-place edit passes for every file it names as plain text, with no
expansion and no unquoted glob character, even when that file holds work
no commit has. An agent edits a file it names on purpose, and the Edit
tool is the way to make the same change with a diff. A variable the same
command sets to a literal before the edit counts as that name:
`f=notes.md; sed -i 's/a/b/' "$f"` names one file, as the shell runs it.
Only the files a glob, a `$(...)`, a feeder or `find -exec` hands the
edit, a variable the command does not set to a literal, and a loop
variable take the ladder. This is a false allow by design:
`sed -i 's/.*//' notes.md` passes and empties the file's lines.

## Consequences

The deny text for a rewrite asks to commit first, add a backup suffix, or
use the Edit tool. A wrong script over a named file is not caught. A named
edit of a build file still leaves a later build in the same command unread.
