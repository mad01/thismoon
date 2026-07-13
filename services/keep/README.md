# keep

A place to write down what you learned about a system, pinned to the code that
proves it. When the code moves on, keep tells you which of those notes to look
at again.

Agents figure things out mid-session (how a service behaves, which approach was
a dead end, what a user prefers), and most of it evaporates when the session
ends. keep makes those findings persistent and keeps them honest: each one is a
one-sentence **assertion** tied to an **evidence pin**, a hashed line range in a
repo. Re-run the check later and any assertion whose pinned code changed turns
**stale**, so you know the note is due for another look instead of trusting it
blind.

## How it works

You write an assertion with at least one pin:

- the **assertion** is a single sentence, e.g. "events dedupes by content hash
  within a 5s window"
- a **pin** points at the evidence: a file and line range in a repo working
  tree, hashed when you write it

`keep check` re-hashes the pins. If the pinned code still matches, the assertion
stays fresh. If it changed, the assertion goes stale and keep names the pin that
moved. Stale is reversible: if the code changes back, or you re-check after the
lines settle, it goes fresh again.

When an assertion turns out to be wrong, you **retract** it with a note saying
why. A retracted assertion sticks around as a record of the withdrawn claim and
is never checked again.

## Install

It ships from the thismoon monorepo. `ralph up` builds the binary to
`~/code/bin/keep`, registers the `keep serve` agent with t-man, adds the
`keep.this` route, and wires the Claude Code tools.

To check the service is up:

```
t-man status keep
```

## Usage

```
$ keep assert \
    --kind code-behavior \
    --subject repo:mad01/thismoon/services/events \
    --statement "events dedupes by content hash within a 5s window" \
    --confidence verified \
    --session sess-0af3 \
    --pin ~/code/src/github.com/mad01/thismoon:services/events/internal/store/dedupe.go:42-58
018f2c...  fresh  events dedupes by content hash within a 5s window

$ keep list --subject repo:mad01/thismoon
018f2c...  code-behavior  fresh  events dedupes by content hash within a 5s window
018f19...  dead-end       fresh  in-memory index can't survive restart, needs the JSON store

$ keep check
checked 2  fresh 1  stale 1  flipped 1
018f2c...  stale  content changed (dedupe.go:42-58)
```

A stale line means the pinned code moved. Open it, decide whether the assertion
still holds, and re-assert with fresh pins if it does.

```bash
keep assert --kind <kind> --subject <key> --statement "<sentence>" \
            --confidence verified|derived|hint --session <id> \
            --pin <repo>:<file>:<start>-<end>    # repeatable, at least one
keep list [--subject <prefix>] [--kind <kind>] [--status fresh|stale|retracted]
keep get <id>                                    # full detail, all pins
keep check [id]                                  # re-hash one assertion, or all
keep retract <id> --note "<why it's wrong>"      # terminal withdrawal
```

## MCP + web view

Mostly you'll use keep through Claude. Ask it to record what it found ("keep
that events dedupes by content hash, pin the store file"), to look things up
("what do we know about the events service"), or to check whether prior
findings still hold. Claude calls the tools below.

| Tool | Purpose |
|------|---------|
| `keep_assert(kind, subject, statement, confidence, session, pins, …)` | Store an assertion with one or more evidence pins |
| `keep_query(subject?, kind?, status?)` | List assertions, newest first; `subject` is a prefix match |
| `keep_get(id)` | Get one assertion's full detail |
| `keep_check(id?)` | Re-hash pins and report what flipped; one assertion or all |
| `keep_retract(id, note)` | Withdraw an assertion with a counter-evidence note |

Open `http://keep.this/` (or `http://localhost:7431/`) for a read-only web view:
every assertion with its kind, status, and pins, filterable by subject, kind,
and status. You write and check assertions through Claude or the CLI, not the
web page.

## Where things live

- Assertions: `~/.local/share/keep/assertions.json`
- Binary: `~/code/bin/keep`
- Web + API: `http://keep.this/` (or `http://localhost:7431/`)

Removing the service doesn't delete your assertions file.

## Develop

```bash
make test    # go test ./...  (hermetic: temp dirs + a throwaway git repo, never touches your real store)
make build   # ./keep
```

Architecture, the data model, the state transitions, and the HTTP API are in
[`CLAUDE.md`](CLAUDE.md).
