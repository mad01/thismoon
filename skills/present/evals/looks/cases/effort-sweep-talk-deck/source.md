# csl ask eval

Started 2026-11-01 16:04, 8 cases (6 train, 2 test), 3 reps, via claude, prompt de149df974ad82ad.

Regraded 2026-11-01 16:30 with the cases as edited since; the answers are the original run's.

## Arms

| arm | model | effort | rows | errors | pass | mean score | forbidden | over budget | line claims wrong | compression | p50 ms | in tok | out tok | cost |
|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| haiku55-low | claude-haiku-5-5 | low | 24 | 0 | 79% | 0.96 | 0% | 12% | 4/201 | 0.027 | 2619 | 18725 | 400 | $0.050 |
| haiku55-medium | claude-haiku-5-5 | medium | 24 | 0 | 83% | 0.96 | 0% | 8% | 1/203 | 0.028 | 3377 | 18725 | 575 | $0.052 |
| haiku55-high | claude-haiku-5-5 | high | 24 | 0 | 67% | 0.99 | 0% | 29% | 1/219 | 0.030 | 5983 | 18725 | 1123 | $0.058 |
| haiku55-xhigh | claude-haiku-5-5 | xhigh | 24 | 0 | 75% | 1.00 | 0% | 25% | 3/221 | 0.030 | 7825 | 18725 | 2076 | $0.070 |
| haiku55-max | claude-haiku-5-5 | max | 24 | 0 | 62% | 1.00 | 0% | 38% | 3/250 | 0.036 | 54803 | 18725 | 14915 | $0.224 |

Pass needs every expected fact, no forbidden match, and an answer inside the budget. Compression is answer chars over corpus chars. Line claims are the path:line references the worker made and how many name an identifier that is not on that line. Cost is the list price of the tokens from the price table, with no cache discount; on the claude transport nothing is charged.

## Cases

| case | split | corpus | haiku55-low | haiku55-medium | haiku55-high | haiku55-xhigh | haiku55-max |
|---|---|---|---|---|---|---|---|
| belt-soft-mode-guards | train | 1 files, 1091 lines | 2/3 1.00 c=0.016 | 2/3 1.00 c=0.017 | 1/3 1.00 c=0.018 | 0/3 1.00 c=0.021 | 0/3 1.00 c=0.023 |
| belt-sweep-readsize-cross | train | 2 files, 855 lines | 2/3 1.00 c=0.032 | 2/3 1.00 c=0.031 | 3/3 1.00 c=0.028 | 3/3 1.00 c=0.030 | 3/3 1.00 c=0.032 |
| belt-validmode | train | 1 files, 1091 lines | 3/3 1.00 c=0.008 | 3/3 1.00 c=0.008 | 2/3 0.93 c=0.008 | 3/3 1.00 c=0.010 | 3/3 1.00 c=0.014 |
| csl-index-flags | test | 1 files, 666 lines | 2/3 1.00 c=0.061 | 3/3 1.00 c=0.062 | 1/3 1.00 c=0.067 | 1/3 1.00 c=0.075 | 0/3 1.00 c=0.097 |
| csl-repo-tools | train | 2 files, 656 lines | 3/3 1.00 c=0.039 | 3/3 1.00 c=0.037 | 3/3 1.00 c=0.034 | 3/3 1.00 c=0.034 | 2/3 1.00 c=0.041 |
| deps-no-backoff | test | 1 files, 683 lines | 1/3 0.67 c=0.027 | 1/3 0.67 c=0.028 | 2/3 1.00 c=0.036 | 2/3 1.00 c=0.029 | 3/3 1.00 c=0.031 |
| present-chart-diagram-kinds | train | 1 files, 1998 lines | 3/3 1.00 c=0.008 | 3/3 1.00 c=0.005 | 3/3 1.00 c=0.008 | 3/3 1.00 c=0.007 | 3/3 1.00 c=0.008 |
| speak-pause | train | 1 files, 875 lines | 3/3 1.00 c=0.029 | 3/3 1.00 c=0.034 | 1/3 1.00 c=0.039 | 3/3 1.00 c=0.036 | 1/3 1.00 c=0.041 |

A cell is passes/reps, the mean score, and the compression.

## Misses

- haiku55-low / belt-validmode (3/3 pass)
  - wrong line claim: 1045 named by "- '\"\"' (empty string) is accepted at line 1045 as the per-rule default."; line reads "if mode == \"\" || mode == ModeHard || mode == ModeSoft {"
  - wrong line claim: 1045 named by "- '\"\"' (line 1045): accepted, returns nil; the per-rule default."; line reads "if mode == \"\" || mode == ModeHard || mode == ModeSoft {"
- haiku55-low / deps-no-backoff (1/3 pass)
  - missing `(?i)will retry|\bdeliver\b`
  - wrong line claim: 255 named by "- Registry lookups (scanner.go:255) also have no retry; failed lookups are recorded as unk..."; line reads "answers := reg.Check(ctx, queries)"
- haiku55-medium / deps-no-backoff (1/3 pass)
  - missing `(?i)will retry|\bdeliver\b`
- haiku55-medium / present-chart-diagram-kinds (3/3 pass)
  - wrong line claim: 249 named by "- 'diagramDirections', line 81: LR, TB. Default: LR (lines 79-80; line 249)."; line reads "Direction string         'json:\"direction,omitempty\"'"
- haiku55-high / belt-validmode (2/3 pass)
  - missing `(?i)\berror\b`
- haiku55-high / present-chart-diagram-kinds (3/3 pass)
  - wrong line claim: 267 named by "- Diagram node kind default: empty 'kind' draws as 'service' (line 77, line 267)"; line reads "// Group the id of the group it sits in, and Step the 1-based step it"
- haiku55-xhigh / belt-soft-mode-guards (0/3 pass)
  - wrong line claim: 704 named by "- 'validated()' (lines 820-829) discards the parsed file when 'validate()' returns an erro..."; line reads "if src.Broken() {"
- haiku55-xhigh / present-chart-diagram-kinds (3/3 pass)
  - wrong line claim: 316 named by "- 'chartKinds', lines 66-68: \"bar\", \"line\", \"area\", \"sparkline\", \"stacked-bar\", \"horizonta..."; line reads "}"
  - wrong line claim: 497 named by "- 'diagramDirections', line 81: \"LR\", \"TB\". Default: \"LR\" (lines 79-80, 248). Empty passes..."; line reads "// normalizeNames applies textNorms until the text stops changing, so it is"
- haiku55-max / belt-validmode (3/3 pass)
  - wrong line claim: 1045 named by "- '\"\"' (empty string) is accepted at line 1045 as the per-rule default."; line reads "if mode == \"\" || mode == ModeHard || mode == ModeSoft {"
  - wrong line claim: 1045 named by "- '\"\"' (empty string) is accepted at line 1045 and is the per-rule default."; line reads "if mode == \"\" || mode == ModeHard || mode == ModeSoft {"
  - wrong line claim: 1045 named by "- '==' at line 1045 is an exact comparison, so no trimming or case folding is applied."; line reads "if mode == \"\" || mode == ModeHard || mode == ModeSoft {"

