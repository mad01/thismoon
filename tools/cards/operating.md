# operating cards

cards shows JSON Lines from stdin as cards in a terminal UI, drawn in
whatever pane it runs in. A producer (a shell loop, jq, a script, an agent)
writes one message per line; cards draws them and reads keys from the
terminal. Two modes: `{{.Bin}} list` is a picker you filter, mark and act
on, and `{{.Bin}} feed` is a log that follows its newest entry.

## how it runs

There is no serve process, no store and no config file. Each run reads
card messages from stdin and keys from /dev/tty, and draws on /dev/tty, so
stdin must be a pipe or a file: `producer | {{.Bin}} list`. Cards live in
memory and are gone when the run ends.

The UI stays open after stdin ends, and the footer says `input ended`.
Ctrl+C quits. Esc quits too in list mode with `--esc-quits`, once there is
nothing left for it to clear.

Exit status:

- 0 after Ctrl+C, or after Esc quits with `--esc-quits`.
- 2 for a usage error: an unknown flag, a flag value that is not one of
  its choices (`--newest middle`), stdin is a terminal, `--max` below 1.
- 1 for anything else, such as no terminal to open or an `--out` file
  that cannot be opened.

## list mode

    {{.Bin}} list [--out FILE] [--card-style none|space|line|box] [--newest top|bottom] [--esc-quits] [--wrap-last]

Flags:

- `--out FILE`: where emit actions go. Each one appends a JSON line to
  FILE, which is created when missing. Without it, an emit action fails.
- `--card-style`: how cards are set apart. `none` puts nothing between
  them, `space` a blank row, `line` a dim rule under each card, and `box`
  a dim rounded border around each card. Default `space`.
- `--newest`: where a card with a new id goes. `bottom` puts it below the
  other cards, `top` above them. Default `bottom`. In both, a card whose id
  is shown keeps its place, and a snapshot keeps the producer's order. The
  cursor and the marks stay on their cards when a card comes in above
  them. With `top`, until the first key or paste the cursor stays on the
  first card, the newest, so cards that come in one at a time start you
  there. After that the cursor stays on the card you moved it to, and
  Enter never lands on a card you did not pick. The filter applies to a
  new card as to any other.
- `--esc-quits`: Esc quits, as Ctrl+C does, when there are no marks and
  the filter is empty. Before that each Esc still clears one thing, marks
  first and then the filter, so marks and a filter take three presses.
  Use it for a popup that should close on Esc the way fzf does. Leave it
  off in a pane that stays open, so a stray Esc does not quit it. Default
  off.
- `--wrap-last`: a card's last line that is wider than the card takes a
  second row instead of being cut. It breaks at the last space that fits,
  or inside the word when there is none, and a second row that is still
  too wide is cut with `…`. Only the last line wraps; any other line wider
  than the card is cut as always. A card whose lines fit looks the same as
  without the flag. A row of cards is as tall as its tallest card, so the
  cards beside a wrapped one stay lined up. A resize wraps the line again,
  or puts it back on one row, to fit the new width. Default off.

Keys:

- Typing: filters the cards (see "the filter"). Space marks instead of
  typing, so the filter is one word. While cards are marked, a letter that
  a marked card has an action for runs that action instead of typing (see
  `a` to `z` below). A paste goes into the filter with its spaces dropped.
  Backspace deletes the last character of the filter.
- Up and Down: move the cursor one row of cards. Left and Right: one card.
  Page Up and Page Down: one screen.
- Space: marks or unmarks the cursor card, then moves to the next card.
- Enter: runs the enter action of every marked card that the filter shows,
  in screen order. With no mark it acts on the cursor card. Then it clears
  every mark. The footer reports the result, such as
  `ran 2, emitted 1, 1 without an action, 1 failed: <error>`, in red when
  anything failed. Enter does not quit.
- `a` to `z`: when a marked card that the filter shows has an action for
  the letter, the letter runs it on every such card in screen order. Then
  it clears every mark, as Enter does. Marked cards without that letter's
  action are skipped. The footer names the key and its label, then the
  result, such as `x ignore: ran 2, 1 without x`. In every other case the
  letter types into the filter: with nothing marked it always types, so a
  letter never acts on the cursor card alone. Shift and Caps Lock type a
  capital letter, which never acts.
- Esc: clears the marks when there are any, marks on cards the filter
  hides included, otherwise the filter, which brings the default set back
  (see "scopes"). With `--esc-quits`, Esc quits when there are no marks
  and the filter is empty.
- Ctrl+C: quits.

