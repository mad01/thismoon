---
id: human-rust-book-panic-or-not
label: likely_human
bucket: human
split: test
source: https://github.com/rust-lang/book/blob/e21e1376d9674b8706a1da3e9af9358b0002a179/src/ch09-03-to-panic-or-not-to-panic.md, chapter opening, commit e21e1376d967 (2018-12-29)
license: MIT or Apache-2.0 (dual, rust-lang/book), quoted with attribution
generator: ""
words: 215
notes: Edited book prose under a heading, with signposting (Let's explore, The chapter will conclude) and curly apostrophes; polished tutorial register.
---
## To `panic!` or Not to `panic!`

So how do you decide when you should call `panic!` and when you should return
`Result`? When code panics, there’s no way to recover. You could call `panic!`
for any error situation, whether there’s a possible way to recover or not, but
then you’re making the decision on behalf of the code calling your code that a
situation is unrecoverable. When you choose to return a `Result` value, you
give the calling code options rather than making the decision for it. The
calling code could choose to attempt to recover in a way that’s appropriate for
its situation, or it could decide that an `Err` value in this case is
unrecoverable, so it can call `panic!` and turn your recoverable error into an
unrecoverable one. Therefore, returning `Result` is a good default choice when
you’re defining a function that might fail.

In rare situations, it’s more appropriate to write code that panics instead of
returning a `Result`. Let’s explore why it’s appropriate to panic in examples,
prototype code, and tests. Then we’ll discuss situations in which the compiler
can’t tell that failure is impossible, but you as a human can. The chapter will
conclude with some general guidelines on how to decide whether to panic in
library code.
