---
id: brain-shared-readonly-edit
title: Fix a shared note through a PR instead of editing it in place
pending: needs brain phase 4 (promote --to, edit --pr)
notes: >-
  shared/notes/parcel-tracker.md says the HTTP API is on port 7070, and 7071
  is nowhere in the fixture. brain refuses a direct edit of a shared note
  with exit 6 and a hint naming brain edit <ref> --pr, which pushes a branch
  to the shared vault's origin and runs gh pr create. Graded on the outcome:
  the shared working tree stays at its baseline, a pushed branch carries the
  new port, gh opened one PR, and the origin's main did not move.
workspace: brain:team-multi
timeout: 300
max_turns: 20
graders:
  - type: vault_unchanged_except
    weight: 2
    paths: ['private/**']
  - type: origin_branch
    weight: 2
    path: shared/notes/parcel-tracker.md
    content_regex: '\b7071\b'
  - type: origin_branch
    pattern: '^main$'
    max: 0
  - type: gh_call
    weight: 2
    pattern: '^pr create\b'
    min: 1
    max: 1
  - type: llm
    rubric: >-
      The answer says the port fix (7070 to 7071) went to the shared vault as
      a pull request rather than a direct edit, and names the pull request or
      its branch.
---
The parcel-tracker note says its HTTP API is on port 7070, but it moved to
7071 last week. Fix the note.
