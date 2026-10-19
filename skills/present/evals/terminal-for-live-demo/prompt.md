---
max_turns: 14
allowed_tools: [Read, Glob, Grep, Skill]
---
You are preparing a present page from the talk summary below: a brief, and a deck the speaker will present. The present MCP is not available in this environment, so do not try to call it. Instead, write the arguments you would pass to `present_create` as one JSON object with the keys `title`, `content`, `deck`, and `references`, inside a single ```json fenced block. Put the fenced block last in your reply.

Talk summary (a test fixture, every number is given here): "ship: one command deploys", 15 minutes for the platform guild on 2026-10-23, 8 people in the room.

- The speaker wants a live demo. At the start of the talk they run `ship deploy --env staging checkout` from the checkout repo at `~/code/checkout`. The deploy takes about four minutes. They explain the three points below while it runs, then come back to the same terminal to show the finished deploy and run `ship status checkout` in it.
- Point one: ship builds once and promotes the same image from staging to prod, so prod never runs an image staging did not.
- Point two: a deploy takes 4 minutes end to end, down from 19 with the old pipeline, measured over 212 deploys in September.
- Point three: rollback is `ship rollback checkout` and takes 40 seconds.
- Ask: the guild moves the three remaining services (search, cart, payments) to ship by 2026-11-15; owner: the platform team.
- Sources: the ship README at https://github.com/example/ship/blob/main/README.md and the September deploy report at https://metrics.example.internal/d/deploys.
