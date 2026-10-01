# ADR-0019: a present page carries a deck beside its brief

**Date:** 2026-09-29
**Status:** Accepted

**Scope:** `services/present` (the page store and its custom resource, the
MCP tools, the page and deck views), `skills/present`.

## Context

present pages are briefs: an agent writes a Doc, the renderer compiles it
to one scrollable HTML fragment, and the reader scrolls. Some of those
briefs get talked through in a meeting, and a scrolling page is the wrong
shape for that. The speaker wants one point on screen at a time, a counter,
and keys to move. The question was where a slide deck lives relative to
the brief it accompanies, and whether the slides come from the brief or
from the author.

Three shapes were on the table. A deck could be a display mode that
re-flows the brief's sections into slides. That costs nothing to author
but makes a poor deck, because a brief's sections run long and a slide
wants three lines. A deck could be its own page with its own id, which
keeps the store untouched but breaks the link between a brief and its
deck. That means two ids to share, two graphs to keep in step, and no way
to get from one to the other. Or a deck could be a second rendition of the
same page, authored on its own.

## Decision

A page id carries up to two renditions, the brief and the deck, and a
page needs at least one of them. The deck is a second Doc the agent writes
for the purpose, with one section per slide. It is compiled through the same
renderer to `deck.html` with its source kept in `deck.json` beside
`content.html` and `doc.json`. Nothing is converted between the two: a
page can be brief-only, deck-only, or both, and `present_update` changes
either without touching the other. The two share what belongs to the id:
title, the one graph, the references, and the share record, so one share
carries both. `/p/{id}` shows the brief and `/p/{id}/deck` the deck; a
deck-only page redirects from the first to the second.

The deck view is client-side, like the brief (ADR-0005). It splits the
compiled fragment into slides in the browser, so the renderer stays one
template and a re-render from source covers both renditions. Remote
control (start, stop, next, prev, goto) is a command file in the page
directory that open deck tabs poll. That keeps the MCP process and the
serve process as decoupled as they are for everything else. The cluster
store has no such file, so a shared instance serves decks without remote
control.

## Consequences

- The store interface, the size cap, the shared wire shape, and the Page
  custom resource all gain a deck body and a deck source. The resource
  fields are optional, so existing objects stay valid, but the definition
  has to be re-applied before a deck reaches a cluster.
- `content` is no longer required on create; a create with neither content
  nor deck is refused, and so is an update that would strip the last
  rendition.
- A deck is authored twice over when the brief already exists. That is the
  point: the deck says less than the brief, on purpose, and the skill tells
  the agent so.
- Sections are slides, so a Doc feature that spans sections (the table of
  contents) has no place in a deck and the view drops it. Speaker notes
  and per-slide layouts stay out of scope; a slide is a section. Superseded
  for decks by `0020-present-slide-layouts-and-chrome.md` (2026-10-01):
  the deck carries its chrome, and slides carry layout, notes, and reveal
  as section fields; the brief's contract is unchanged.
