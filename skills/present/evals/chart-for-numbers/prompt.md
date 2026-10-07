---
max_turns: 12
allowed_tools: [Read, Glob, Grep, Skill]
---
You are preparing a present page from the work summary below. The present MCP is not available in this environment, so do not try to call it. Instead, write the arguments you would pass to `present_create` as one JSON object with the keys `title`, `content`, and `references`, inside a single ```json fenced block. Put the fenced block last in your reply.

Work summary (a test fixture, every number is given here):

Checkout latency review, weeks 36 to 40. The team shipped a connection pool for the pricing service in week 38.

p95 checkout latency in ms per week: week 36: 910, week 37: 940, week 38: 620, week 39: 480, week 40: 455.
p50 checkout latency in ms per week: week 36: 210, week 37: 215, week 38: 180, week 39: 150, week 40: 148.

5xx responses in week 40 by service: pricing 42, checkout-api 17, stock 9, mailer 3.

Open follow-ups: raise the pool size on the two remaining replicas (owner: platform, due Friday); add a p95 alert at 600 ms (owner: checkout team).

Sources: PR #418 (the connection pool) at https://github.com/example/shop/pull/418, and the "checkout latency" dashboard at https://metrics.example.internal/d/checkout.
