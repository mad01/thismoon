# csl ask eval

Started 2026-11-01 15:52, 8 cases (6 train, 2 test), 3 reps, via claude, prompt de149df974ad82ad.

Regraded 2026-11-01 16:30 with the cases as edited since; the answers are the original run's.

## Arms

| arm | model | effort | rows | errors | pass | mean score | forbidden | over budget | line claims wrong | compression | p50 ms | in tok | out tok | cost |
|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| opus55-low | claude-opus-5-5 | low | 24 | 0 | 46% | 1.00 | 0% | 50% | 5/246 | 0.038 | 5052 | 18700 | 425 | $1.999 |

Pass needs every expected fact, no forbidden match, and an answer inside the budget. Compression is answer chars over corpus chars. Line claims are the path:line references the worker made and how many name an identifier that is not on that line. Cost is the list price of the tokens from the price table, with no cache discount; on the claude transport nothing is charged.

## Cases

| case | split | corpus | opus55-low |
|---|---|---|---|
| belt-soft-mode-guards | train | 1 files, 1091 lines | 1/3 1.00 c=0.021 |
| belt-sweep-readsize-cross | train | 2 files, 855 lines | 1/3 0.96 c=0.036 |
| belt-validmode | train | 1 files, 1091 lines | 2/3 1.00 c=0.016 |
| csl-index-flags | test | 1 files, 666 lines | 1/3 1.00 c=0.082 |
| csl-repo-tools | train | 2 files, 656 lines | 0/3 1.00 c=0.058 |
| deps-no-backoff | test | 1 files, 683 lines | 3/3 1.00 c=0.034 |
| present-chart-diagram-kinds | train | 1 files, 1998 lines | 3/3 1.00 c=0.010 |
| speak-pause | train | 1 files, 875 lines | 0/3 1.00 c=0.044 |

A cell is passes/reps, the mean score, and the compression.

## Misses

- opus55-low / belt-sweep-readsize-cross (1/3 pass)
  - missing `(?i)more than two|third|three|> ?2|over two`
  - wrong line claim: 697 named by "- The search is recursive but every path is a regular file (sweep.go:697)."; line reads "return false"
- opus55-low / belt-validmode (2/3 pass)
  - wrong line claim: 1045 named by "- Accepted values (line 1045):"; line reads "if mode == \"\" || mode == ModeHard || mode == ModeSoft {"
- opus55-low / present-chart-diagram-kinds (3/3 pass)
  - wrong line claim: 1615 named by "- Any other name is refused by 'validateDiagramNodes' (doc.go:1615-1617)."; line reads "if n.Kind != \"\" && !slices.Contains(diagramKinds, n.Kind) {"

