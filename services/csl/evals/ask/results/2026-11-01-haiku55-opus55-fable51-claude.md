# csl ask eval

Started 2026-11-01 20:57, 8 cases (6 train, 2 test), 3 reps, via claude, prompt de149df974ad82ad.

## Arms

| arm | model | effort | rows | errors | pass | mean score | forbidden | over budget | line claims wrong | compression | p50 ms | in tok | out tok | cost |
|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| haiku55 | claude-haiku-5-5 | - | 24 | 0 | 79% | 0.99 | 0% | 8% | 2/185 | 0.027 | 3725 | 20041 | 615 | $0.055 |
| opus55 | claude-opus-5-5 | - | 24 | 0 | 29% | 0.98 | 0% | 71% | 6/253 | 0.037 | 4952 | 20016 | 464 | $2.144 |
| fable51 | claude-fable-5-1 | - | 24 | 0 | 50% | 0.98 | 0% | 46% | 1/213 | 0.035 | 7261 | 20039 | 435 | $5.331 |

Pass needs every expected fact, no forbidden match, and an answer inside the budget. Compression is answer chars over corpus chars. Line claims are the path:line references the worker made and how many name an identifier that is not on that line. Cost is the list price of the tokens from the price table, with no cache discount; on the claude backend nothing is charged.

## Cases

| case | split | corpus | haiku55 | opus55 | fable51 |
|---|---|---|---|---|---|
| belt-soft-mode-guards | train | 1 files, 1140 lines | 3/3 1.00 c=0.015 | 0/3 1.00 c=0.021 | 0/3 1.00 c=0.024 |
| belt-sweep-readsize-cross | train | 3 files, 1316 lines | 0/3 0.89 c=0.018 | 0/3 0.85 c=0.027 | 0/3 0.89 c=0.026 |
| belt-validmode | train | 1 files, 1140 lines | 3/3 1.00 c=0.009 | 1/3 1.00 c=0.016 | 2/3 0.93 c=0.009 |
| csl-index-flags | test | 1 files, 666 lines | 3/3 1.00 c=0.057 | 0/3 1.00 c=0.085 | 1/3 1.00 c=0.085 |
| csl-repo-tools | train | 2 files, 663 lines | 3/3 1.00 c=0.038 | 0/3 1.00 c=0.063 | 0/3 1.00 c=0.059 |
| deps-no-backoff | test | 1 files, 683 lines | 3/3 1.00 c=0.031 | 3/3 1.00 c=0.031 | 3/3 1.00 c=0.032 |
| present-chart-diagram-kinds | train | 1 files, 1998 lines | 3/3 1.00 c=0.006 | 3/3 1.00 c=0.011 | 3/3 1.00 c=0.010 |
| speak-pause | train | 1 files, 875 lines | 1/3 1.00 c=0.046 | 0/3 1.00 c=0.044 | 3/3 1.00 c=0.034 |

A cell is passes/reps, the mean score, and the compression.

## Misses

- haiku55 / belt-soft-mode-guards (3/3 pass)
  - wrong line claim: 706 named by "- 'validate()' (line 886) joins this error, and 'validated()' (line 869) then discards the..."; line reads "if src.Broken() {"
- haiku55 / belt-sweep-readsize-cross (0/3 pass)
  - missing `\b13\b`
- haiku55 / belt-validmode (3/3 pass)
  - wrong line claim: 1094 named by "- '\"\"' (empty string) is accepted at line 1094, meaning the per-rule default."; line reads "if mode == \"\" || mode == ModeHard || mode == ModeSoft {"
- opus55 / belt-sweep-readsize-cross (0/3 pass)
  - missing `\b13\b`
  - missing `(?i)more than two|third|three|> ?2|over two`
  - wrong line claim: 697 named by "- A recursive search where every path is an existing regular file (sweep.go:697)."; line reads "return false"
  - wrong line claim: 697 named by "- When every path is a regular file (sweep.go:697)."; line reads "return false"
- opus55 / belt-validmode (1/3 pass)
  - wrong line claim: 1094 named by "- Accepted values (line 1094):"; line reads "if mode == \"\" || mode == ModeHard || mode == ModeSoft {"
- opus55 / present-chart-diagram-kinds (3/3 pass)
  - wrong line claim: 1615 named by "- Any other name is refused by 'validateDiagramNodes' (doc.go:1615-1617)."; line reads "if n.Kind != \"\" && !slices.Contains(diagramKinds, n.Kind) {"
  - wrong line claim: 1615 named by "- Any other name is refused by 'validateDiagramNodes' (lines 1615-1617)."; line reads "if n.Kind != \"\" && !slices.Contains(diagramKinds, n.Kind) {"
- fable51 / belt-sweep-readsize-cross (0/3 pass)
  - missing `\b13\b`
  - wrong line claim: 702 named by "- 'isRegularFile' — sweep.go:701 stats the path and requires 'st.Mode().IsRegular()'; an..."; line reads "if p == \"\" {"
- fable51 / belt-validmode (2/3 pass)
  - missing `(?i)\berror\b`

