---
id: human-go-preemption-alternatives
label: likely_human
bucket: human
split: test
source: https://github.com/golang/proposal/blob/5b63da9579c3b19294be614dcad33e20a9a4ad22/design/24543-non-cooperative-preemption.md, Alternatives section (Single-stepping, Jump rewriting) by Austin Clements, commit 5b63da9579c3 (2019-02-25)
license: BSD-3-Clause (golang/proposal), quoted with attribution
generator: ""
words: 244
notes: Design-doc alternatives with markdown headings, parentheticals and wry asides (thoroughly bamboozles debuggers); human counterpart to ai-design-alternatives.
---
## Alternatives

### Single-stepping

Rather than making an effort to be able to stop at any instruction,
the compiler could emit metadata for safe-points only at back-edges
and the runtime could use hardware single-stepping support to advance
the thread to a safe-point (or a point where the compiler has provided
a branch to reach a safe-point, like in the current loop preemption
approach).
This works (somewhat surprisingly), but thoroughly bamboozles
debuggers since both the debugger and the operating system assume the
debugger owns single-stepping, not the process itself.
This would also require the compiler to provide register flushing
stubs for these safe-points, which increases code size (and hence
instruction cache pressure) as well as stack size, much like
cooperative loop preemption.
However, unlike cooperative loop preemption, this approach would have
no effect on mainline code size or performance.

### Jump rewriting

We can solve the problems of single-stepping by instead rewriting the
next safe-point jump instruction after the interruption point to jump
to a preemption path and resuming execution like usual.
To make this easy, the compiler could leave enough room (via padding
NOPs) so only the jump target needs to be modified.

This approach has the usual drawbacks of modifiable code.
It's a security risk, it breaks text page sharing, and simply isn't
allowed on iOS.
It also can't target an individual goroutine (since another goroutine
could be executing the same code) and may have odd interactions with
concurrent execution on other cores.
