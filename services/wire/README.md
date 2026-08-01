# wire

A place for two sessions to talk. One agent opens a channel and gives you a
connection string; paste that into another agent and the two can talk to each
other.

Work often outgrows one session, usually one of two ways. You **hand off** —
start a refactor in one place, finish it somewhere else, or split a problem
between two agents who then each have to guess what the other did. Or you want
a **second session to test** what the first one built, without inheriting the
assumptions that produced the bug. Both need the same thing: the two sessions
have to be able to ask each other questions while the work is happening.

wire gives them a **channel**: a named conversation with an append-only
transcript. A read can block until the other side answers, so waiting for a
reply is one call, not a polling loop.

## How it works

- a **channel** is a named conversation. Opening one gives you a **connection
  string** like `wire://localhost:7432/refactor-auth`. That token is the entire
  join protocol — hand it over and the other session is in. No invites, no
  secrets. It works anywhere a channel is named: any command, any tool, the
  web page's URL.
- a **message** is one turn, signed with who sent it. Messages are immutable and
  numbered from 1. A message can also carry a **kind** (task, result, question,
  answer, ack), point at the message it answers with **reply_to**, and flag
  **reply_needed** when it expects an answer — which is how two agents keep an
  interleaved conversation straight without inventing their own conventions in
  prose.
- a **cursor** is the number of the last message you read. Ask for everything
  after it and you get only what's new. Every read also reports
  **awaiting_reply**: the messages still owed an answer.
- a channel can declare **conventions** when it's opened — the ground rules of
  the conversation, shown to anyone who joins, even mid-transcript.

Reading with a wait blocks until the next message lands, up to two minutes. If
nothing arrives you get an empty answer, not an error — ask again, or stop.

When the work is done, **close** the channel. That's terminal: no more messages,
and anyone blocked on a read wakes up instead of waiting for a reply that isn't
coming. The transcript stays readable.

## Install

It ships from the thismoon monorepo. `ralph up` builds the binary to
`~/code/bin/wire`, registers the `wire serve` agent with t-man, adds the
`wire.this` route, and wires the Claude Code tools.

To check the service is up:

```
t-man status wire
```

## Usage

```
$ wire open refactor-auth --topic "splitting the auth migration" --from planner
wire://localhost:7432/refactor-auth
channel refactor-auth · ch_79debcd3a329
paste that first line into the other session

$ wire post refactor-auth --from planner "schema is migrated, the handler is yours"
#1  planner

$ wire read refactor-auth --wait 60        # blocks until someone answers
#2  builder  Jul 28 11:41:53
    on it — will ping when the handler compiles
-- cursor 2

$ wire follow refactor-auth                # keep printing until it closes
$ wire close refactor-auth --note "shipped in #124"
refactor-auth closed  4 messages
```

The other session uses the connection string exactly as handed over:

```
$ wire read wire://localhost:7432/refactor-auth --wait 60
$ wire post wire://localhost:7432/refactor-auth --from builder "on it"
```

```bash
wire open [name] [--topic <text>] [--conventions <rules>]  # omit the name to have one generated
wire list [--all]                    # --all includes closed channels
wire connect <ref>                   # print the connection string, e.g. | pbcopy
wire post <ref> [message] [--kind <k>] [--reply-to <seq>] [--reply-needed]
wire read <ref> [--since <cursor>] [--wait <seconds>] [--limit <n>]
wire follow <ref>                    # print messages as they arrive
wire close <ref> [--note "<why>"]    # terminal
```

Every command takes `--from` (who you are); it defaults to `WIRE_FROM` or your
username. `<ref>` is a channel name, an id, or a connection string — whichever
you happen to have.

## MCP + web view

Mostly you'll use wire through Claude — it's the surface the whole thing is
built for. Ask one session to open a channel, paste the connection string it
gives you into another session, and they'll talk to each other. Claude calls
the tools below.

| Tool | Purpose |
|------|---------|
| `wire_open(name?, topic?, from, conventions?)` | Open a channel and get the connection string to pass on |
| `wire_post(channel, from, body, kind?, reply_to?, reply_needed?)` | Post a message, typed and correlated |
| `wire_read(channel, since?, wait?, limit?)` | Read after a cursor, optionally blocking; reports `awaiting_reply` |
| `wire_list(include_closed?)` | List channels, most recently active first |
| `wire_close(channel, note?)` | End the conversation and wake everyone waiting |

Open `http://wire.this/` (or `http://localhost:7432/`) to watch. The channel
list shows who's talking and the last thing said; clicking one opens the
transcript, which updates live as messages arrive and shows the channel's
connection string. The page is read-only — you post through Claude or the CLI.

## Where things live

- Channels: `~/.local/share/wire/channels.jsonl`
- Messages: `~/.local/share/wire/messages/<channel-id>.jsonl`
- Binary: `~/code/bin/wire`
- Web + API: `http://wire.this/` (or `http://localhost:7432/`)

Removing the service doesn't delete your transcripts.

## Develop

```bash
make test    # go test ./...  (hermetic: temp dirs, never touches your real store)
make build   # ./wire
```

Architecture, the data model, the wait semantics, and the HTTP API are in
[`CLAUDE.md`](CLAUDE.md).
