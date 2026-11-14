---
name: ask
description: Answer a question from the brain vault, the local markdown knowledge base, and cite the notes behind the answer. Use when the user says "/ask", "ask the brain", "what do we know about", "what do my notes say", "have I written about", or wants an answer from saved knowledge before anything is looked up elsewhere. Read-only. Backed by the brain MCP (brain_hybrid_search, brain_search, brain_peek, brain_read, brain_backlinks) with the `brain` CLI as fallback.
argument-hint: "<question>"
---

# ask

Answers a question from the brain vault and shows which notes the answer
comes from. The vault is the source. Nothing in the answer comes from the
web or from model memory unless the reply says so.

ask only reads. It never writes, edits or promotes a note. To add knowledge,
capture it with `brain capture` or `brain_write`. To research a topic the
vault does not cover, say so and let the user choose the next step.

## Tools

Prefer the MCP tools: `brain_hybrid_search`, `brain_search`, `brain_peek`,
`brain_read`, `brain_backlinks`. The `brain` CLI is the fallback when the MCP
server is not connected (`brain hybrid-search`, `brain search`, `brain peek`,
`brain read`, `brain backlinks`). On a tool error or an empty result that
looks wrong, call `brain_doctor` before a retry.

## Steps

1. **Find the notes.** Run `brain_hybrid_search` with the question in the
   user's words. It joins a literal search and a vector search. When its
   `warnings[]` say the vector half did not run, the hits are literal only:
   follow up with `brain_search` on one or two distinct terms from the
   question. When it returns an error, do the same and tell the user in one
   line that the vector search is down. A `brain_search` query is zoekt
   syntax: two words mean both must be in the same note, and `a|b` means
   either.
2. **Widen once on a miss.** Try a synonym, the singular form, or a broader
   term. Search covers every vault and every tier, so do not narrow with
   `tier` or `vault` unless the user named one. After two misses stop and
   report that the vault holds nothing on the question.
3. **Pick what to read.** `brain_peek` the top hits, five at most. Peek is
   cheap: it returns the frontmatter, the first lines and the backlink count.
   Prefer `notes/` and `MOCs/` over `inbox/`, and a note with a higher
   evidence grade over a lower one.
4. **Read the notes that answer.** `brain_read` each one, with `start_line`
   and `end_line` when only a section matters. A map of content (a note under
   `MOCs/`) is an index: follow its wikilinks to the notes it lists. When a
   note points at a neighbour that looks closer to the question, check
   `brain_backlinks` and read that one too.
5. **Answer.** Follow the reply rules below.

## What the vault says about trust

Every read tool returns the fields that say how far to trust a note. Carry
them into the answer.

| Field | Value | What to do |
|---|---|---|
| `evidence` | `A`, `B` | State the claim plainly |
| `evidence` | `C` | State the claim and say the evidence is weak |
| `evidence` | `D` | The claim is refuted. Say so. Never give it as the answer |
| `evidence` | `mixed` | Grades sit beside each claim in the body. Use the grade of the claim you quote |
| `status` | `raw` | An unchecked capture from `inbox/`. Say it is unverified |
| `status` | `disputed` | Show the `banner` from `brain_read` before the claim |
| `stale` | true | The note is past its `review_by` date. Say it may be out of date |

Search leaves out revoked notes (superseded, retracted, withdrawn). Keep it
that way: set `include_revoked` only when the user asks what the vault used
to say.

## The reply

- Lead with the answer in one or two sentences.
- Then the sources, one line each: the note title as a link, its evidence
  grade when it has one, and a flag for raw, disputed or stale.
- Link a note to the brain web view when the machine runs it: the root URL
  plus `?note=` and the URL-encoded note ref in the `vault:path` form. With
  the host `brain.this`, the note `personal:notes/ops/k8s-upgrades.md` is
  `http://brain.this/?note=personal%3Anotes%2Fops%2Fk8s-upgrades.md`. Without
  the web view, give the `vault:path` ref.
- Quote a note for a number, a date or a decision. Paraphrase the rest.
- Keep what the notes say apart from what you add. Mark an inference as
  yours.
- When notes disagree, show both claims with their grades and dates. Do not
  pick a side without saying why.
- When the vault has no answer, say that in the first line. Offer the
  general answer as a separate, labelled part only when the user wants it.

## Do not

- Do not write, edit or promote a note from this skill.
- Do not fill a gap in the notes with a guess that reads like a citation.
- Do not read a whole vault to answer one question. Five notes is the cap;
  ask the user to narrow the question past that.