A `run` action starts its argv directly, with no shell, in a session of
its own: it outlives the UI, never reads the terminal, and its output is
discarded. cards does not wait for it. For pipes or redirects, make the
argv `["sh", "-c", "..."]`. An `emit` action appends
`{"action":"NAME","id":"CARD_ID"}` and a newline to the `--out` file in
one write. The id is the one the producer sent, byte for byte.

Every card needs an id in list mode: a card without one is refused, and so
is a snapshot that holds one. A card whose id is already shown replaces it
in place. Panes narrower than 120 cells show one column of cards; 120 cells
and wider show two, and 180 and wider show three. The cursor card has a
gray bar down its left side and a bold first line; a marked card has a
green bar just right of that one, so a focused, marked card shows both.
The bars look the same in every card style. The footer
shows the filter, the shown and total counts, and the marked count. The
total counts the set the filter shows from: the default set, or the cards
in the scopes it selects, whose names come before the counts, as in
`> i:  ignored  2/5  0 marked`.

While a card the filter shows is marked, the footer also names the keys
the marked cards offer, such as `enter open  x ignore`. Each key comes
with its label, or with `run` or `emit` when it has none. Enter comes
first, then the letters in order. When marked cards label a key
differently, the first of them on screen names it. A label is cut to 16
cells there, and the whole row to the pane width.

## feed mode

    {{.Bin}} feed [--max N] [--card-style none|space|line|box] [--newest top|bottom] [--wrap-last]

Flags:

- `--max N`: keep the newest N entries and drop older ones. Default 1000;
  below 1 is a usage error.
- `--card-style`: as in list mode. Default `none`.
- `--newest`: where the newest entry shows. `bottom` puts it at the
  bottom, `top` at the top with older entries pushed down. `--max` drops
  the oldest entries either way. Default `bottom`.
- `--wrap-last`: as in list mode, for an entry's last line that is wider
  than the space after the time column. Default off.

Each entry starts with a time column in `15:04` form and local time: the
card's `at`, or the time its line arrived when `at` is missing. The newest
entry is at the bottom, or at the top with `--newest top`.

The feed follows its newest entry: the screen stays at the newest end as
entries arrive. A scroll key stops following and pins the screen, so it
does not move while entries arrive. While the screen shows no entry (an
empty feed, or a filter that hides all of them), a scroll key changes
nothing, so a feed that follows keeps following. The footer says
`following`, or `scrolled, End follows` (`Home follows` with
`--newest top`).

Keys:

- Up and Down: scroll one entry. Page Up and Page Down: one screen. Each
  one stops following.
- With the newest at the bottom, Home goes to the oldest entry and stops
  following, and End follows the newest entry again.
- With `--newest top`, the two swap: End goes to the oldest entry and
  stops following, and Home follows the newest entry again.
- Typing, Space included: filters the entries. A paste goes into the
  filter too. Backspace deletes the last character.
- Esc: clears the filter, which brings the default set back.
- Ctrl+C: quits. Enter does nothing; feed mode has no actions or marks.

The footer counts as in list mode: the shown entries, then the entries in
the set the filter shows from, after the names of the scopes it selects.

A card whose id is already in the feed replaces that entry in place. The
entry keeps its place and its arrival time, so an event sent again does
not show twice. A card with no id always adds an entry.

## the filter

Both modes filter as you type. In list mode the filter is one word,
because Space marks. In feed mode the filter takes spaces: the query
splits on them and every word must match. A word matches when its
characters appear in order, with gaps allowed, so `lgn` matches `login`. Smart case: a query with no capital
letter ignores case, and one with a capital letter matches case exactly.
The filter runs against the card's `match` text, or else every line's text
joined with spaces.

### scopes

A card may carry a `scope`, the name of a set such as `ignored`. The
filter shows the default set, the cards with no scope, until its first
word selects scopes. That word selects them when it holds a colon and the
part before the first colon starts the name of a scope that some card
has, shown or not. Then only the cards in every scope that part starts
show. The rest of the word after the colon, and any later words in feed
mode, filter within them as above. The empty filter, and any filter whose
first word selects nothing, shows the default set.

Only the scopes present count. With cards in the scope `ignored`, `i:`,
`ign:` and `ignored:` show them all, and `i:lgn` shows the ignored ones
that match `lgn`. With no card in a scope that starts with `docs`, `docs:`
is plain filter text, so it still finds pull request titles like
`docs: fix typo`. When the first card of a scope arrives, or the last one
goes, the filter is read again.

Everything else works within the shown cards as it does without a scope:
the cursor, marks, letter keys, Enter and `--newest`. Marks on cards the
filter hides stay, as they do for any hidden card. The cards in a scope
keep the producer's order, so a producer that wants the latest ignored
pull request first sends its snapshot in that order.

