---
id: human-go-contributors-summit-compiler
label: likely_human
bucket: human
split: test
source: https://github.com/golang/blog/blob/cd9c7db29b815fd4e93402ad0d31b631aa2a6746/content/contributors-summit-2019.article, Compiler and Runtime report by Lynn Boger in Contributors Summit 2019 (2019-08-15), repo commit cd9c7db29b81 (2019-08-15)
license: CC BY 3.0 (blog.golang.org footer at that commit), quoted with attribution
generator: ""
words: 313
notes: Meeting-report prose, human counterpart to ai-meeting-notes; present-format line breaks and the *Binary*size* bold marker kept verbatim, hedged and slightly repetitive.
---
The Go contributors summit was a great opportunity
to meet and discuss topics and ideas with others who also contribute to Go.

The day started out with a time to meet everyone in the room.
There was a good mix of the core Go team
and others who actively contribute to Go.
From there we decided what topics were of interest
and how to split the big group into smaller groups.
My area of interest is the compiler, so I joined that group
and stayed with them for most of the time.

At our first meeting, a long list of topics were brought up
and as a result the compiler group decided to keep meeting throughout the day.
I had a few topics of interest that I shared and many that others suggested
were also of interest to me.
Not all items on the list were discussed in detail;
here is my list of those topics which had the most interest and discussion,
followed by some brief comments that were made on other topics.

*Binary*size*.
There was a concern expressed about binary size,
especially that it continues to grow with each release.
Some possible reasons were identified such as increased inlining and other optimizations.
Most likely there is a set of users who want small binaries,
and another group who wants the best performance possible and maybe some don’t care.
This led to the topic of TinyGo, and it was noted that TinyGo was not a full implementation of Go
and that it is important to keep TinyGo from diverging from Go and splitting the user base.
More investigation is required to understand the need among users and the exact reasons
contributing to the current size.
If there are opportunities to reduce the size without affecting performance,
those changes could be made, but if performance were affected
some users would prefer better performance.
