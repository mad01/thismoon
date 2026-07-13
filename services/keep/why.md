# why keep

## The problem

An agent session works out something non-obvious about a system (how a
service behaves, which approach was a dead end) and the finding evaporates
when the session ends. Writing it into docs or notes only trades one failure
for another: the code moves on and the note goes quietly wrong, with nothing
to signal that it is now misleading the next reader. keep stores each finding as a one-sentence
assertion pinned to the code that proves it, and `keep check` flips the
assertion stale the moment the pinned lines change — the note tells you when
it is due for another look instead of asking to be trusted blind.

## Why its own service

The value is the check loop, and the check loop needs a process that can read
the working trees the evidence pins point at: pin resolution and re-hashing
happen inside `keep serve`, the one process with a coherent view of those
files. Assertions are also mutated from three surfaces (the MCP tools, the
CLI, and `keep check` itself), so a single writer has to own the store to keep
them from racing on the file. A plain notes file or a generic store has no
notion of evidence and no way to detect that its contents have drifted from
the code they describe.

## Why this shape

Evidence is mandatory: every assertion carries at least one evidence
pin, a hashed line range in a repo working tree, and keep refuses to store an
assertion with zero pins because such an assertion could never go stale. The
server resolves and hashes pins at write time and rejects any it cannot
ground. The store is an append-only JSONL log: every mutation appends
one complete record, and load resolves the newest record per id: history is
never rewritten in place, and machine-scoped subjects are routed to a separate
local file at write time so they never leave the machine. The two
failure states are deliberately different: stale is reversible (the pinned
content changed; it flips back fresh if the content matches again), while
retract is terminal and requires a counter-evidence note. Checking is
conservative on purpose — an edit above a pin shifts its lines and flips the
assertion stale even though the code only moved, because stale means
"re-verify", not "wrong".

## Non-goals

keep is not a general note store: one sentence per assertion, at least one
evidence pin, nothing free-form. There is no delete and no un-retract; a
retracted assertion stays as the record of a withdrawn claim. The web page at
`keep.this` is read-only: writing, checking, and retracting go through the
MCP or the CLI. keep does not decide whether a stale assertion still holds or
re-pin it for you; a human or agent re-verifies and re-asserts. And v1 pins
code only: no other evidence kinds.
