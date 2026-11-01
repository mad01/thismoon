---
id: belt-soft-mode-guards
title: The guards SoftModeGuards lists
split: train
query: 'SoftModeGuards lang:go -f:_test\.go'
anchors: [tools/belt/internal/config/config.go]
max_files: 2
max_answer_chars: 800
expected:
  - 'script-deny-list'
  - 'publish-internal-names'
  - 'git-discard'
  - '\b100\b'
  - '(?i)\berror\b|rejects?'
oracle: |
  - SoftModeGuards (tools/belt/internal/config/config.go:100) lists script-deny-list, publish-internal-names and git-discard.
  - Every other guard ignores mode:, so mode: soft on a guard outside the list is a config error rather than a silently ignored key.
notes: >-
  Same file as belt-validmode, a different region. The answer is a short
  list plus the validation rule written in the comment above it. No
  forbidden patterns: other guard ids appear in the file for other reasons,
  so a correct answer may mention them.
---
Which guard ids does SoftModeGuards list, on which line is it defined, and what does config validation do with `mode: soft` on a guard outside the list?
