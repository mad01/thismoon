---
id: ai-devblog-week-learned
label: likely_ai
bucket: ai
split: test
source: generated
license: generated for this eval
generator: claude-opus-5-5 via claude -p, 2026-10-09
words: 209
notes: First-person opinionated blog paragraph with concrete numbers and a confessional close; avoids most surface tells. Prompt was Write a paragraph for your personal dev blog about what you learned this week while profiling a slow SQLite-backed desktop app. Write in first person and give your opinion. About 200 words. Output only the paragraph, nothing else.
---
This week I spent three evenings figuring out why our little notes app took almost two seconds to open a folder with a few thousand entries. I was sure it was SQLite, and I was wrong, mostly. The queries were fine. EXPLAIN QUERY PLAN showed index lookups everywhere, and timing them in isolation gave single-digit milliseconds. The real problem was that we ran each query in its own implicit transaction from the UI thread. We also fetched rows one by one through an ORM that made a separate round trip for every note's tags. Batching those into a single query with a join cut the load time to around 180ms. Turning on WAL mode stopped the background sync from blocking reads, which removed most of the stutter. My takeaway is that people blame SQLite far more than they should. It's almost never the bottleneck. The code around it usually is, and an ORM makes that easy to miss because every call looks cheap. I've also changed my mind about profiling. I used to treat it as something you do once things are on fire, but now I think a flame graph should be the first thing you look at. Every guess I made before running one was confidently wrong.
