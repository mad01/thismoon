---
id: belt-validmode
title: What validMode accepts
split: train
query: 'sym:validMode'
anchors: [tools/belt/internal/config/config.go]
max_files: 1
max_answer_chars: 700
expected:
  - '\bhard\b'
  - '\bsoft\b'
  - '(?i)empty|""'
  - '\b1193\b'
  - '(?i)\berror\b'
forbidden:
  - '(?i)\bmedium\b'
  - '(?i)\bstrict\b'
oracle: |
  - validMode (tools/belt/internal/config/config.go:1193) accepts "" (the empty per-rule default), "hard" and "soft".
  - Any other value returns an error: mode is %q (must be "hard" or "soft").
notes: >-
  One large file (1091 lines) and a question with a small answer: the
  baseline shape bulk delegation exists for. The line number checks that
  the worker reads the numbers it was given instead of guessing.
---
Which mode values does validMode accept, what does it do with anything else, and on which line is it defined?
