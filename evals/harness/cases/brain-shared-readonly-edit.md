---
id: brain-shared-readonly-edit
title: Fix a shared note through a PR instead of editing it in place
pending: "needs multi-vault (vaults: config, --vault, promote --to shared)"
notes: >-
  Target contract: the shared vault is read-only and changes only through
  PRs (brain edit ... --pr, or promote --to shared). Fixture team-multi
  needs shared/notes/parcel-tracker.md with "HTTP API on port 7070". The
  brain_cmd pattern matches the full brain command line, not only the
  subcommand.
workspace: brain:team-multi
timeout: 300
max_turns: 20
graders:
  - type: vault_unchanged_except
    weight: 2
    paths: ['private/**']
  - type: brain_cmd
    weight: 2
    pattern: '(edit .*--pr\b|promote .*--to[ =]shared)'
  - type: llm
    rubric: >-
      The answer says the port fix (7070 to 7071) went to the shared vault as
      a pull request, not as a direct edit, and names the PR or its branch.
---
The parcel-tracker note says its HTTP API is on port 7070, but it moved to
7071 last week. Fix the note.
