# The memory stack

Every agent session starts with an empty context window. On a machine running
the fleet, seven stores carry what one session learned into the next. Six of
them ship here or are set up from the examples here. The seventh, auto
memory, is built into Claude Code. Each store answers one question. The two that
matter at a predictable moment, agent-memory and kof, don't wait for the
agent to remember them: belt's hooks put them in front of the agent at
session start and after a search.

This page says which store holds what, how a fact gets in and back out,
whether a command line tool exists for it, and which hook does the prompting.
The short version is in [HOW-IT-FITS-TOGETHER.md](HOW-IT-FITS-TOGETHER.md);
every hook in depth is in [tools/belt/docs/hooks.md](../tools/belt/docs/hooks.md).

## The stores at a glance

| Store | Answers | Lives at | Interfaces | Reaches the session through |
|---|---|---|---|---|
| Claude Code auto memory | what this agent learned in this repo | `~/.claude/projects/<project>/memory/` | `/memory` inside Claude Code, plain files | Claude Code loads the index itself |
| agent-memory | facts about *you*, for every agent on every machine | `~/.config/agent-memory/` (usually a git clone) | git, plain files | belt's `agent-memory` hint |
| keeper-of-facts (kof) | how a piece of code behaves, pinned to its lines | `~/.local/share/kof/` | `kof` CLI, MCP, web | belt's `kof-consult` and `kof-assertions` hints |
| worklog | where a task stands | `~/code/worklog/` | `worklog` CLI, MCP | the `/worklog` skill |
| handoff | what the next session needs to pick this one up | `~/.handoff/` | the `/handoff` skill | the next session reads the file |
| events | what actually happened | `~/.local/share/events/` | `events` CLI, MCP, web | you query it; nothing is injected |
| moondb | which turn and prompt wrote a line, and what else that turn changed | `~/.local/state/moondb/` | `moondb` CLI, MCP, web | the agent asks with `moondb_why`; nothing is injected |

## Claude Code auto memory

Not a thismoon component. It is listed because it is the store an agent
reaches for by default, and routing facts to the right store only works once
you know where its edges are.

- Claude Code keeps one directory per git repository at
  `~/.claude/projects/<project>/memory/`, shared by every worktree of that
  repo. It holds a `MEMORY.md` index plus one file per fact.
- At session start Claude Code loads the first 200 lines or 25KB of
  `MEMORY.md`, whichever comes first. The fact files are read on demand with
  the ordinary file tools.
- The agent writes it with the same file tools. When you tell Claude to
  remember something, this is where it lands unless your instructions route
  it elsewhere.
- It is machine-local and Claude-only. Nothing syncs it to another machine,
  Codex never sees it, and subagents start without it (forks excepted).
