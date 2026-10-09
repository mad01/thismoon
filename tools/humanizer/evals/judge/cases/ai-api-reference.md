---
id: ai-api-reference
label: likely_ai
bucket: ai
source: generated
license: generated for this eval
generator: openai/gpt-5-mini via OpenRouter chat completions, 2026-10-08
words: 124
notes: Flat API reference prose with few stylistic tells; AI reference docs read close to human reference docs. Prompt was Write an API reference paragraph for a Go function ParseDuration(s string) (time.Duration, error) in a config library that accepts strings like "1h30m" and "2d". About 150 words. Output only the reference text, nothing else.
---
ParseDuration(s string) (time.Duration, error) parses a human‑readable duration string from configuration and returns the corresponding time.Duration or an error. The input accepts numeric values (integers or floats) followed by unit suffixes, for example "1h30m", "90m", "1.5h" or "2d" (days interpreted as 24h). Supported units are ns, us (or µs), ms, s, m, h and d; multiple unit elements may be concatenated in any order (e.g., "2d3h15m"). Leading and trailing whitespace are ignored and an optional leading + or - sign is accepted. If the string is syntactically invalid, contains unsupported units, or the resulting duration would overflow the range of time.Duration, an error is returned. This function is intended for parsing durations in config files where a concise, human-friendly representation (including days) is required.
