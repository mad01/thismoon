# Worker definitions

Three Claude Code agent definitions, one per effort level, each in its own
file under `agents/` beside this one. Those files are the canonical text.
The `thismoon/factory` recipe links each into the user agents directory,
`~/.claude/agents/`, and renders the same three into
`recipes/factory/agents.json` for unattended workers.

- `agents/factory-worker-low.md`: trivial units.
- `agents/factory-worker-medium.md`: known-domain units.
- `agents/factory-worker-high.md`: novel units.

Each file is named after its `name` field. An unnamed spawn reports that
field as its agent type, and the `factory-worker` hint matches on it.

The `model` line follows `routing.md`: low and medium run on the mid tier,
high runs on the top tier. When the tier mapping changes, change these lines
with it, then run `python3 recipes/factory/render_agents_json.py` from the
repo root so `agents.json` follows.

The tool list is the same in all three and is exact: Read, Edit, Write,
Bash, Glob, Grep, SendMessage, and the MCP tools of csl, humanizer and
keeper-of-facts (kof). Nothing else. No Agent tool, because workers do not
spawn workers. No GitHub, Linear, worklog, present, speak or brain tools: the
weaver lands branches, the orchestrator alone writes worklog checkpoints, and
the board page is the orchestrator's. A tool list cannot stop `gh` through
Bash, so the body forbids it.

The body is the same in all three. It points the worker at the brief and
tells it to read `~/.claude/skills/factory/worker-brief.md` when no brief
arrived, which is the case for an unattended worker.

The Claude Code subagent docs confirm both frontmatter fields used here:
`effort` takes low, medium, high, xhigh or max, and `tools` accepts the
`mcp__<server>__*` wildcard form. See
https://code.claude.com/docs/en/sub-agents.
