# cards architecture

## Overview

cards is a tool: one Go program installed to `~/code/bin/cards`. At
runtime it is a short-lived terminal UI built on bubbletea. It reads card
messages from stdin, reads keys from /dev/tty and draws on /dev/tty.
cards itself leaves nothing running; run actions it started keep going.
There is no store: the cards live in memory for one run.

## Structure

```
cmd/cards/          entrypoint
internal/cli/       cobra commands; tui.go opens /dev/tty and runs the program
internal/protocol/  Decode: one JSON line to a typed Message, with every rule
internal/input/     stdin lines to program messages
internal/liststore/ list-mode state: cards, filter, cursor, marks
internal/listview/  list-mode model and view, plus run and emit actions
internal/feedstore/ feed-mode state: entries, replace by id, the --max limit
internal/feedview/  feed-mode model and view
internal/frame/     the header and footer rows both modes share
internal/match/     the smart-case subsequence filter and scope prefixes
internal/order/     the --newest end: where new cards go in both modes
internal/render/    spans to styled text, cut to a width in cells
```

The stores and the views do no I/O. The side effects sit at two edges:
`internal/cli` owns the terminal, and `listview/action.go` starts `run`
argvs and writes `emit` lines.

## Data flow

1. `cards list` or `cards feed` checks that stdin is not a terminal, opens
   /dev/tty, and starts a bubbletea program on it.
2. A goroutine reads stdin line by line. Each line goes through
   `protocol.Decode`, which checks the rules and replaces control
   characters, and reaches the model as a message.
3. The model applies the message to its store and redraws. A bad line
   becomes a footer error instead of a card, and reading goes on.
4. Keys from /dev/tty edit the filter, move the cursor or scroll. In list
   mode, Enter takes the target cards from the store and runs their enter
   actions. A letter key runs its action on the marked cards when one of
   them has it, and types into the filter otherwise.
5. At the end of stdin the footer says `input ended` and the UI stays up.
   Ctrl+C quits. In list mode with `--esc-quits`, so does Esc once there
   are no marks and no filter left to clear.

## Interfaces

CLI: `list [--out FILE] [--card-style STYLE] [--newest END]
[--esc-quits]`, `feed [--max N] [--card-style STYLE] [--newest END]`,
`docs`, `doctor`, and `version` (bare token, or the shared four-key build
metadata object with `-o json`) via the shared
`github.com/mad01/thismoon/buildinfo` package.

Input: the JSON Lines protocol on stdin, with five message types
(`header`, `card`, `remove`, `snapshot`, `clear`). The full reference is
in [operating.md](operating.md).

Output: list mode's emit actions append `{"action":"NAME","id":"ID"}` lines
to the `--out` file. cards writes nothing else; stdout is unused.
