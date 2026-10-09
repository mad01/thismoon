---
id: ai-pr-description
label: likely_ai
bucket: ai
source: generated
license: generated for this eval
generator: claude-sonnet-5-5 via claude -p, 2026-10-08
words: 206
notes: Sectioned PR description with Problem, Changes, Testing headings. Prompt was Write a pull request description for a change that adds retries with exponential backoff to a webhook delivery worker. About 200 words. Output only the PR description, nothing else.
---
## Add exponential backoff retries to webhook delivery worker

### Problem
The webhook delivery worker made a single attempt per event. A brief outage, timeout, or 5xx from a receiver dropped the event permanently. Customers had to request manual redelivery.

### Changes
- Failed deliveries are now retried up to 6 times with exponential backoff: 5s, 10s, 20s, 40s, 80s, then 160s.
- Each delay gets ±20% random jitter, so many failing endpoints don't retry at the same moment.
- Only retryable failures trigger a retry: network errors, timeouts, `429`, and `5xx`. Other `4xx` responses fail immediately.
- A `Retry-After` header on `429` and `503` responses is honored, capped at 15 minutes.
- Retry state (`attempt_count`, `next_attempt_at`) is stored on the delivery record, so retries survive worker restarts.
- Events that exhaust all attempts move to `failed` and emit a `webhook.delivery.exhausted` metric.

### Configuration
`WEBHOOK_MAX_ATTEMPTS`, `WEBHOOK_BASE_DELAY`, and `WEBHOOK_MAX_DELAY` control the retry behavior. Defaults match the schedule above.

### Testing
- Unit tests cover the backoff calculation, jitter bounds, and `Retry-After` handling.
- Integration tests cover recovery after transient failures, permanent failure on `400`, and resuming after a worker restart.

### Notes
Receivers may see duplicate deliveries. Each request already carries a stable `Webhook-Id` header for deduplication.
