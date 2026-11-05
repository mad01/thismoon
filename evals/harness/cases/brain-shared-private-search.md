---
id: brain-shared-private-search
title: Answer from a shared and a private note and name each vault
notes: >-
  The production port (8443) is only in shared/notes/dispatch-api.md and the
  local development port (18443) only in
  private/notes/dispatch-api-local-dev.md, which links [[dispatch-api]]
  across the vaults. Search tags every hit with its vault, as
  shared:notes/dispatch-api.md. A read-only question: no vault, origin
  branch or gh call may change.
workspace: brain:team-multi
timeout: 240
max_turns: 15
graders:
  - type: regex
    pattern: '\b8443\b'
  - type: regex
    pattern: '\b18443\b'
  - type: regex
    pattern: 'dispatch-api(\.md|\]\]|\||#)'
  - type: regex
    pattern: 'dispatch-api-local-dev(\.md|\]\]|\||#)'
  - type: regex
    pattern: '(?i)\bshared\b'
  - type: regex
    pattern: '(?i)\bprivate\b'
  - type: llm
    weight: 2
    rubric: >-
      The answer gives the production port (8443) as coming from the shared
      vault and the local development port (18443) as coming from the private
      vault, and names the vault for each fact.
  - type: brain_cmd
    pattern: '^(search|list|read|peek)$'
  - type: vault_unchanged_except
  - type: origin_branch
    max: 0
  - type: gh_call
    pattern: '.'
    max: 0
---
What port does dispatch-api listen on in production, and which port do I use
for it in local development?
