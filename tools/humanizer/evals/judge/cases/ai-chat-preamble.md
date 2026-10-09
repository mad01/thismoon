---
id: ai-chat-preamble
label: likely_ai
bucket: ai
source: generated
license: generated for this eval
generator: claude-sonnet-5-5 via claude -p, 2026-10-08
words: 118
notes: Chat preamble (Here's a draft) and a trailing offer left in; the same prompt to openai/gpt-5-mini returned no preamble so that output was dropped. Prompt was hey, could you help me write a short intro paragraph for the README of my little python library? it validates environment variables when the app starts. thanks!
---
Here's a draft. I don't know your library's name, so I used `envcheck` as a placeholder:

> **envcheck** is a small Python library that validates your environment variables when your app starts. Declare which variables you expect, along with their types and any defaults, and `envcheck` checks them up front. If something is missing or malformed, it fails fast with a clear error message. That way you find out about a bad config at startup, not in the middle of a request.

If you tell me the real name, the API style (decorator, schema, function call), and any standout features like type coercion, `.env` support, or secrets masking, I can tailor it and add a short usage snippet.
