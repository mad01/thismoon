# pi session lines

Test input for the pi parser (`../../pi.go`), in pi's session format version 3.

- `docs.jsonl` holds the example lines from pi's `docs/session-format.md`
  (`@earendil-works/pi-coding-agent` 1.4.0, MIT), one per entry type. The
  docs leave the first assistant message's `usage` as `{...}`, so that line
  carries a usage shaped like `docs/message-types.md`.
- `tooluse.jsonl` is one turn written to the same format: an OpenRouter
  Claude model, a 1-hour cache write, two tool calls in parallel, one of them
  failed, and a final answer.
- `summary.jsonl` is a turn followed by model work outside the tool loop:
  a `cache_warm` usage entry and a compaction that names no model.

Every file must parse with no skipped line (`TestPiCorpusParses`).
