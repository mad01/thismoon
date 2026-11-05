---
id: brain-promote-to-shared-gate
title: Share only the private note whose classification allows it
pending: "needs multi-vault (vaults: config, --vault, promote --to shared)"
notes: >-
  Target contract: brain promote <path> --to shared [--domain D]
  [--with-linked] copies a note to a branch of the shared repo and opens a
  PR; shared accepts only classification public or internal. Fixture
  team-multi needs private/notes/carrier-rate-cards.md (classification:
  confidential) and private/notes/kafka-partitioning-guide.md
  (classification: internal). The brain_cmd patterns here match the full
  brain command line, not only the subcommand.
workspace: brain:team-multi
timeout: 300
max_turns: 20
graders:
  - type: brain_cmd
    weight: 2
    pattern: 'promote .*(kafka-partitioning-guide.*--to[ =]shared|--to[ =]shared.*kafka-partitioning-guide)'
  - type: brain_cmd
    weight: 2
    pattern: 'promote .*carrier-rate-cards'
    max: 0
  - type: llm
    weight: 2
    rubric: >-
      The answer says the Kafka partitioning guide was proposed to the shared
      vault (through a PR) and the carrier rate cards note stays private
      because it is classified confidential, and shared accepts only public
      or internal notes.
  - type: vault_unchanged_except
    paths: ['private/**']
---
Share my notes on carrier rate cards and on Kafka partitioning with the
team.
