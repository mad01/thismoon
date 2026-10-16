# fixture glossary

A small glossary the STE tests run against, so they do not depend on the
CONTEXT.md files of the repo that holds them.

## Language

**Claimed worktree**:
A linked git worktree that one session owns until its branch lands.
_Avoid_: sandbox, workspace, checkout (that means the canonical one)

**Ephemeral page**:
A shared page that expires 30 days after its last write.
_Avoid_: temporary page, TTL page

**Slide**:
One section of a deck as the deck view shows it.
_Avoid_: page (a page is the whole id), frame

**Retract**:
Terminal withdrawal of an assertion with a counter-evidence note.
_Avoid_: delete, cancel
