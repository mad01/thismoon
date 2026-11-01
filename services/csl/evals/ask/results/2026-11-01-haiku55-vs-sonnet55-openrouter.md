# csl ask eval

Started 2026-11-01 15:37, 8 cases (6 train, 2 test), 3 reps, via -, prompt de149df974ad82ad.

Regraded 2026-11-01 16:30 with the cases as edited since; the answers are the original run's.

## Arms

| arm | model | effort | rows | errors | pass | mean score | forbidden | over budget | line claims wrong | compression | p50 ms | in tok | out tok | cost |
|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| haiku55 | anthropic/claude-haiku-5.5 | - | 24 | 0 | 100% | 1.00 | 0% | 0% | 0/167 | 0.024 | 2388 | 18177 | 404 | $0.048 |
| sonnet55 | anthropic/claude-sonnet-5.5 | - | 24 | 0 | 17% | 1.00 | 0% | 83% | 8/282 | 0.044 | 4325 | 18177 | 628 | $1.023 |

Pass needs every expected fact, no forbidden match, and an answer inside the budget. Compression is answer chars over corpus chars. Line claims are the path:line references the worker made and how many name an identifier that is not on that line. Cost is the list price of the tokens from the price table, with no cache discount; on the claude transport nothing is charged.

## Cases

| case | split | corpus | haiku55 | sonnet55 |
|---|---|---|---|---|
| belt-soft-mode-guards | train | 1 files, 1091 lines | 3/3 1.00 c=0.013 | 0/3 1.00 c=0.028 |
| belt-sweep-readsize-cross | train | 2 files, 855 lines | 3/3 1.00 c=0.029 | 0/3 1.00 c=0.053 |
| belt-validmode | train | 1 files, 1091 lines | 3/3 1.00 c=0.007 | 2/3 1.00 c=0.015 |
| csl-index-flags | test | 1 files, 666 lines | 3/3 1.00 c=0.046 | 1/3 1.00 c=0.070 |
| csl-repo-tools | train | 2 files, 656 lines | 3/3 1.00 c=0.031 | 0/3 1.00 c=0.059 |
| deps-no-backoff | test | 1 files, 683 lines | 3/3 1.00 c=0.031 | 0/3 1.00 c=0.051 |
| present-chart-diagram-kinds | train | 1 files, 1998 lines | 3/3 1.00 c=0.005 | 1/3 1.00 c=0.013 |
| speak-pause | train | 1 files, 875 lines | 3/3 1.00 c=0.032 | 0/3 1.00 c=0.059 |

A cell is passes/reps, the mean score, and the compression.

## Misses

- sonnet55 / belt-sweep-readsize-cross (0/3 pass)
  - wrong line claim: 678 named by "- **sweep.go:400-401**: the 'recursive' argument that 'parseSweep' passes in is 'walks'. I..."; line reads "if tool == \"find\" || tool == \"fd\" {"
- sonnet55 / belt-validmode (2/3 pass)
  - wrong line claim: 850 named by "- Callers (for context): 'custom_guards' entries (line 850), 'git_identity' rules (line 86..."; line reads "if err := validMode(cg.Mode); err != nil {"
  - wrong line claim: 862 named by "- Callers (for context): 'custom_guards' entries (line 850), 'git_identity' rules (line 86..."; line reads "if err := validMode(r.Mode); err != nil {"
  - wrong line claim: 867 named by "- Callers (for context): 'custom_guards' entries (line 850), 'git_identity' rules (line 86..."; line reads "if err := validMode(r.Mode); err != nil {"
  - wrong line claim: 1025 named by "- Callers (for context): 'custom_guards' entries (line 850), 'git_identity' rules (line 86..."; line reads "if err := validMode(mode); err != nil {"
  - wrong line claim: 1049 named by "- Defined at line 1044: 'func validMode(mode string) error {'. The doc comment is at lines..."; line reads "}"
- sonnet55 / csl-index-flags (1/3 pass)
  - wrong line claim: 25 named by "- Flags are registered in 'init()' at lines 58-77; backing variables are declared at lines..."; line reads "indexAllFlag         bool"
- sonnet55 / present-chart-diagram-kinds (1/3 pass)
  - wrong line claim: 247 named by "- Default: 'LR' (doc.go:79-80, doc.go:247)."; line reads "// sit inside another), Edges are labelled arrows between nodes, and"

