# cards configuration

## Where config lives

cards has no configuration file. Each run takes its settings from flags,
its cards from stdin, and its keys from /dev/tty. Nothing is saved
between runs.

## Flags

- `--version` (bool, default `false`): print the build version and exit,
  the standard cobra root flag.

`cards list`:

- `--out FILE` (default none): append each emit action to FILE as a JSON
  line. FILE is created when missing. Without it, emit actions fail.
- `--card-style STYLE` (default `space`): how cards are set apart: `none`
  (nothing), `space` (a blank row), `line` (a dim rule under each card) or
  `box` (a dim rounded border).
- `--newest END` (default `bottom`): where a card with a new id goes,
  `bottom` or `top`. A snapshot keeps its own order either way.
- `--esc-quits` (bool, default `false`): Esc quits, with exit status 0,
  when there are no marks and the filter is empty. Before that, Esc
  clears the marks, then the filter. Feed mode has no such flag.
- `--wrap-last` (bool, default `false`): a card's last line that is wider
  than the card takes a second row, broken at a space, instead of being
  cut. A card whose lines fit looks the same.

`cards feed`:

- `--max N` (default `1000`): keep the newest N entries. Below 1 is a
  usage error.
- `--card-style STYLE` (default `none`): as in list mode.
- `--newest END` (default `bottom`): where the newest entry shows,
  `bottom` or `top`. With `top`, Home follows the newest entry and End
  goes to the oldest.
- `--wrap-last` (bool, default `false`): as in list mode.

`cards version -o json` prints the build metadata object instead of the
bare version token. `docs` and `doctor` take no flags.

## Environment variables

cards reads no variable of its own. The terminal library reads these to
pick the colors it draws with, and `cards doctor` reports the result:

- `TERM`: must be set and not `dumb`.
- `COLORTERM`: `truecolor` or `24bit` turns on full color.
- `NO_COLOR`: any value turns color off. Bold and dim still show.

A `run` action starts its argv with the environment and `PATH` cards was
started with.
