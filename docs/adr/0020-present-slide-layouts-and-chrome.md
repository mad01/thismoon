# ADR-0020: present decks carry layout and chrome; the Doc gains the blocks both renditions share

**Date:** 2026-10-01
**Status:** Accepted

**Scope:** `services/present` (the Doc renderer, the deck view), `webkit`
(the content blocks, the header's audio toggle), `skills/present`.
Supersedes the last consequence of ADR-0019 for decks only. The brief's
contract is otherwise unchanged.

## Context

ADR-0019 made a slide a section and left layout out of scope. That gives
one slide shape: a heading over a list, left aligned, at the brief's type
sizes, with no position cue while presenting. A review deck wants more: a
figure that fills the slide, a quotation, a chart beside its caption, a
row of numbers. It wants a mark and a progress cue at the edge, and a room
that never sees the read-aloud controls. The question was how much
vocabulary to add without a second document model, and what of it belongs
to the deck alone.

## Decision

For decks, a slide carries its layout in the Doc. A section `layout`
(`default`, `center`, `statement`, `section`), speaker `notes`, and
`reveal` follow as section fields in stage 2 of the same ticket, decided
and pending, and so does the rule that a slide holding one stat or one
quote alone is the big-number or quote slide, with no field to learn.

The deck's chrome is deck-level: `logo`, `logo_position`, `progress`,
`presenter`, and `footer` sit beside `summary`, `meta`, and `chips`. The
logo and the progress dots are on by default. The footer line is opt-in,
shown only when `presenter` or `footer` is set. The title is already in the
tab and on the title slide, and the presenter is content the deck cannot
invent. The browser builds the chrome from one JSON island the renderer
emits only when a field is set. So the store, the shared wire shape, and
the custom resource stay as they are, and the embedded repo logo is served
from the binary. The header's audio toggle hides every read-aloud control.
It defaults off in the deck view and on in the brief, and each view
remembers its own choice under its own key.

What is Doc-wide, and therefore shared with the brief: the `columns`,
`stat`, `quote`, and `details` blocks, the `ok` and `error` callout
severities, and, in the later step, `tone`, a palette role validated
against `webkit.Roles()`. A container holds plain blocks only, never a
graph or another container. Speaker notes as a presenter view and an image
block stay out.

## Consequences

- Every new field and block is optional. A Doc without them renders
  byte-identically, pinned by a golden fixture, so `present rerender`
  reports existing pages unchanged.
- The deck defaults change the look of every existing deck at its next
  load: a logo in the corner and dots under the bar. On a shared instance
  that reaches every reader once the instance is upgraded. `logo: "none"`
  and `progress: "none"` opt a deck out.
- The `cols` key means two things, a table's headers and a columns block's
  arrays. The block type tells them apart in a custom (un)marshal, so stored
  tables keep their canonical JSON.
- The audio default is a view default. The deck shell names its own storage
  key on `<html>`, and webkit's boot script and header both read it. A
  reader's choice in a brief never hides a deck's controls, or the other
  way round.
- An older shared instance shows the new elements unstyled until its webkit
  is current, and its deck view ignores the island.
- `internal/server/logo.png` is a second copy of `docs/assets/logo.png`. A
  test keeps the two byte-identical, the way the custom resource definition
  is kept.
