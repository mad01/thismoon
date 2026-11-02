---
id: belt-sweep-readsize-cross
title: Two hints, two files, one answer
split: train
query: 'readSizeMinLines|spansManyFiles lang:go -f:_test\.go'
anchors: [tools/belt/internal/hint/readsize.go, tools/belt/internal/hint/sweep.go]
max_files: 3
max_answer_chars: 1000
expected:
  - '\b300\b'
  - '\bfind\b'
  - '\bfd\b'
  - '(?i)glob|wildcard|\*'
  - '(?i)more than two|third|three|> ?2|over two'
  - '(?i)director|not an existing regular file|not a regular file|regular file'
  - '(?i)recursive'
  - '\b16\b'
  - '\b677\b'
forbidden:
  - '\b350\b'
  - '\b500\b'
oracle: |
  - readSizeMinLines = 300 (tools/belt/internal/hint/readsize.go:16): a Read without a range earns the hint from 300 lines.
  - spansManyFiles (tools/belt/internal/hint/sweep.go:677) returns true for find or fd, for more than two path operands, and for any operand with a glob character (* ? [).
  - Under a recursive search it also returns true for an operand that is not an existing regular file, such as a directory or a missing path; a non-recursive grep over one or two named files is not a sweep.
notes: >-
  The cross-file case. The query is an OR over two symbols in two files,
  which is the corpus shape csl can build and a path list cannot. Two line
  numbers in two files check that the worker keeps them apart. The
  directory fact accepts the code's own wording, "not an existing regular
  file", since the function never says directory.
  Since thismoon #363 the corpus carries a third file, csl's tools_ask.go,
  which names the constant in a comment only; the worker must cite the
  definition's file and line, not the mention.
---
At what line count does the read-size hint fire, and which conditions make spansManyFiles treat a command as a sweep over many files? Cite the constant and the function with their lines.
