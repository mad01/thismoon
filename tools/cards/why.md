# why cards

## The problem

Scripts and agents keep making short lists a person should look at or
pick from: open pull requests, failing jobs, branches, events as they
happen. The usual ways to show them each fall short. Plain output scrolls
away and cannot be acted on. A fuzzy finder picks one line but has no
room for a status line or a color. A purpose-built TUI per list costs a
day of work every time.

## Why its own tool

One small program with a fixed input protocol covers all of them. The
producer only writes JSON Lines, which a shell loop with printf or jq can
do, and cards owns the drawing, the filter and the keys. A list that
updates in place and a log that follows its tail are two modes over the
same protocol. A producer moves from one to the other without a rewrite.

## Why this shape

- Cards come on stdin and keys from /dev/tty, so the UI sits at the end of
  an ordinary pipe and runs in whatever pane it is started in.
- Colors are the 16 terminal colors by name, so the user's theme picks the
  shades and a producer cannot clash with it.
- Card text often comes from untrusted sources such as commit messages.
  Decode replaces every control character in the text cards draws, so no
  producer can send a terminal escape sequence.
- A `run` action starts its argv with no shell, so card data never turns
  into shell code.
- No serve process and no store: the cards live for one run, and the
  producer is the source of truth.

## Non-goals

No saved state between runs, no mouse, no network, no layout language
beyond 1 to 3 lines per card. cards is not a general TUI framework; a
view that needs more is its own program.
