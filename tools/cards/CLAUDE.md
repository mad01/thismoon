# cards, a terminal UI for JSON Lines cards (CLI)

Go CLI that reads card messages as JSON Lines on stdin and draws them in
the pane it runs in. `cards list` is a picker (filter, mark, act on Enter);
`cards feed` is a log that follows its newest entry. Keys come from
/dev/tty, so stdin stays the data stream.

## Build, run, test by hand

```bash
make build    # ./cards (build metadata via ldflags, from ../../buildinfo.mk)
make install  # build + cp to ~/code/bin/cards + codesign
make test     # go test ./...
make lint     # golangci-lint run ./...
make fmt      # golines -m 100 with gofumpt

# one card on screen; Ctrl+C quits
printf '%s\n' '{"type":"card","id":"a","lines":["hello"]}' | ./cards list
./cards doctor          # tty, TERM and color checks
./cards docs            # the operating doc, rendered from operating.md
```

## Module layout

```
cards/
  cmd/cards/         entrypoint; exits 2 on cli.ErrUsage, 1 on other errors
  embed.go facts.go  operating doc embed + agentdoc.Facts
  operating.md       protocol reference, modes, failure modes (`cards docs`)
  internal/
    cli/             cobra commands: list, feed, version, docs, doctor;
                     tui.go opens /dev/tty and runs the bubbletea program
    protocol/        Decode: one JSON line to a typed Message, all rules
                     and control-character sanitizing live here
    input/           reads stdin lines, sends them to the program
    liststore/       list-mode state: cards, filter, cursor, marks (no I/O)
    listview/        list-mode bubbletea model, view and actions (run, emit)
    feedstore/       feed-mode state: entries, replace by id, --max (no I/O)
    feedview/        feed-mode bubbletea model and view
    frame/           header and footer rows both modes share
    match/           smart-case subsequence filter and scope prefixes
    order/           the --newest end (bottom or top) both modes take
    render/          spans to styled, width-cut terminal text
  Makefile           package path github.com/mad01/thismoon/tools/cards (monorepo module, no own go.mod)
```

## How it works

`input.Read` turns each stdin line into a message for the program, and
`protocol.Decode` turns the line into a typed `Message`. A store applies
it; the view draws the store. Stores and views do no I/O, so their tests
drive them with messages and key presses. The only side effects sit in
`internal/cli` (the terminal) and `listview/action.go` (starting a `run`
argv, writing an `emit` line).

## Changing the protocol

1. Change `internal/protocol` (types in `protocol.go`, rules in
   `decode.go`) and its tests.
2. Update the protocol section of `operating.md`. `cards docs` prints it,
   and `TestDocsCoversProtocol` fails when a message type has no example.
3. Update the short table in `README.md`.

## Gotchas

- **Tests never need a real terminal.** `doctor.go` probes through the
  `terminal` struct (open, environ, profile), and tests fill it by hand.
  The mode tests drive the models directly. Keep it that way: CI runs with
  no /dev/tty to open.
- **Card text is untrusted.** Decode replaces control characters in every
  string a mode draws. A new text field goes through `protocol.Sanitize`.
  An id and a `run` argv are never drawn and pass verbatim: an id is a key,
  and emit writes it back as sent.
- **Count cells with the model's `render.Cells`.** Bubble Tea's renderer
  counts by wcwidth until the terminal reports mode 2027, then by grapheme
  cluster, and each model follows the switch. A view that calls
  `ansi.StringWidth` or `ansi.Truncate` itself cuts rows the renderer sees
  as wider, so an emoji sequence joined by zero-width joiners pushes the
  row's tail off screen.
- **A letter key types unless a marked card offers it.** `listview`
  checks the marked cards the filter shows for an action on the text the
  key types (`frame.Typed`, so a capital letter still types) and acts only
  when one has it. Keep that gate, and keep letters off the cursor card
  when nothing is marked: otherwise typing a filter word fires actions.
  The footer hint and that check both read `liststore.Marked`, which costs
  the marks, not the cards, because the view calls it on every frame.
- **A scope prefix only counts for scopes present.** `match.Parse` takes
  the scopes the cards have, so `i:` selects `ignored` only while some
  card is in it, and stays filter text otherwise. `liststore` counts cards
  per scope. It parses the filter again and matches every card only when
  the first card of a scope comes or the last one goes. Keep the other
  cards on the one-card path so `TestManySingleCardsApplyFast` holds.
  `feedstore` parses on every read, since it rebuilds Visible anyway.
- **Run actions have no shell.** The argv starts in its own session with
  its output discarded, and cards does not wait for it.
- **Version probe convention.** `cards version -o json` returns the shared
  four-key build metadata object (`version`, `commit`, `tag`,
  `build_time`) from `github.com/mad01/thismoon/buildinfo`; plain
  `cards version` stays a bare token that status parses. cards has no
  serve process, so there is no `/version` endpoint.
- **Runtime debugging lives in `operating.md`.** Keep failure modes there,
  not here.

## See also

- Recipe: `recipes/cards/recipe.toml` (build and install only)
- Human docs: `README.md`, `architecture.md`, `config.md`, `why.md`
