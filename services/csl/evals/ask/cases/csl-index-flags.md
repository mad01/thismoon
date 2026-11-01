---
id: csl-index-flags
title: Flags of the csl index command
split: test
query: 'indexCmd\.Flags -f:_test\.go'
anchors: [services/csl/internal/cli/index.go]
max_files: 1
max_answer_chars: 1400
expected:
  - '\ball\b'
  - 'lexical-all'
  - '\bstatus\b'
  - '\bclean\b'
  - '\brepair\b'
  - '\bjson\b'
  - '\brepo\b'
  - '\bdrain\b'
  - '\bsemantic\b'
  - 'semantic-all'
  - '(?i)\bfalse\b'
forbidden:
  - '--force\b'
  - '--verbose\b'
  - '--dry-run\b'
  - '--limit\b'
oracle: |
  - --all (false): force a full re-index of all repos, lexical and semantic.
  - --lexical-all (false): force a full lexical re-index only.
  - --status (false): show the index status table.
  - --clean (false): delete the index directory.
  - --repair (false): validate shards and remove corrupted ones.
  - --json (false): output as JSON.
  - --repo (""): re-index a single repo by absolute path, skipping the global staleness check.
  - --drain (false): batch-index repos queued by post-merge hooks.
  - --semantic (false): also build the semantic embedding index for discovered repos.
  - --semantic-all (false): re-embed all repos, incremental per file, fetching the model on first run.
notes: >-
  Held out. Ten flags in one init block; the budget fits ten one-line
  bullets and nothing else. The forbidden flags are common cobra flags
  this command does not define.
---
List every flag the csl index command defines, with its default and what it does.
