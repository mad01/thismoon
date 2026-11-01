---
id: csl-repo-tools
title: The repo tools csl registers
split: train
query: 'registerRepoTools'
file: 'mcpserver/'
anchors: [services/csl/internal/mcpserver/tools_repo.go]
max_files: 2
max_answer_chars: 1200
expected:
  - 'csl_repo_lookup'
  - 'csl_repo_info'
  - 'csl_repo_health'
  - 'csl_repo_pull'
  - 'csl_repo_reindex'
forbidden:
  - 'csl_repo_(delete|clone|add|remove|status|list)\b'
oracle: |
  - csl_repo_lookup: resolve a repo name to its local checkout path, or find repos by catalog owner, system or component.
  - csl_repo_info: git health for one repo (branch, dirty files, index staleness) with an action field saying what to do.
  - csl_repo_health: git health across every local checkout, which ones hold uncommitted or unpushed work.
  - csl_repo_pull: git pull a workspace repo with safety checks.
  - csl_repo_reindex: reindex one repo in the local zoekt index.
notes: >-
  An enumeration question. The query pulls the registering file and its
  one caller, so the corpus is two files. The forbidden pattern names
  plausible tools that do not exist.
---
List every MCP tool that registerRepoTools registers, one bullet each with the tool name and what it does.
