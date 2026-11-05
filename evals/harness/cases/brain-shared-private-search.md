---
id: brain-shared-private-search
title: Answer from a shared and a private note and name each vault
pending: "needs multi-vault (vaults: config, --vault, promote --to shared)"
notes: >-
  Target contract: search and list cover every vault by default and each hit
  carries its vault; read takes vault:path (shared:notes/x.md). Fixture
  team-multi needs shared/notes/dispatch-api.md (production port 8443) and
  private/notes/dispatch-api-local-dev.md (local port 18443). Vault paths in
  graders are prefixed shared/ or private/.
workspace: brain:team-multi
timeout: 240
max_turns: 15
graders:
  - type: regex
    pattern: '\b8443\b'
  - type: regex
    pattern: '\b18443\b'
  - type: regex
    pattern: 'shared(:|/)notes/dispatch-api\.md|\[\[dispatch-api\]\]'
  - type: regex
    pattern: 'private(:|/)notes/dispatch-api-local-dev\.md|\[\[dispatch-api-local-dev\]\]'
  - type: llm
    weight: 2
    rubric: >-
      The answer gives the production port (8443) from the shared vault and
      the local development port (18443) from the private vault, and says
      for each fact which vault it came from.
  - type: brain_cmd
    pattern: '^(search|list|read)$'
  - type: vault_unchanged_except
---
What port does dispatch-api listen on in production, and which port do I use
for it in local development?