A pull request list can send each ignored pull request with a scope and
an `x` action that brings it back. Type `i:` to see them, then Space and
`x` on one:

    {"type":"card","id":"pr-9","lines":["Bump deps"],"scope":"ignored","actions":{"x":{"run":["/usr/local/bin/unmute","pr-9"],"label":"unignore"}}}

## protocol

Each line on stdin is one JSON object, and its `type` field names the
message. Leading and trailing white space is trimmed, blank lines are
skipped, and fields the protocol does not know are ignored, so a producer
may send fields a later version understands. The one exception is a key
name in a card's `actions`, which must be one the protocol knows. A line may be up to 16 MiB,
its line ending not counted. A longer line is skipped and reported as an
error, and reading goes on with the next line.

### messages

`header` sets the line drawn above the cards. `text` is a LINE and is
required. A header with no color or bold of its own is drawn bold.

    {"type":"header","text":"Open pull requests"}

`card` adds a card, or replaces the card with the same id.

    {"type":"card","id":"pr-12","lines":["Fix login timeout",[{"text":"ci passing","color":"green"}]],"tone":"default","match":"fix login timeout pr-12","at":"2026-11-21T09:30:00Z","actions":{"enter":{"run":["open","https://example.com/pr/12"],"label":"open"},"x":{"run":["/usr/local/bin/mute","pr-12"],"label":"ignore"}}}

| Field | Type | Rule |
|-------|------|------|
| `id` | string | Keys the card for replace and remove. Required in list mode; in feed mode a card with no id always adds an entry. |
| `lines` | array of LINE | Required, 1 to 3 lines. |
| `tone` | COLOR | Colors every span that has no color of its own. Default `default`. |
| `match` | string | The text the filter runs against. Default: the line texts joined with spaces. |
| `at` | string | When the event happened, RFC3339 (`2026-11-21T09:30:00Z`). Feed mode shows it in the time column; list mode ignores it. |
| `scope` | string | The set the card belongs to, which the filter selects by prefix (see "scopes"). A lowercase letter, then lowercase letters, digits and hyphens, at most 32 characters. Default: no scope, the default set. |
| `actions.enter` | ACTION | What Enter does on the card in list mode. Feed mode ignores it. |
| `actions.a` to `actions.z` | ACTION | What that lowercase letter does on the card in list mode while it is marked. Feed mode ignores them. |

`actions` takes only the key names `enter` and `a` to `z`. Any other
name, a capital letter included, makes the card invalid, so a mistyped
key cannot leave a card without its action. A key whose action is `null`
has no action.

`remove` drops the card with `id`, which is required. In list mode an
unknown id is ignored; in feed mode every entry with that id goes.

    {"type":"remove","id":"pr-12"}

`snapshot` replaces every card at once with `cards`, an array of card
objects with the fields above and no `type`. One invalid card refuses the
whole snapshot. List mode keeps the snapshot's order with either
`--newest`, and a repeated id keeps its first place and its last content.
Feed mode drops every entry, then adds the cards in order, so the last
card is the newest entry.

    {"type":"snapshot","cards":[{"id":"a","lines":["first"]},{"id":"b","lines":["second"]}]}

`clear` drops every card and keeps the header.

    {"type":"clear"}

### values

A LINE is a JSON string, or an array of spans. A string is one span with
no color. Any other JSON value, null included, is a bad line.

A span is `{"text": STRING, "color": COLOR, "bold": BOOL}`. Every field is
optional. A span whose color is `default` or missing takes the card's
tone.

A COLOR is one of `default`, `red`, `green`, `yellow`, `blue`, `magenta`,
`cyan`, `white` and `dim`. They are the terminal's own colors, so the
user's theme picks the shade. `default` is the terminal's normal text
color, and `dim` is the faint attribute, not a hue. An empty or missing
color is `default`; any other name is invalid.

An ACTION sets exactly one of two fields:

- `{"run": ["PROGRAM", "ARG", ...]}`: an argv to start. It must not be
  empty and its first element must not be empty.
- `{"emit": "NAME"}`: an event name written to the `--out` file. It must
  not be empty.

Setting both, or neither, is invalid. An ACTION may also carry `label`,
a short name for it, such as `{"run": [...], "label": "ignore"}`. List
mode shows the label in the footer next to the key. Keep it to a word or
two; the footer cuts a label to 16 cells.

### untrusted text

Card text often comes from places the producer does not control, such as
pull request titles and commit messages. cards replaces every control
character with a space in each string it keeps for drawing: span text,
`match`, emit names and action labels. The control characters are U+0000 to U+001F (tab,
CR, LF and ESC among them), U+007F, and U+0080 to U+009F. So no producer
can send a terminal escape sequence. Text typed or pasted into the filter
gets the same replacement.

