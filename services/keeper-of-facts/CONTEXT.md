# keeper-of-facts

The assertion store and its evidence pins. These terms apply to the files under
`services/keeper-of-facts/`; the repo-wide terms are in the root CONTEXT.md.

## Language

**Assertion**:
A one-sentence, evidence-pinned statement about system behavior stored by keeper-of-facts, carrying a kind, a subject, a confidence, and a fresh/stale/retracted status.
_Avoid_: fact, memory

**Evidence pin**:
A hashed line range in a repo working tree that grounds an assertion. kof v1's only pin kind is a code pin.
_Avoid_: citation, reference

**Stale**:
A reversible assertion status set by `kof check` when a pin's content no longer hashes the same; it flips back to fresh when the content matches again.
_Avoid_: expired, invalid

**Retract**:
Terminal withdrawal of an assertion with a counter-evidence note. Retracted assertions are never re-checked.
_Avoid_: delete, cancel
