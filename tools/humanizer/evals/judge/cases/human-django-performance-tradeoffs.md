---
id: human-django-performance-tradeoffs
label: likely_human
bucket: human
split: test
source: https://github.com/django/django/blob/2.1/docs/topics/performance.txt, section What are you optimizing for, tag 2.1 commit e7ad40fcf4b1 (2018-08-01)
license: BSD-3-Clause (Django), quoted with attribution
generator: ""
words: 170
notes: Reference-docs advice prose with It's important to, trade-offs, a spaced hyphen as a dash and a generic close; the bland docs register a judge can mistake for AI.
---
It's important to have a clear idea what you mean by 'performance'. There is
not just one metric of it.

Improved speed might be the most obvious aim for a program, but sometimes other
performance improvements might be sought, such as lower memory consumption or
fewer demands on the database or network.

Improvements in one area will often bring about improved performance in
another, but not always; sometimes one can even be at the expense of another.
For example, an improvement in a program's speed might cause it to use more
memory. Even worse, it can be self-defeating - if the speed improvement is so
memory-hungry that the system starts to run out of memory, you'll have done
more harm than good.

There are other trade-offs to bear in mind. Your own time is a valuable
resource, more precious than CPU time. Some improvements might be too difficult
to be worth implementing, or might affect the portability or maintainability of
the code. Not all performance improvements are worth the effort.
