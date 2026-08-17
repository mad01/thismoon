---
name: worklog
description: Save and resume cross-session work state for long, cross-repo tasks — keyed by ticket id or topic, not by working directory. Use when the user says "worklog", "checkpoint", "save my progress", "where was I", "pick up where I left off", or when a session is ending mid-task and progress should survive. Backed by the worklog MCP (worklog_checkpoint/list/search/show/status) with the `worklog` CLI as fallback.
argument-hint: "[ticket|topic|query]  — omit to list active items"
---

# worklog

Durable, searchable memory for work that spans sessions and repos. The user
often starts in a tmp dir and works across several repos on one ticket, then
stops and resumes days later. worklog keys that state on the **task** (a ticket
id or topic slug) so it's findable again — unlike Claude Code's per-directory
auto-memory, which orphans anything started in a tmp dir.

This is a different path from `/handoff` (which writes a one-off `$TMPDIR`
bridge doc). worklog is the durable, keyed, searchable store.

## Tools

Prefer the **MCP tools** (no shell prompt): `worklog_checkpoint`, `worklog_list`,
`worklog_search`, `worklog_show`, `worklog_status`. The `worklog` CLI is the
fallback if the MCP server isn't connected (`worklog checkpoint|list|search|show|status`).

When checkpointing via MCP, **always pass `cwd`** (the user's current working
directory) so the touched repo is recorded — the MCP server runs from `/` and
can't detect it otherwise.

## Modes

### Read — resume (default)

- **No argument** → `worklog_list` with `status: "active"`. Also run it with
  `repo: <current repo>` if the user is inside a git repo, to surface items
  touched here. Present the list; let the user pick one.
- **A key or query** → try `worklog_show` with the key first; if not found,
  `worklog_search` the term and show matches.
- After loading, read the item's `CONTEXT.md` ("Where I am" + recent Log) and,
  if re-entering a specific repo, `worklog_show` with that `repo` to pull its
  notes. Restore that context into the session before continuing.

### Write — checkpoint

Triggered by "checkpoint", "save my progress", "worklog this", or when a
session is wrapping up mid-task.

**Core principle: the checkpoint is the ONLY context a fresh session gets.**
Write `where` for a reader with zero prior knowledge of the task. A thin
checkpoint forces the next session to re-discover everything — wasting time and
risking different conclusions. Err on the side of too much detail, not too
little.

1. **Derive the key.** Prefer a ticket id if one is in play (`MAD-1234`); else a
   short stable topic slug. Confirm with the user if ambiguous.
2. **Gather state.** The `where` field must include ALL of the following that
   apply:
   - **Goal** — what the task is trying to achieve and why
   - **Repos** — full paths (host + local disk path), which code search tool
     covers each host, any repos not yet cloned that will be needed
   - **Conclusions and findings** — every conclusion reached, with the evidence
     and reasoning. Not just "X is confirmed" but "X is confirmed because
     querying Y returned Z during incident window W"
   - **Working tool/query examples** — exact tool names, parameters, query
     strings that produced results. A resuming session should be able to copy
     these verbatim, not re-derive them through trial and error
   - **Anti-patterns** — things that were tried and failed, with WHY they
     failed. Prevents the next session from repeating dead ends
   - **Decisions made** — what was decided and the reasoning. Include who
     decided and when if relevant
   - **Links** — tickets, RFCs, Google Docs, PRs, Jira comment IDs, SLO IDs,
     dashboard URLs. Anything a resuming session would need to look up
   - **External content that can't be re-fetched easily** — if the session
     generated replacement text, draft wording, or received important data from
     a tool that requires auth/context to reproduce, capture the substance
     inline rather than just referencing it
   - **Next steps** — concrete, actionable, ordered by priority. Include
     prerequisites (e.g., "clone repo X before starting step 3")
3. **Call `worklog_checkpoint`** with:
   - `key`
   - `where` — the full state snapshot (replaces "Where I am")
   - `note` — a log entry (what changed this session + remaining work list)
   - `cwd` — the user's working directory
   - `ticket` / `topic` — on first creation
4. Report the key and how to resume (`/worklog <key>`).

Durable *insights* (preferences, decisions, reusable facts) still go to
auto-memory — worklog holds resumable *task state*. "remember" stays mapped to
auto-memory; worklog triggers only on the verbs above.

### Manage

- Close out: `worklog_status` with `done` when the task ships.
- Pause: `worklog_status` with `paused` to keep it out of the active list.

## Notes

- The store is local-only (`~/code/worklog/`, no remote) — resume on the same
  machine. Don't promise cross-machine sync.
- One task spanning N repos is **one item** with N per-repo notes, not N items.
- **Ticket firewall:** a personal item (github.com/mad01 work) keys on a Linear
  key (`MAD-NN`) — legacy `mad01/issues` refs (`#NN`) are still recognized for
  older items; an item from any other tracker world (e.g. a day-job issue
  tracker) keys on that tracker's own key. Never reference one world's ticket
  from the other's item — the worlds stay strictly separated.
