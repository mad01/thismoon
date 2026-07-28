# why wire

## The problem

Work outgrows a session, in two shapes.

**Handoff.** A refactor starts in one place and finishes in another, or two
agents get halves of the same problem. The receiving session inherits a summary
and nothing else, so the moment it hits an ambiguity the thread is dead: it
cannot ask the question that would have taken one line to answer.

**Testing from somewhere else.** Checking your own work from inside the session
that did it is the weakest kind of check — the same context that produced the
mistake is judging it. What you want is a second session that exercises the
change without inheriting the assumptions, and reports back what it actually
saw. That only works if the two can talk while it happens: the tester asks
which endpoint to hit, the builder answers, the tester says what broke.

Today each session is sealed. The only way one reaches another is a human
copying text between two terminals, and the only way to wait for an answer is
to keep asking.

## Why its own service

The value is the waiting. A session that can block until its peer replies
spends one call on the exchange; a session that can only poll spends a call per
attempt and still has to guess how long to sleep between them. Blocking needs a
process that holds the waiters and knows the moment a message lands, which a
shared file cannot do — a file gives you polling with extra steps. That same
process has to be the single writer anyway, since several sessions append to
the same conversation at once.

## Why this shape

A channel's **connection string is the entire join protocol**: one session
opens a channel and gets back a single token (`wire://localhost:7432/name`)
that it hands to the other, which is exactly how the handoff happens in
practice. It is one thing to copy rather than an endpoint plus a name plus an
explanation, and it works as the channel argument of every command and tool, so
the receiving session pastes it and is in. Secret tokens or invites would add a
step to the one interaction the service exists for, and the server is
loopback-only with no authentication on any other route, so a credential would
buy nothing real. The endpoint travels in the string anyway, which is what a
non-local wire would need later.

Messages are immutable and numbered from 1, which makes the **cursor a single
integer** — "everything after 7" needs no timestamps, no dedupe, and no
question about what a client has already seen. Sender is required: in a channel
several sessions write to, a message nobody signed cannot be answered.

**Closing is terminal, and it wakes every waiter.** The dangerous state in a
blocking system is a session waiting for a reply that will never come, so the
end of a conversation has to be a fact the store can announce rather than
something a reader infers from silence.

Storage is append-only JSONL, one file per channel: messages never change, so
file order is sequence order, and reading one conversation never scans another's
traffic.

## Non-goals

wire is not a chat app and not a queue: no threads, no reactions, no delivery
receipts, no retry semantics, no fan-out to subscribers who were not asked.
There is no reopen and no delete — a closed channel stays as its transcript.
The web page is read-only; opening, posting, and closing go through the MCP or
the CLI. Nothing is authenticated, because the server binds loopback only and
the conversations are one person's. And wire carries messages, nothing else: it
does not summarize a conversation, decide who should answer, or route work.
