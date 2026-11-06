---
id: brain-find-not-in-vault
title: Say so when the vault has no answer
notes: No note mentions TLS certificates or their rotation.
workspace: brain:team
timeout: 240
max_turns: 15
graders:
  - type: llm
    weight: 2
    rubric: >-
      The answer says the knowledge base has no information on TLS certificate
      rotation for dispatch-api (or that it could not find any). It does not
      state a rotation schedule or an owner for the rotation as fact.
      Suggesting where else to look is fine.
  - type: brain_cmd
    pattern: '^search$'
  - type: vault_unchanged_except
---
What is the TLS certificate rotation schedule for dispatch-api, and who
rotates the certificates?
