---
id: ai-design-alternatives
label: likely_ai
bucket: ai
split: test
source: generated
license: generated for this eval
generator: claude-opus-5-5 via claude -p, 2026-10-09
words: 207
notes: One dense design-doc paragraph walking three alternatives with named products and a throughput figure; AI counterpart to human-go-preemption-alternatives. Prompt was Write the alternatives considered paragraph of a design doc that proposes moving a background job queue from Redis to Postgres using SELECT ... FOR UPDATE SKIP LOCKED. About 200 words. Output only the paragraph, nothing else.
---
We considered three alternatives before settling on Postgres with SELECT ... FOR UPDATE SKIP LOCKED. The first was to keep Redis and harden it: turn on AOF persistence with fsync on every write, and replace our homegrown list-based queue with Redis Streams and consumer groups. That would fix the lost-job problem we hit during failovers, but we would still be running a second stateful system whose durability we only partly trust. We also still couldn't enqueue a job in the same transaction as the database write that triggers it. The second was a dedicated broker like RabbitMQ or Amazon SQS. Both are mature and handle much higher throughput than we need, but each adds infrastructure to run, monitor and secure, and neither fixes the transactional enqueue gap without an outbox table, which puts us back in Postgres anyway. The third was an existing Postgres-backed library such as River or pg-boss. We prototyped River and liked it, but it covers only one of our two worker languages, and we'd end up building around its schema rather than ours. At our current peak of roughly 400 jobs per second, a small purpose-built queue table on the primary database is the simplest option that meets our durability and consistency requirements.
