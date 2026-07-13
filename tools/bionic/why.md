# why bionic

## The problem

Long markdown piles up on a single developer machine: notes, generated
summaries, docs read through a pager. Reading it linearly is slow. Bionic
reading bolds the first half of each word so the eye anchors on the prefix
and completes the rest, which makes dense prose faster to scan. What was
missing was that transform as something any text flow can pass through: a
shell pipe on one side, an agent tool call on the other.

## Why its own tool

The transform is a pure text filter with no state and no process to keep
running, so it belongs under `tools/`, not `services/`. It is also too
specific to borrow from elsewhere: naive word-bolding wrecks markdown, so
the transform has to skip fenced code blocks, inline code, link URLs,
headers, and text that is already bold or italic, and those rules need
their own table-driven tests. Keeping it a small standalone component means
the CLI and the MCP tool ship from one place and the skip rules live next
to the tests that pin them down.

## Why this shape

One core function, two thin frontends. `render` (stdin or file in,
transformed text out) and `mcp` (a stdio server exposing `bionic_render`)
both call the same `transform.Bionic`, so a shell pipe and an agent tool
call can never disagree about output for the same input.

The transform walks lines rune by rune instead of parsing markdown into a
tree: the skip rules are cheap to express that way and a mistake stays
local to one line. Output is plain markdown `**bold**` markup, so anything
that renders markdown downstream renders the result.

Delivery follows the repo's two-layer recipe split (docs/adr/0006): this
component owns the portable part — build, install to the local bin, and the
adhoc codesign macOS requires before it will keep launching a stdio MCP
server — while MCP registration and the sandbox wrapper are machine-private
wiring in the consuming repo's companion recipe.

## Non-goals

bionic does not decide when to apply itself; callers pipe text in or invoke
the tool. It does not render HTML or serve a page: output is markdown bold
markup for whatever renderer sits downstream. It does not do full markdown
parsing, only the line-level skip rules the tests cover. And it has no
configuration: the bold ratio is fixed at roughly the first half of each
word.
