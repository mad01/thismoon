# cards

A terminal UI that shows JSON Lines from stdin as cards. Pipe any producer
into it and it draws the cards in whatever pane it runs in.

- `cards list` is a picker. Type to filter, Space to mark, Enter to act on
  the marked cards. A card can also offer letter keys, such as `x` to
  ignore a pull request, that act on the marked cards.
- `cards feed` is a live log. The newest entry is at the bottom, or at the
  top with `--newest top`, and a re-sent event replaces its old entry
  instead of adding a second one.

The producer can be a shell loop, jq, a script or an agent. It writes one
small JSON message per line, and cards reads the keys from the terminal,
so stdin stays free for the data.

## Install

```bash
make install   # builds and installs to ~/code/bin/cards
```

With ralph, the `cards` recipe does the same on every `ralph up`.

## Usage

A picker. Mark both targets and press Enter, and each one appends a line
to the `--out` file:

```bash
{
  printf '%s\n' '{"type":"header","text":"Deploy targets"}'
  printf '%s\n' '{"type":"card","id":"staging","lines":["staging",[{"text":"healthy","color":"green"}]],"actions":{"enter":{"emit":"deploy"}}}'
  printf '%s\n' '{"type":"card","id":"prod","lines":["prod",[{"text":"2 alerts","color":"red","bold":true}]],"actions":{"enter":{"emit":"deploy"}}}'
} | cards list --out /tmp/picked.jsonl

cat /tmp/picked.jsonl   # {"action":"deploy","id":"staging"} and the same for prod
```

A live log. Each log line becomes an entry, and error lines are red:

```bash
tail -f app.log \
  | jq -R -c --unbuffered '{type: "card", lines: [.], tone: (if test("ERROR") then "red" else "default" end)}' \
  | cards feed
```

Keys in both modes: type to filter, Backspace to delete a character, Esc
to clear, Ctrl+C to quit. In list mode Space marks, so the filter is one
word. In feed mode the filter takes spaces, and every word must match.
The UI stays open after the input ends, so you can keep filtering.

A card with a `scope`, such as `ignored`, stays hidden until the filter's
first word is the start of that name and a colon. So `i:`, `ign:` or
`ignored:` show the ignored cards, and `i:lgn` filters within them.
The footer names the scope while it shows. This only works for scopes
that some card has, so `docs:` stays plain text while no scope starts
with `docs`.

| Mode | Keys | Flags |
|------|------|-------|
| `list` | arrows move, Space marks, Enter runs the enter action of the marked cards (or the cursor card), a letter with an action on a marked card runs it on the marked cards instead of typing, Esc clears the marks and then the filter | `--out FILE`, `--card-style` (default `space`), `--newest` (default `bottom`), `--esc-quits` (default off) |
| `feed` | arrows, Page Up, Page Down and Home scroll and stop following, End follows again; with `--newest top`, End scrolls and Home follows | `--max N` (default 1000), `--card-style` (default `none`), `--newest` (default `bottom`) |

`--card-style` takes `none`, `space`, `line` or `box`. `--newest` takes
`bottom` or `top`. In list mode it says where a card with a new id goes; a
snapshot keeps its own order. In feed mode it says where the newest entry
shows.

`--esc-quits` is for a popup that runs `cards list` and should close on
Esc the way fzf does. Esc still clears the marks first, then the filter,
and quits only when nothing is left to clear. Without the flag, Esc never
quits, so a list in a pane that stays open survives a stray Esc.

## The protocol in short

Five message types, one JSON object per line:

| Type | What it does |
|------|--------------|
| `header` | Sets the line above the cards: `{"type":"header","text":"..."}` |
| `card` | Adds a card, or replaces the card with the same `id` |
| `remove` | Drops the card with `id` |
| `snapshot` | Replaces every card at once with the `cards` array |
| `clear` | Drops every card and keeps the header |

A card has 1 to 3 `lines`. A line is a string, or a list of spans
(`{"text","color","bold"}`) in the 16 terminal colors by name. A card can
also carry `tone`, `match`, `at`, `scope` and `actions`: one for Enter
and one for each lowercase letter `a` to `z` it offers. An action runs an argv
(`run`) or writes a name to the `--out` file (`emit`), and may carry a
short `label` that the footer shows while cards are marked. Any other key
name in `actions` makes the card invalid. cards replaces control
characters in all text it draws, so untrusted text such as commit
messages cannot send escape sequences to your terminal.

`cards docs` prints the full reference: every field and rule, both modes
and the failure modes. The same text is in [operating.md](operating.md).

## Debugging by hand

- `cards doctor` checks that `/dev/tty` opens, TERM is set and not `dumb`,
  and the terminal shows at least 16 colors. It exits nonzero when a check
  fails.
- A bad line does not stop the stream. The footer counts it and shows the
  latest one in red as `line N: <error>`.
- Check what a producer sends, line by line: `producer | head -5 | jq -c .`
- Try one card on its own:
  `printf '%s\n' '{"type":"card","id":"a","lines":["hello"]}' | cards list`
- `cards version -o json` shows which build you run.

## Develop

```bash
make build    # ./cards
make test     # go test ./...
make lint     # golangci-lint
make fmt      # golines + gofumpt
```

The protocol lives in `internal/protocol`: change a message or a field
there first, then update the reference in `operating.md`. See
[CLAUDE.md](CLAUDE.md) for the package layout.

## Docs

- [architecture](architecture.md): internal structure and data flow
- [operating](operating.md): the protocol reference, both modes, failure
  modes and first moves (printed by `cards docs`)
- [why](why.md): why this component exists
- [config](config.md): flags and environment
