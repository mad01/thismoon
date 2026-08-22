# ADR-0009: components carry an embedded operating doc for agents

**Date:** 2026-08-21
**Status:** Accepted

**Scope:** every component under `services/` and `tools/`, plus a new
`kit/agentdoc` package. Decided after a five-reviewer design panel ran two
rounds over the proposal.

## Context

Coding agents are first-class users of the platform's MCP servers and CLI
tools, and they use them on machines where this repo is not checked out.
When a tool misbehaves there, the agent has nothing to consult: the
operating knowledge lives in per-component `why.md`, `architecture.md`, and
CLAUDE.md gotcha sections, all of which require the source tree, or in the
maintainer's head.

The binary is the only artifact guaranteed present wherever the tool is
used, so the doc has to travel inside it. Embedding also fixes a staleness
problem repo docs cannot: an embedded doc describes the binary it shipped
in, while a repo doc describes HEAD.

Two facts about the platform shaped the surface choices. Every MCP
server here is a stdio process spawned from the component binary's own
`mcp` subcommand, separate from the backing HTTP service. When the service
is down, the MCP stays connected and keeps returning tool errors, so
anything attached to the MCP connection (its `instructions`, its error
strings) survives the most common failure. And agents do not go looking
for docs. Panel walkthroughs of real failures showed the surfaces an agent
actually reads, in order: the error string in front of it, the system
prompt it already has, and `--help`. An MCP resource (`docs://...`) was in
the original design and was cut for exactly this reason: agents rarely read
resources unprompted, and a resource is only reachable when the server is
healthy enough that it is barely needed.

The remaining gap is results that are wrong without erroring, an empty
search result being the canonical case. No error fires, so no pointer
fires. The real fix is richer zero-result payloads on the tools themselves;
that is separate work, and until it lands the instructions text has to
carry the hint.

## Decision

Each component keeps one `operating.md` beside `why.md` and
`architecture.md`: 50 to 80 lines, written for an agent with no checkout,
compiled into the binary with `go:embed` (toss-bin, being Swift, gets the
doc codegenned into a string constant at build time). The shared rendering
lives in `kit/agentdoc`. Four surfaces expose it, ranked by how reliably
they sit on an agent's failure path:

1. **Error-string pointers.** Errors that cross the MCP tool-handler or
   CLI boundary carry a suffix naming the next step, formatted by
   `agentdoc.Hint`: `(run '<bin> doctor' to diagnose)`, or a pointer to
   `<bin> docs` while the component has no doctor. Wrapping happens at
   chokepoints (tool-handler returns, the backing-service HTTP client),
   not at individual error sites.
2. **`<bin> doctor`.** Executable checks beat prose and rot visibly. A
   shared `kit/doctor` helper covers the near-identical checks for
   serve-bearing components: service reachable, store readable, and a skew
   check comparing the running service's `/version` to the probing
   binary's own build metadata, which is the only surface that catches the
   known "new binary on disk, old process serving" failure. Doctor is
   required for serve-bearing components, enforced in CI against an
   explicit component list (directory inference does not work; some tools
   carry MCP subcommands). CLI-only tools add one when they earn it.
3. **`<bin> docs`.** Prints the rendered operating doc. Also listed in
   `--help`, which agents do read.
4. **MCP `instructions`.** A fixed skeleton from `agentdoc.Instructions`,
   three lines, hard-capped: what the tool is, that it is a stdio shim
   over a local service that must be running, and "on any tool error or
   unexpected empty result: run doctor, full doc under docs". Ten servers
   inject this into every session whether or not anything fails, so
   failure-mode prose stays out of it. The "unexpected empty result"
   wording is deliberate: it is the only surface covering failures that
   produce no error.

Mechanical facts (default base URL, store path) render into the doc, the
instructions, and doctor from the same `agentdoc.Facts` struct, populated
from the component's own constants, so the checkable facts cannot drift
from the code. Machine overlays (ADR-0006) mean the binary cannot know a
machine's actual wiring; the doc states defaults and defers to doctor for
live values.

## Consequences

- A component now ships four doc files. The split holds because each
  answers a different question: `why.md` the rationale, `architecture.md`
  the structure for someone with the source, `operating.md` how to run and
  debug it without the source, CLAUDE.md how to develop it. Operational
  gotchas that accumulated in component CLAUDE.md files move into
  `operating.md`, with CLAUDE.md keeping a reference line; each fact lives
  in one file.
- Rollout is two waves. Wave one is mechanical and covers every component
  in one PR series: the doc, the `docs` subcommand, instructions, and the
  chokepoint error hints. Uniformity is the point; a surface present on
  half the fleet trains agents not to rely on it. Wave two is `kit/doctor`
  plus doctor commands for the serve-bearing components and the CI list
  check.
- Prose beyond the rendered facts rots silently, and no CI check can catch
  it. A doc-touched-when-code-touched nag was considered and rejected as
  rubber-stamp bait for a solo maintainer. The docs stay short, and the
  periodic repo-wide docs sweep is the honesty pass.
- The doc's advice must match the binary it ships in: "run doctor first"
  appears only once the component has a doctor, which `agentdoc` gates on
  a `HasDoctor` fact rather than trusting prose to stay consistent.
- A stale stdio MCP process whose binary was replaced mid-session stays
  invisible to the skew check; it is self-consistent with its own
  instructions, so the harm is low and accepted.
- The missing-binary case is out of scope. If the binary is not installed,
  every surface here is absent with it; that is a provisioning problem,
  not a docs problem.
