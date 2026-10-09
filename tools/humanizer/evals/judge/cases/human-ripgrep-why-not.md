---
id: human-ripgrep-why-not
label: likely_human
bucket: human
source: https://github.com/BurntSushi/ripgrep/blob/0.10.0/README.md section Why shouldn't I use ripgrep, tag 0.10.0 (2018-09-07)
license: MIT or Unlicense (dual), quoted with attribution
generator: ""
words: 183
notes: README section with a bulleted list of reasons and two (Please file a bug report!) asides; list-heavy but personal voice.
---
### Why shouldn't I use ripgrep?

Despite initially not wanting to add every feature under the sun to ripgrep,
over time, ripgrep has grown support for most features found in other file
searching tools. This includes searching for results spanning across multiple
lines, and opt-in support for PCRE2, which provides look-around and
backreference support.

At this point, the primary reasons not to use ripgrep probably consist of one
or more of the following:

* You need a portable and ubiquitous tool. While ripgrep works on Windows,
  macOS and Linux, it is not ubiquitous and it does not conform to any
  standard such as POSIX. The best tool for this job is good old grep.
* There still exists some other feature (or bug) not listed in this README that
  you rely on that's in another tool that isn't in ripgrep.
* There is a performance edge case where ripgrep doesn't do well where another
  tool does do well. (Please file a bug report!)
* ripgrep isn't possible to install on your machine or isn't available for your
  platform. (Please file a bug report!)
