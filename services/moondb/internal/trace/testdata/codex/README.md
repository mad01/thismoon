# Codex rollout lines

Test input for the Codex parser (`../../codex.go`): rollout files Codex CLI
wrote in the paginated history mode, from `codex exec` runs on a small Go
package through OpenRouter (`anthropic/claude-haiku-5.5`). Home paths read
`/home/user`, the file owner in command output reads `user`, and the
instruction texts (Codex's base instructions, the developer messages, the
AGENTS.md instructions and the skills list) are trimmed; every other line is
as Codex wrote it.

- `answer.jsonl` is one turn with one response and no tool call.
- `listing.jsonl` is one turn: a command, then the answer.
- `twoturns.jsonl` is a turn with five responses and five tool calls,
  three of them failed: a command whose `item_completed` line says it
  failed, a command whose output header names exit code 1, and an
  `apply_patch` Codex answered with `unsupported call`. A resumed second
  turn follows.
- `toolsearch.jsonl` (Codex 0.167.0, a home with deferred MCP servers) is
  one turn whose first response asks for a `tool_search_call` and a
  command at once, then the answer.

`answer`, `listing` and `twoturns` come from Codex 0.163.0.

Every file must parse with no skipped line (`TestCodexCorpusParses`).
