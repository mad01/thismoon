---
id: ai-commit-message
label: likely_ai
bucket: ai
source: generated
license: generated for this eval
generator: openai/gpt-5-mini via OpenRouter chat completions, 2026-10-08
words: 183
notes: AI counterpart to human-go-commit-copyn. The first Claude attempt appended a Co-Authored-By trailer, so the no-trailer prompt went to gpt-5-mini. Prompt was Write a git commit message (subject line and body) for a change that fixes a race between Close and an in-flight Get in a database connection pool. Do not add Co-Authored-By or any other trailers. Output only the commit message, nothing else.
---
pool: fix race between Close and concurrent Get

There was a race where Close could run concurrently with an in‑flight Get and allow that Get to return a connection (or otherwise observe the pool in a partially closed state) after the pool had begun closing. This could lead to use-after-close, panics, leaked resources, or Gets erroneously succeeding while the pool was being torn down.

This change fixes the race by:
- marking the pool closed under the pool lock before waking waiters or releasing resources,
- ensuring Get checks the closed flag both before blocking and after being woken,
- tracking active/Get waiters so Close can wait (or drain) safely without racing with a concurrently running Get,
- and removing the window where a Get could receive a connection after Close started.

Added a deterministic test that repeatedly races Close with concurrent Gets (run under the race detector) to validate there are no panics, leaks, or incorrect successes. Behavior is preserved for normal usage: after Close returns, no new Gets succeed and resources are cleaned up; callers of Get receive ErrPoolClosed when appropriate.
