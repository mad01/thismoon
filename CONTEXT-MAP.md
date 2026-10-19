# thismoon context map

The domain glossary is split by context. Each CONTEXT.md applies to its own
directory and everything below it; the root one applies to the whole repo.

| Context | Glossary | Covers |
|---------|----------|--------|
| thismoon | [CONTEXT.md](CONTEXT.md) | Repo-wide terms: component, service, tool, recipe, mod, release tags |
| keeper-of-facts | [services/keeper-of-facts/CONTEXT.md](services/keeper-of-facts/CONTEXT.md) | Assertions, evidence pins, stale and retracted status |
| present | [services/present/CONTEXT.md](services/present/CONTEXT.md) | Briefs, decks, slides, steps, charts and diagrams, the shared instance |
| loom | [skills/loom/CONTEXT.md](skills/loom/CONTEXT.md) | Claimed worktrees and the weaver |
| factory | [skills/factory/CONTEXT.md](skills/factory/CONTEXT.md) | Orchestrator, units, workers, and the board |
| suspenders | [tools/suspenders/CONTEXT.md](tools/suspenders/CONTEXT.md) | Allowlist, safe references, ignores and rule exclusions |

The factory builds on the loom vocabulary, and the scope stays as it is by
design: a path-scoped check of a file under `skills/factory/` applies the
factory and root glossaries only. Factory worker messages are checked without
a path, so they get every glossary.
