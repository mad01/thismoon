# Routing

The orchestrator picks a model and an effort for every unit before it writes
the task. The rules below name model tiers, not models, so the mapping can
change without touching them.

## Model tiers

| Tier | Model today | Alias in the header |
|------|-------------|---------------------|
| top | Opus | `opus` |
| mid | Sonnet | `sonnet` |
| cheap | Haiku | `haiku` |

When a new model generation ships, change this table and nothing else.

## Defaults by work-on tier

| work-on tier | Model tier | Effort | Worker definition |
|--------------|------------|--------|-------------------|
| trivial | mid | low | `factory-worker-low` |
| known-domain | mid | medium | `factory-worker-medium` |
| novel | top | high | `factory-worker-high` |

A trivial unit is a mechanical change with an obvious done criterion: a
broken link, a renamed term, a one-line config value. A known-domain unit
follows a pattern the repo already has, such as a docs sweep against the
shared outline or a new grader beside existing ones. A novel unit needs a
design decision that no file in the repo already makes.

No default route uses the cheap tier. It is in the table so that an override
can name it.

Reviewers do not follow this table. A review or verifier subagent runs on
the model the panel-policy mod configures (`reviewModel`, Opus by default);
see `mods/panel-policy/README.md`.

## Overrides

The orchestrator may override the model, the effort, or both for one unit,
for example a known-domain unit that touches release configuration. The
override goes into the header: `model` and `effort` always carry the result,
never the default. The worker copies them into every message it sends.

An effort override changes the worker definition, because each definition
fixes one effort. A model override alone keeps the definition and passes the
model on the spawn.
