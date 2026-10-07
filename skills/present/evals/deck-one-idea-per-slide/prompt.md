---
max_turns: 14
allowed_tools: [Read, Glob, Grep, Skill]
---
You are preparing a present page from the work summary below: a brief, and a deck for the review meeting on Thursday with the platform and catalog teams. The present MCP is not available in this environment, so do not try to call it. Instead, write the arguments you would pass to `present_create` as one JSON object with the keys `title`, `content`, `deck`, and `references`, inside a single ```json fenced block. Put the fenced block last in your reply.

Work summary (a test fixture, every number is given here): incident review for the 2026-09-30 search outage.

- Timeline: 11:04 search latency alert; 11:09 paged; 11:12 the search index cluster at 100% CPU; 11:31 index rebuilt on the warm standby; 11:38 recovered. 34 minutes of degraded search, 6 of them a full outage.
- Cause: a catalog import of 2.1 million rows ran the reindex in the foreground with no rate limit.
- Search error rate per five minutes during the window: 11:00: 0.2%, 11:05: 4.1%, 11:10: 18.5%, 11:15: 22.0%, 11:20: 21.4%, 11:25: 19.8%, 11:30: 9.3%, 11:35: 1.1%, 11:40: 0.2%.
- Actions: rate-limit the reindex to 5000 rows per second (owner: catalog, PR #431 open at https://github.com/example/shop/pull/431); move imports to the standby first (owner: platform, proposal due 2026-10-14); add a CPU alert at 80% on the index cluster (done).
- Risk: the standby is one node; a second failure during an import would have no fallback.
