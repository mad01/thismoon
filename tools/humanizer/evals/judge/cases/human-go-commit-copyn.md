---
id: human-go-commit-copyn
label: likely_human
bucket: human
source: https://github.com/golang/go/commit/7781fed24ea79d819bc0ecfdafe8c24151a83c31 by Russ Cox (2017-11-14), message without Gerrit trailers
license: BSD-3-Clause (golang/go repository), quoted with attribution
generator: ""
words: 236
notes: Long expert commit message with benchmark tables, a parenthetical aside and a small grammar slip; human counterpart to ai-commit-message.
---
io: eliminate full copy of copy loop in CopyN

CL 60630 claimed to and did “improve performance of CopyN”
but in doing so introduced a second copy of the I/O copying loop.
This code is subtle and easy to get wrong and the last thing we
need is of two copies that can drift out of sync. Even the newly
introduced copy contains various subtle changes that are not
obviously semantically equivalent to the original. (They probably
are, but it's not obvious.)

Although the CL description does not explain further what the
important optimization was, it appears that the most critical
one was not allocating a 32kB buffer for CopyN(w, r, 512).

This CL deletes the forked copy of copy and instead applies
the buffer size restriction optimization directly to copy itself.

CL 60630 reported:

name          old time/op    new time/op    delta
CopyNSmall-4    5.09µs ± 1%    2.25µs ±86%  -55.91%  (p=0.000 n=11+14)
CopyNLarge-4     114µs ±73%     121µs ±72%     ~     (p=0.701 n=14+14)

Starting with that CL as the baseline, this CL does not change a ton:

name          old time/op  new time/op  delta
CopyNSmall-8   370ns ± 1%   411ns ± 1%  +11.18%  (p=0.000 n=16+14)
CopyNLarge-8  18.2µs ± 1%  18.3µs ± 1%   +0.63%  (p=0.000 n=19+20)

It does give up a small amount of the win of 60630 but preserves
the bulk of it, with the benefit that we will not need to debug these
two copies drifting out of sync in the future.