- There is no CLI. `/memory` inside a session lists the memory files, opens
  the folder, and toggles the feature. The settings are `autoMemoryEnabled`
  (per user or per project) and `autoMemoryDirectory` (to relocate it), plus
  the `CLAUDE_CODE_DISABLE_AUTO_MEMORY=1` environment variable. Reference:
  [Claude Code's memory docs](https://code.claude.com/docs/en/memory).

## agent-memory

The fleet's answer to auto memory's two limits: it follows you across
machines and across agents.

- The store is a directory at `~/.config/agent-memory/`, usually a clone of
  a private git repo you create. Its layout matches auto memory: one fact per
  file with `name`, `description` and `metadata.type` frontmatter, plus a
  `MEMORY.md` index with one line per fact. A fact can move between the two
  stores unchanged.
- belt's `agent-memory` hint reads it at SessionStart and injects the index
  lines, capped at 30 facts per store. A second store at
  `~/.config/agent-memory-work/` is injected as well when that directory
  exists. A missing store is silence, so a machine without the second store
  never sees it.
- To add a fact, the agent writes the fact file and its index line in the
  clone, then commits and pushes there. belt never writes to it. With the
  clone's ralph item set to `update = true`, the next `ralph up` on every
  other machine pulls the fact down.
- There is no CLI, and none is needed: git does the sync, and the files are
  the interface.
- To set one up, start from
  [`examples/dotfiles/recipes/agent-memory/`](../examples/dotfiles/recipes/agent-memory/):
  the ralph item that clones it, plus templates for the index and a fact file.

Codex gets no SessionStart hook in the reference setup, so its instruction
file (`AGENTS.md`) has to tell it to read the index.

## keeper-of-facts, worklog, handoff, events and moondb

- kof holds claims about code, not about you. Each assertion is one
  sentence pinned to a hashed file line range. `kof check` flips it stale
  when those lines change, so this memory decays visibly instead of silently
  going wrong. The agent writes with the `kof_assert` MCP tool (or
  `kof assert`); belt does the reading for it. See
  [services/keeper-of-facts/README.md](../services/keeper-of-facts/README.md).
- worklog holds task state keyed by ticket or topic rather than by
  directory. That covers the case auto memory loses: work started in
  `/tmp/foo` that spans three repos ends up filed under a tmp directory
  nobody finds again. `worklog_checkpoint` writes, `worklog_show` resumes,
  and the store is a git repo that can push to a private remote. See
  [tools/worklog/README.md](../tools/worklog/README.md).
- handoff is a one-off bridge document. When a session ends mid-task,
  the `/handoff` skill writes it to `~/.handoff/`, a directory every agent
  can read, and persists any durable learnings to memory. See
  [skills/handoff/SKILL.md](../skills/handoff/SKILL.md).
- events is the record of what happened: every guard deny, every hint,
  belt's ledger of subagent and session lifecycle, and the services' own
  audit trails, on one queryable timeline. Nothing from it is injected; you
  query it with `events_query` or browse `events.this`. See
  [services/events/README.md](../services/events/README.md).
- moondb holds the edits between commits. Claude Code hooks run
  `moondb record` on every prompt, every file edit and every stop, so each
  edit is stored with its before and after content, the turn and prompt
  behind it, and later the commit it landed in. Nobody writes to it by
  hand. The agent reads it on demand: `moondb_why` names the turn that
  wrote a line, and `moondb_turn` shows what else that turn changed. See
  [services/moondb/README.md](../services/moondb/README.md).

## Keeping the stores private

None of these stores needs a git host. kof, events and moondb never leave
the machine. The other three can stay local too, or sync through a Google Drive
folder that only your account can see. A git remote is one option among
these, not a requirement.

- Auto memory is local by default. To carry one project's memory across
  machines, set `autoMemoryDirectory` in that project's
  `.claude/settings.local.json` to a folder under Google Drive for desktop,
  such as `~/Library/CloudStorage/GoogleDrive-<account>/My Drive/claude-memory/<project>`.
  A value in user settings applies to every project, so set it per project
  unless you want them all in one place.
- For agent-memory, belt reads the fixed path
  `~/.config/agent-memory/MEMORY.md` and doesn't care whether a git repo
  sits behind it. To keep it local, use a plain directory, or a clone with
  no remote, and leave out the ralph item that clones it. To sync through
  Drive, keep the store in a Drive folder and symlink
  `~/.config/agent-memory` to it; belt follows the link.
- worklog stays on the machine that created it when its config has no
  `remote:`. `$WORKLOG_DIR` moves the store, a Drive folder included, but
  csl only indexes the store while it sits under your code roots.

Two cautions for the Drive route. Drive for desktop streams files on demand
by default, so mark the folder "Available offline", or a session-start read
can wait on a download. And Drive syncs files, not git history: two machines
writing the same store between syncs leave conflicted copies, so keep one
writer at a time. worklog's own git sync assumes the same.

## Which store gets a new fact

The agent picks the store when it writes, and no hook sees that choice. So
the routing has to live in your global instruction file; the example
[`CLAUDE.md.example`](../examples/dotfiles/CLAUDE.md.example) carries one.
The split it uses:

- A preference, correction or working rule that should hold in every agent
  goes to agent-memory.
- A gotcha about one project, or about Claude Code itself, goes to auto
  memory.
- A claim about how code behaves, one you could prove by pointing at lines,
  goes to kof.
- Progress on a task that will span sessions goes to worklog.
- A session ending mid-task gets a worklog checkpoint, plus a handoff when
  the next agent needs the story and not only the state.

## The hooks behind it

Claude Code loads auto memory on its own. Everything else arrives through
belt:

| belt entrypoint | Claude Code event | Hint | What the agent gets |
|---|---|---|---|
| `belt hint session-start` | SessionStart | `agent-memory` | the agent-memory indexes |
| `belt hint session-start` | SessionStart | `kof-consult` | up to five assertions about the repo the session opened in |
| `belt hint search` | PostToolUse on the csl search tools | `kof-assertions` | up to three assertions about the code the search hit, stale ones marked |
| `belt hint prompt` | UserPromptSubmit | `kof-deposit` | once per session, after 30 tool calls with no `kof_assert`: a prefilled nudge to deposit what it derived |
| `belt hint session-start`, `belt hint subagent-start` | SessionStart, SubagentStart | custom hints | machine-private context from a command you configure, such as a daily journal or a worker brief |
| `belt record <event>` | nine lifecycle events | the ledger | nothing; it writes to events and never to the session |

Every hint prints under a `belt[<name>]:` prefix, so a transcript shows which
context was injected and which the agent produced. On Codex the reference
setup wires only the tool-event entries: `kof-assertions` fires there, while
the session-start injections and the deposit nudge stay Claude-only. It wires
no `belt record` entry either, so a Codex session leaves nothing in the
ledger; Codex can still read the ledger through the events service.

To turn all of this on, register the hooks in your agent settings. The
wiring is in [tools/belt/docs/hooks.md](../tools/belt/docs/hooks.md), and a
worked settings block is in
[`examples/dotfiles/recipes/agent-config/settings.json`](../examples/dotfiles/recipes/agent-config/settings.json).
Check the result with `belt doctor`, which also tells "kof down" apart from
"kof empty"; the hints render both as silence.
