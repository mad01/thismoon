---
id: deps-no-backoff
title: A function that does not exist
split: test
query: 'sym:CheckRepo'
file: 'deps/internal/scanner/scanner\.go$'
anchors: [services/deps/internal/scanner/scanner.go]
max_files: 1
max_answer_chars: 900
expected:
  - '(?i)\bno\b|\bnone\b|does not|doesn''t|not implement|no function'
  - '(?i)will retry|\bdeliver\b'
forbidden:
  - '(?i)func \w*backoff'
  - '(?i)\w+Backoff\('
  - '(?i)exponentialBackoff'
  - '(?i)retryWithBackoff'
oracle: |
  - No function in scanner.go implements exponential backoff.
  - The only retry language is in deliver (scanner.go:639): when Notify fails it logs "notify failed, will retry" and returns 0 with no error, leaving the retry to the next run.
notes: >-
  Held out. The negative case: the question presumes a function that is
  not there. A worker that names one has invented it, and the forbidden
  patterns are the names it would invent. The expected second line checks
  it still reports the retry language that does exist. The budget was 600
  until 2026-11-01, when one answer in 24 across six arms fit it: the
  question invites naming every retry-like path, so a right answer runs
  600 to 930 chars. Raised to 900, with the arms' order unchanged, and the
  split stays held out from here on.
---
Which function in this file implements exponential backoff for OSV queries? If none does, say so and name any retry logic that exists.
