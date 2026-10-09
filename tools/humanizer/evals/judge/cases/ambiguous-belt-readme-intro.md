---
id: ambiguous-belt-readme-intro
label: unknown
bucket: ambiguous
source: thismoon tools/belt/README.md lines 1-12, commit 358ca407e409 (2026-10-06)
license: BSD-3-Clause (this repository)
generator: ""
words: 168
notes: AI-drafted then humanized README intro with a bold lead-in two-item list (MAD-337/MAD-340).
---
# belt

Guard and hint hooks for Claude Code sessions. belt inspects tool calls and either denies the risky ones before they run or advises after them, always with a reason the agent can act on.

Named for the layer it adds: suspenders holds up the git side (pre-commit secret scanning and internal-name guard). belt holds up the session side, before anything reaches git.

belt has two halves, and the split is deliberate (see `docs/adr/0008`):

- **Guards** run as a PreToolUse hook and can deny. They are reserved for damage that is hard to undo.
- **Hints** only advise. Most run as a PostToolUse hook after the tool call whose result stands; two ride session lifecycle events instead (SessionStart and UserPromptSubmit). A hint that misfires costs a few lines of ignored text rather than a stalled session.

The one-paragraph summaries below say what each guard and hint does; [docs/hooks.md](docs/hooks.md) covers how each one decides, why it exists, how it fails, and the `~/.claude/settings.json` entries that wire belt up.
