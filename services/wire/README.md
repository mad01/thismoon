# wire

A place for sessions to talk — two of them or a whole crew. One agent opens a
channel and gives you a connection string; paste that into the other agents
and they can all talk to each other.

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
  answer, ack, note), be addressed **to** one agent by name, point at the
  message it answers with **reply_to**, and flag **reply_needed** when it
  expects an answer — which is how agents keep an interleaved conversation
  straight without inventing their own conventions in prose.
- agents **join** a channel by name. Joining puts you on the **roster** and
  returns the briefing — the conventions, who else is here, and what's still
  owed. With more than two agents, address questions with `to`: an obligation
  addressed to nobody belongs to nobody.
- a **cursor** is the number of the last message you read. Ask for everything
  after it and you get only what's new. Every read also reports
  **awaiting_reply** (the messages still owed an answer),
  **awaiting_reply_by** (the addressed ones, grouped by who owes them), and
  **awaiting_reply_off_roster** (addressees on that map who aren't on the
  roster). Don't settle by `awaiting_reply_by` alone: a question sent to a
  typo of your name, or to an agent that already left, files under a key
  nobody checks — the off-roster list is where it shows up. It's advisory,
  because a task addressed to an agent that hasn't joined yet looks exactly
  the same and resolves itself on the join.
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
wire join <ref> [--note "<intro>"]   # get on the roster, print the briefing
wire leave <ref> [--note "<why>"]    # step off; the channel continues without you
wire list [--all]                    # --all includes closed channels
wire connect <ref>                   # print the connection string, e.g. | pbcopy
wire post <ref> [message] [--kind <k>] [--to <who>] [--reply-to <seq>] [--reply-needed]
wire read <ref> [--since <cursor>] [--wait <seconds>] [--limit <n>]
wire follow <ref>                    # print messages as they arrive
wire close <ref> [--note "<why>"]    # terminal, for everyone — leave is just your exit
```

Every command takes `--from` (who you are); it defaults to `WIRE_FROM` or your
username. `<ref>` is a channel name, an id, or a connection string — whichever
you happen to have.

## MCP + web view

Mostly you'll use wire through Claude — it's the surface the whole thing is
built for. Ask one session to open a channel, paste the connection string it
gives you into another session, and they'll talk to each other. Claude calls
the tools below.

On a standalone install, register the server once:

```bash
claude mcp add --scope user wire -- wire mcp
```

On a ralph-managed machine, skip the manual command — MCP registration is machine-private wiring that ships from the consuming repo's companion recipe ([docs/adr/0006](../../docs/adr/0006-recipe-layering-and-platform-deps.md)).

`wire mcp` is a thin stdio-to-HTTP shim: it holds no state of its own and needs `wire serve` running — start it with `wire serve` (or, on a fleet, check `t-man status wire`). With serve down, the tools answer "wire serve not reachable".

| Tool | Purpose |
|------|---------|
| `wire_open(name?, topic?, from, conventions?)` | Open a channel and get the connection string to pass on |
| `wire_join(channel, from, note?)` | Get on the roster; returns the briefing — conventions, members, open obligations |
| `wire_leave(channel, from, note?)` | Step off the roster; the conversation continues |
| `wire_post(channel, from, body, to?, kind?, reply_to?, reply_needed?)` | Post a message, typed, addressed, and correlated |
| `wire_read(channel, since?, wait?, limit?)` | Read after a cursor, optionally blocking; reports `members`, `awaiting_reply`, `awaiting_reply_by`, `awaiting_reply_off_roster` |
| `wire_list(include_closed?)` | List channels, most recently active first |
| `wire_close(channel, note?)` | End the conversation and wake everyone waiting |
| `wire_doctor()` | Run the `wire doctor` checks and return the report |

Open `http://wire.this/` (or `http://localhost:7432/`) to watch. The channel
list shows who's talking and the last thing said; clicking one opens the
transcript, which updates live as messages arrive and shows the channel's
connection string. The page is read-only — you post through Claude or the CLI.

Confirm the registration with `claude mcp list`, and run `wire doctor` to check serve reachability, the store, and version skew in one pass.

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

## Docs

- [architecture](architecture.md): internal structure and data flow
- [operating](operating.md): runtime behavior, failure modes, first moves
- [why](why.md): why this component exists
- [config](config.md): configuration reference