Two fields are never drawn, so they pass through as sent. A `run` argv is
started, not drawn. An id is a key: ids that differ only in a control
character stay two cards, and an emit writes the id back unchanged.

### errors

A line that breaks the protocol does not stop the stream. The footer
counts it and shows the latest one in red as `line N: <error>`, where N
counts every line from 1, blank lines included. The errors read:

- `protocol: bad JSON: ...`: not JSON, or a field of the wrong JSON type.
- `protocol: unknown message type "x"`: a missing or unknown `type`.
- `protocol: invalid message: card: lines: got 4, want 1 to 3`: well-formed
  JSON that breaks a rule; the text names the message and the field.
- `liststore: card id is required`: list mode got a card with no id.
- `longer than 16 MiB, skipped`: the line was too long to read, so cards
  dropped it.

## examples

A picker built with printf. Marking both targets and pressing Enter
appends one line per card to the `--out` file:

```sh
{
  printf '%s\n' '{"type":"header","text":"Deploy targets"}'
  printf '%s\n' '{"type":"card","id":"staging","lines":["staging",[{"text":"healthy","color":"green"}]],"actions":{"enter":{"emit":"deploy"}}}'
  printf '%s\n' '{"type":"card","id":"prod","lines":["prod",[{"text":"2 alerts","color":"red","bold":true}]],"actions":{"enter":{"emit":"deploy"}}}'
} | {{.Bin}} list --out /tmp/picked.jsonl

cat /tmp/picked.jsonl
# {"action":"deploy","id":"staging"}
# {"action":"deploy","id":"prod"}
```

A live log built with jq. Each log line becomes an entry, and error lines
are red. `--unbuffered` makes jq pass each line on at once instead of in
blocks:

```sh
tail -f app.log \
  | jq -R -c --unbuffered '{type: "card", lines: [.], tone: (if test("ERROR") then "red" else "default" end)}' \
  | {{.Bin}} feed --max 500
```

## failure modes

`stdin is a terminal; pipe card messages in`: the command ran with no
producer in front of it. Pipe something in: `producer | {{.Bin}} list`.

`list: open terminal: open /dev/tty: ...` (or `feed: ...`): there is no
controlling terminal, as in a launchd job, cron or a shell an agent runs
with no terminal. cards needs one to read keys. Run it in a terminal pane;
`{{.Bin}} doctor` checks it.

The footer shows `line N: protocol: ...` in red: the producer sent a bad
line, and the "errors" section above says which rule it broke. Check the
producer's lines one by one with `producer | head -5 | jq -c .`.

Enter reports `emit needs --out FILE`: list mode was started without
`--out`, so an emit action has nowhere to go. A letter key reports the
same.

A letter runs an action instead of going into the filter: cards are
marked, and a marked card has an action for that letter. Press Esc to
clear the marks, then type; or type the filter before you mark.

The list quits on Esc: it runs with `--esc-quits`, and there were no
marks and no filter for Esc to clear. Drop the flag where the list should
stay open.

A card never shows: it has a `scope`, and the filter shows the default
set. Type the start of the scope's name and a colon, such as `i:` for
`ignored`; the footer then names the scope.

A word with a colon filters as text when you meant a scope: no card in
that scope is present, so there is nothing to select. The footer names no
scope.

The footer shows `protocol: invalid message: card: actions: unknown key
"..."`: a card's `actions` holds a key name other than `enter` and the
lowercase letters `a` to `z`.

Enter reports `failed: start "PROGRAM": ...`: the program is not on the
PATH cards runs with, or not executable. The argv runs with no shell, so
pipes, globs and `~` do nothing; use `["sh", "-c", "..."]` for those.

A run action seems to do nothing: its output is discarded by design. Have
the command write to a file, or open a window of its own.

No colors, or the wrong ones: NO_COLOR is set, or TERM names a terminal
with fewer than 16 colors. `{{.Bin}} doctor` reports what it detects.

The feed never shows new lines: the producer buffers its output. Use
`jq --unbuffered`, `grep --line-buffered`, or `stdbuf -oL` in front of it.

## first moves

1. `{{.Bin}} doctor` (the terminal opens, TERM is usable, 16 colors or
   more)
2. `printf '%s\n' '{"type":"card","id":"a","lines":["hello"]}' | {{.Bin}} list`
   (one card on screen; Ctrl+C quits)
3. `producer | head -5 | jq -c .` (each line parses as JSON on its own)
4. `{{.Bin}} version -o json` after an install
