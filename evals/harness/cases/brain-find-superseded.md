---
id: brain-find-superseded
title: Answer from the newer decision and name the superseded one
notes: >-
  ADR-0004 (RabbitMQ) does not say it is superseded; only ADR-0011 (Kafka)
  says so.
workspace: brain:team
timeout: 240
max_turns: 15
graders:
  - type: regex
    pattern: '(?i)kafka'
  - type: regex
    pattern: '(?i)adr-0011'
  - type: regex
    pattern: '(?i)adr-0004|rabbitmq'
  - type: llm
    weight: 2
    rubric: >-
      The answer says shipment events run on Kafka, citing ADR-0011, and says
      that ADR-0004 (RabbitMQ) was superseded by it. It does not present
      RabbitMQ as the current broker.
  - type: vault_unchanged_except
---
Which message broker carries our shipment events today, and which decision
record says so?
