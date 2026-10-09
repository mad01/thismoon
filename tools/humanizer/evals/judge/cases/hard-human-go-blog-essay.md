---
id: hard-human-go-blog-essay
label: likely_human
bucket: hard
source: https://github.com/golang/blog/blob/da069a0697710daf6b99c8cd4a6d02098d6f86d6/content/errors-are-values.article, Errors are values by Rob Pike (2015-01-12), repo commit da069a069771 (2019-05-06)
license: CC BY 3.0 (blog.golang.org footer at that commit), quoted with attribution
generator: ""
words: 176
notes: Polished essay paragraph that opens with a tricolon (unfortunate, misleading, and easily corrected); present-format one-sentence-per-line kept verbatim.
---
This is unfortunate, misleading, and easily corrected.
Perhaps what is happening is that programmers new to Go ask,
"How does one handle errors?", learn this pattern, and stop there.
In other languages, one might use a try-catch block or other such mechanism to handle errors.
Therefore, the programmer thinks, when I would have used a try-catch
in my old language, I will just type `if` `err` `!=` `nil` in Go.
Over time the Go code collects many such snippets, and the result feels clumsy.

Regardless of whether this explanation fits,
it is clear that these Go programmers miss a fundamental point about errors:
_Errors_are_values._

Values can be programmed, and since errors are values, errors can be programmed.

Of course a common statement involving an error value is to test whether it is nil,
but there are countless other things one can do with an error value,
and application of some of those other things can make your program better,
eliminating much of the boilerplate that arises if every error is checked with a rote if statement.
