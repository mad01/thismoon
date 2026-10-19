---
type: llm
weight: 2
---
The reply ends with one JSON object for present_create with `title`, `content`, `deck`, and `references`. You see only the reply, so the fixture it was written from is repeated here.

Fixture: a 15 minute talk, "ship: one command deploys", for the platform guild on 2026-10-23, 8 people. The speaker runs `ship deploy --env staging checkout` from `~/code/checkout` at the start as a live demo; it takes about four minutes; they explain three points while it runs, then come back to the same terminal to show the finished deploy and run `ship status checkout`. Points: ship builds once and promotes the same image from staging to prod; a deploy takes 4 minutes end to end, down from 19 with the old pipeline, measured over 212 deploys in September; rollback is `ship rollback checkout` and takes 40 seconds. Ask: move search, cart, and payments to ship by 2026-11-15, owner the platform team. Sources: the ship README at https://github.com/example/ship/blob/main/README.md and the September deploy report at https://metrics.example.internal/d/deploys.

PASS if all of these hold for the `deck`:

1. The live demo is a block with `t: "terminal"` and a `session` name of lowercase letters, digits, and dashes. It is not a `code` block of pasted output, not an `image`, and not an `html` block.
2. At least two sections of the deck hold a terminal block with the same `session` name. One comes early, where the deploy starts. One comes later, after the explaining slides, where the speaker comes back to it.
3. Every terminal block that names that session carries the same `cmd`, `cwd`, and `prefill` values (each may be absent, but then it is absent on all of them). The `cwd` is `~/code/checkout` or an equivalent path to the checkout repo, and the `prefill` is `ship deploy --env staging checkout` or the block's `cmd` is that command.
4. Each terminal slide holds the terminal block alone, or beside one short paragraph inside a `columns` block. No terminal block sits inside a `details` block.
5. The slides between the two terminal slides carry the three points, each heading a claim rather than a topic, and every number on them is one from the fixture.
6. The deck ends on the ask: an `h` of "Next" or a close equivalent with a list of at most three actions, naming the platform team and 2026-11-15.
7. The `content` (the brief) tells the speaker to switch the live terminal on before the talk and off after it, in prose or in a `code` block with `present terminal on` and `present terminal off`.

FAIL if any of these hold: the deck is missing; the reply tried to call an MCP tool instead of writing the JSON; there is no JSON object; the demo is a `code` block, an `image`, or an `html` block instead of a terminal; only one section holds a terminal block; two terminal blocks name the same session with a different `cmd`, `cwd`, or `prefill`; a terminal block sits inside `details`; a number appears that the fixture does not contain and cannot be computed from it.

Do not fail for any of these: a `rows` value on a terminal block; a second session name used for a different purpose; `ship status checkout` in a note, a paragraph, or a second prefill of a different session; the deploy command in `cmd` rather than `prefill`; a lone `stat` or a `chart` for the 4 versus 19 minutes; `notes`, `reveal`, `tone`, or `layout` fields; a `callout` about the switch; `details` or `code` blocks in `content`; the brief repeating the commands in a `list`.
