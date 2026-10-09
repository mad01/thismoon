---
id: human-go19-release-notes
label: likely_human
bucket: human
source: https://github.com/golang/go/blob/go1.9/doc/go1.9.html introduction section, tag go1.9 (2017-08-24)
license: BSD-3-Clause (golang/go repository), quoted with attribution
generator: ""
words: 114
notes: Go 1.9 release-notes introduction; HTML tags stripped and lines reflowed, words unchanged. Clean, even prose that ends in a five-item feature run.
---
The latest Go release, version 1.9, arrives six months after Go 1.8 and is the tenth release in the Go 1.x series. There are two changes to the language: adding support for type aliases and defining when implementations may fuse floating point operations. Most of the changes are in the implementation of the toolchain, runtime, and libraries. As always, the release maintains the Go 1 promise of compatibility. We expect almost all Go programs to continue to compile and run as before.

The release adds transparent monotonic time support, parallelizes compilation of functions within a package, better supports test helper functions, includes a new bit manipulation package, and has a new concurrent map type.
