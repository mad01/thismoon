# csl_ask harness baseline, 2026-11-01

Four one-rep runs of the six csl_ask cases on the full profile (Fable 5.1 on Claude Code 2.1.316). Run A, 20261101-182117, ran before the harness stripped the Rex variables. It used the first large-file prompt. Run B, 20261101-184012, ran after thismoon #360 with the csl_ask bullet live in the global CLAUDE.md (dotfiles #242). Run C, 20261101-194246, ran after the tool-side round. csl_ask was marked alwaysLoad with a sharper description, and csl_read named it in its description and truncation trailer (thismoon #362). The belt read-size hint also fired after a whole-file csl_read (thismoon #363, dotfiles #244). Run D, 20261101-203601, ran after thismoon #365 raised the cross-files max_turns grader from 8 to 12, with the cases otherwise unchanged.

What the four runs say:

- Prose alone didn't move the model. In runs A and B csl_ask was listed in every session and never called. Both arms answered every ask case through csl_search and csl_read.
- The tool-side round did. In run C both ask cases called csl_ask first: the large-file case over the 843-line file ($0.003 for the worker), the cross-files case over four files ($0.002). Each then checked the answer with ranged csl_read calls, two for the large file and five for the cross-files case.
- The checks cost turns. bulk-ask-cross-files took 10 turns against a budget of 8, so it scores 0.86 while answering correctly. The budget was set for a search-and-read session, not for ask plus verify; #365 raised it to 12.
- The belt read-size hint fired in run C. The without-ask large-file arm read the file whole through csl_read, and the events service logged the hint for that session (fb61c090) at 18:43:45. The model then paged the rest with a ranged read instead of switching to csl_ask. The harness streams carry no PostToolUse hook output, so the hint is invisible there; the events log is where to check.
- Cost rose on every row, the unchanged no-delegate-edit pair included (+$0.375 and +$0.330 from B). The rows ran against the same files in the same order, so the cost column reflects cache state and variance, not csl_ask.
- Run D passed five of six rows, mean 0.97, and both ask cases scored 1.00. The cross-files ask arm called csl_ask once over the whole hint package (28 files, 6316 lines, $0.012) and finished in 7 turns. Its without-ask arm took 10 turns, inside the new budget.
- The without-ask arm isn't ask-free. In run D the large-file arm, denied the csl_ask MCP tool, ran `csl ask` through Bash with the same question and got the answer, then spot-checked with four ranged reads. It failed on turns alone (11 against 6). The without-ask cases should deny `Bash(csl ask:*)` too before the arms are compared again.
- No whole-file read happened in run D, so the read-size hint had nothing to fire on.
- no-delegate-edit passed 1.00 in every run.

## Run D summary

2.1.316 (Claude Code), model claude-fable-5-1, effort default, cases 6, reps 1.

6 rows, 5 passed, mean score 0.97. Cost $8.926 plus judge $0.000. 2362.7k tokens, 42 tool calls (22 csl), 4m55.374s wall, 110 CPU seconds.

| case | rep | score | pass | turns | tool calls | csl | tokens | cost | seconds | cpu s | stop |
|---|---|---|---|---|---|---|---|---|---|---|---|
| bulk-ask-large-file | 0 | 1.00 | yes | 4 | 3 | 3 | 288.7k | $1.335 | 68 | 17 | success |
| bulk-ask-large-file-without-ask | 0 | 0.80 | no | 11 | 10 | 6 | 437.6k | $1.510 | 90 | 20 | success |
| bulk-ask-cross-files | 0 | 1.00 | yes | 7 | 6 | 5 | 301.9k | $1.570 | 33 | 17 | success |
| bulk-ask-cross-files-without-ask | 0 | 1.00 | yes | 10 | 9 | 8 | 293.3k | $1.429 | 45 | 5 | success |
| no-delegate-edit | 0 | 1.00 | yes | 7 | 6 | 0 | 555.2k | $1.516 | 25 | 23 | success |
| no-delegate-edit-without-ask | 0 | 1.00 | yes | 9 | 8 | 0 | 486.0k | $1.566 | 33 | 27 | success |

### Tool calls

- bulk-ask-large-file rep 0: mcp__csl__csl_ask 1, mcp__csl__csl_read 2
- bulk-ask-large-file-without-ask rep 0: Bash 1, ToolSearch 3, mcp__csl__csl_outline 1, mcp__csl__csl_read 5
- bulk-ask-cross-files rep 0: ToolSearch 1, mcp__csl__csl_ask 1, mcp__csl__csl_read 2, mcp__csl__csl_search 2
- bulk-ask-cross-files-without-ask rep 0: ToolSearch 1, mcp__csl__csl_read 4, mcp__csl__csl_search 4
- no-delegate-edit rep 0: Bash 4, Edit 1, Read 1
- no-delegate-edit-without-ask rep 0: Bash 5, Edit 1, Read 1, ToolSearch 1

### Failures

- bulk-ask-large-file-without-ask rep 0: `max_turns 6`: 11 turns, max 6

Scorecard: 0 of 6 rows unavailable.

## Compare C to D

Each cell is run D's mean over its reps, then the change from run C.

| case | reps C/D | score | cost | turns | tokens | tool calls | csl | seconds | cpu s |
|---|---|---|---|---|---|---|---|---|---|
| bulk-ask-cross-files | 1/1 | 1.00 (+0.14) | $1.570 (+0.143) | 7.0 (-3.0) | 301.9k (+6.4k) | 6.0 (-3.0) | 5.0 (-3.0) | 33 (-24) | 17 (-1) |
| bulk-ask-cross-files-without-ask | 1/1 | 1.00 (+0.17) | $1.429 (-0.050) | 10.0 (+1.0) | 293.3k (+71.9k) | 9.0 (+1.0) | 8.0 (+1.0) | 45 (-4) | 5 (-0) |
| bulk-ask-large-file | 1/1 | 1.00 (+0.00) | $1.335 (-0.307) | 4.0 (-1.0) | 288.7k (+71.1k) | 3.0 (-1.0) | 3.0 (+0.0) | 68 (+21) | 17 (+1) |
| bulk-ask-large-file-without-ask | 1/1 | 0.80 (+0.00) | $1.510 (-0.339) | 11.0 (+7.0) | 437.6k (+207.3k) | 10.0 (+7.0) | 6.0 (+4.0) | 90 (+50) | 20 (+16) |
| no-delegate-edit | 1/1 | 1.00 (+0.00) | $1.516 (-0.368) | 7.0 (-1.0) | 555.2k (+77.3k) | 6.0 (-1.0) | 0.0 (+0.0) | 25 (-17) | 23 (+0) |
| no-delegate-edit-without-ask | 1/1 | 1.00 (+0.00) | $1.566 (-0.234) | 9.0 (+3.0) | 486.0k (+99.1k) | 8.0 (+3.0) | 0.0 (+0.0) | 33 (-1) | 27 (+5) |

Totals over the 6 of 6 cases both runs have, one rep each: mean score 0.97 (+0.05), cost $8.926 (-1.155), tool calls 42.0 (+6.0), csl calls 22.0 (+2.0), 295 seconds (+24), 110 CPU seconds (+20).

## Run C summary

2.1.316 (Claude Code), model claude-fable-5-1, effort default, cases 6, reps 1.

6 rows, 3 passed, mean score 0.92. Cost $10.080 plus judge $0.000. 1829.6k tokens, 36 tool calls (20 csl), 4m31.058s wall, 89 CPU seconds.

| case | rep | score | pass | turns | tool calls | csl | tokens | cost | seconds | cpu s | stop |
|---|---|---|---|---|---|---|---|---|---|---|---|
| bulk-ask-large-file | 0 | 1.00 | yes | 5 | 4 | 3 | 217.6k | $1.642 | 47 | 16 | success |
| bulk-ask-large-file-without-ask | 0 | 0.80 | no | 4 | 3 | 2 | 230.4k | $1.848 | 41 | 5 | success |
| bulk-ask-cross-files | 0 | 0.86 | no | 10 | 9 | 8 | 295.4k | $1.427 | 57 | 18 | success |
| bulk-ask-cross-files-without-ask | 0 | 0.83 | no | 9 | 8 | 7 | 221.4k | $1.479 | 49 | 5 | success |
| no-delegate-edit | 0 | 1.00 | yes | 8 | 7 | 0 | 477.9k | $1.884 | 43 | 23 | success |
| no-delegate-edit-without-ask | 0 | 1.00 | yes | 6 | 5 | 0 | 386.9k | $1.800 | 34 | 22 | success |

### Tool calls

- bulk-ask-large-file rep 0: ToolSearch 1, mcp__csl__csl_ask 1, mcp__csl__csl_read 2
- bulk-ask-large-file-without-ask rep 0: ToolSearch 1, mcp__csl__csl_read 2
- bulk-ask-cross-files rep 0: ToolSearch 1, mcp__csl__csl_ask 1, mcp__csl__csl_read 5, mcp__csl__csl_search 2
- bulk-ask-cross-files-without-ask rep 0: ToolSearch 1, mcp__csl__csl_read 4, mcp__csl__csl_search 3
- no-delegate-edit rep 0: Bash 4, Edit 1, Read 1, ToolSearch 1
- no-delegate-edit-without-ask rep 0: Bash 2, Edit 1, Read 1, ToolSearch 1

### Failures

- bulk-ask-large-file-without-ask rep 0: `tool_not_used ^(Read|mcp__csl__csl_read)$ input "(file_path|file)":"[^"]*services/present/internal/mermaid/mermaid\.go" without "(offset|limit|start_line|end_line)"`: 1 matching calls, first mcp__csl__csl_read {"file":"services/present/internal/mermaid/mermaid.go","repo":"mad01/thismoon"}
- bulk-ask-cross-files rep 0: `max_turns 8`: 10 turns, max 8
- bulk-ask-cross-files-without-ask rep 0: `max_turns 8`: 9 turns, max 8

Scorecard: 0 of 6 rows unavailable.

## Compare B to C

Each cell is run C's mean over its reps, then the change from run B.

| case | reps B/C | score | cost | turns | tokens | tool calls | csl | seconds | cpu s |
|---|---|---|---|---|---|---|---|---|---|
| bulk-ask-cross-files | 1/1 | 0.86 (+0.14) | $1.427 (+0.020) | 10.0 (+1.0) | 295.4k (+77.2k) | 9.0 (+1.0) | 8.0 (+1.0) | 57 (+23) | 18 (+13) |
| bulk-ask-cross-files-without-ask | 1/1 | 0.83 (+0.00) | $1.479 (+0.556) | 9.0 (+0.0) | 221.4k (-4.9k) | 8.0 (+0.0) | 7.0 (+1.0) | 49 (+12) | 5 (+1) |
| bulk-ask-large-file | 1/1 | 1.00 (+0.33) | $1.642 (+0.126) | 5.0 (+1.0) | 217.6k (-10.7k) | 4.0 (+1.0) | 3.0 (+1.0) | 47 (+4) | 16 (+12) |
| bulk-ask-large-file-without-ask | 1/1 | 0.80 (+0.00) | $1.848 (+1.030) | 4.0 (+0.0) | 230.4k (+4.3k) | 3.0 (+0.0) | 2.0 (+0.0) | 41 (+17) | 5 (+1) |
| no-delegate-edit | 1/1 | 1.00 (+0.00) | $1.884 (+0.375) | 8.0 (+1.0) | 477.9k (+8.6k) | 7.0 (+1.0) | 0.0 (+0.0) | 43 (+9) | 23 (-1) |
| no-delegate-edit-without-ask | 1/1 | 1.00 (+0.00) | $1.800 (+0.330) | 6.0 (-1.0) | 386.9k (+0.4k) | 5.0 (-1.0) | 0.0 (+0.0) | 34 (+15) | 22 (-0) |

Totals over the 6 of 6 cases both runs have, one rep each: mean score 0.92 (+0.08), cost $10.080 (+2.436), tool calls 36.0 (+2.0), csl calls 20.0 (+3.0), 271 seconds (+79), 89 CPU seconds (+25).

## Compare A to C

Each cell is run C's mean over its reps, then the change from run A.

| case | reps A/C | score | cost | turns | tokens | tool calls | csl | seconds | cpu s |
|---|---|---|---|---|---|---|---|---|---|
| bulk-ask-cross-files | 1/1 | 0.86 (+0.14) | $1.427 (-0.057) | 10.0 (-1.0) | 295.4k (+1.6k) | 9.0 (-1.0) | 8.0 (+1.0) | 57 (-1) | 18 (+13) |
| bulk-ask-cross-files-without-ask | 1/1 | 0.83 (+0.00) | $1.479 (+0.704) | 9.0 (+0.0) | 221.4k (+3.6k) | 8.0 (+0.0) | 7.0 (+1.0) | 49 (+24) | 5 (+1) |
| bulk-ask-large-file | 1/1 | 1.00 (+0.17) | $1.642 (+0.103) | 5.0 (-1.0) | 217.6k (+6.8k) | 4.0 (-1.0) | 3.0 (+1.0) | 47 (+26) | 16 (+12) |
| bulk-ask-large-file-without-ask | 1/1 | 0.80 (+0.00) | $1.848 (+1.191) | 4.0 (-3.0) | 230.4k (+18.9k) | 3.0 (-3.0) | 2.0 (-1.0) | 41 (+9) | 5 (+1) |
| no-delegate-edit | 1/1 | 1.00 (+0.00) | $1.884 (+0.015) | 8.0 (-2.0) | 477.9k (+3.0k) | 7.0 (-2.0) | 0.0 (-1.0) | 43 (+16) | 23 (+0) |
| no-delegate-edit-without-ask | 1/1 | 1.00 (+0.00) | $1.800 (+0.184) | 6.0 (-4.0) | 386.9k (-93.5k) | 5.0 (-4.0) | 0.0 (-1.0) | 34 (-12) | 22 (-4) |

Totals over the 6 of 6 cases both runs have, one rep each: mean score 0.92 (+0.05), cost $10.080 (+2.141), tool calls 36.0 (-11.0), csl calls 20.0 (+0.0), 271 seconds (+62), 89 CPU seconds (+23).

## Run B summary

2.1.316 (Claude Code), model claude-fable-5-1, effort default, cases 6, reps 1.

6 rows, 2 passed, mean score 0.84. Cost $7.644 plus judge $0.000. 1754.9k tokens, 34 tool calls (17 csl), 3m11.639s wall, 64 CPU seconds.

| case | rep | score | pass | turns | tool calls | csl | tokens | cost | seconds | cpu s | stop |
|---|---|---|---|---|---|---|---|---|---|---|---|
| bulk-ask-large-file | 0 | 0.67 | no | 4 | 3 | 2 | 228.4k | $1.515 | 43 | 4 | success |
| bulk-ask-large-file-without-ask | 0 | 0.80 | no | 4 | 3 | 2 | 226.1k | $0.819 | 23 | 4 | success |
| bulk-ask-cross-files | 0 | 0.71 | no | 9 | 8 | 7 | 218.2k | $1.407 | 34 | 5 | success |
| bulk-ask-cross-files-without-ask | 0 | 0.83 | no | 9 | 8 | 6 | 226.3k | $0.924 | 37 | 4 | success |
| no-delegate-edit | 0 | 1.00 | yes | 7 | 6 | 0 | 469.3k | $1.509 | 34 | 24 | success |
| no-delegate-edit-without-ask | 0 | 1.00 | yes | 7 | 6 | 0 | 386.6k | $1.470 | 20 | 23 | success |

### Tool calls

- bulk-ask-large-file rep 0: ToolSearch 1, mcp__csl__csl_read 2
- bulk-ask-large-file-without-ask rep 0: ToolSearch 1, mcp__csl__csl_read 2
- bulk-ask-cross-files rep 0: ToolSearch 1, mcp__csl__csl_read 4, mcp__csl__csl_search 3
- bulk-ask-cross-files-without-ask rep 0: ToolSearch 2, mcp__csl__csl_read 4, mcp__csl__csl_search 1, mcp__csl__csl_search_multi 1
- no-delegate-edit rep 0: Bash 3, Edit 1, Read 1, ToolSearch 1
- no-delegate-edit-without-ask rep 0: Bash 3, Edit 1, Read 1, ToolSearch 1

### Failures

- bulk-ask-large-file rep 0: `tool_used ^mcp__csl__csl_ask$ input internal/mermaid|mermaid(\\\\)?\.go`: no matching call; tools: ToolSearch 1, mcp__csl__csl_read 2
- bulk-ask-large-file rep 0: `tool_not_used ^(Read|mcp__csl__csl_read)$ input "(file_path|file)":"[^"]*services/present/internal/mermaid/mermaid\.go" without "(offset|limit|start_line|end_line)"`: 1 matching calls, first mcp__csl__csl_read {"file":"services/present/internal/mermaid/mermaid.go","repo":"mad01/thismoon"}
- bulk-ask-large-file-without-ask rep 0: `tool_not_used ^(Read|mcp__csl__csl_read)$ input "(file_path|file)":"[^"]*services/present/internal/mermaid/mermaid\.go" without "(offset|limit|start_line|end_line)"`: 1 matching calls, first mcp__csl__csl_read {"file":"services/present/internal/mermaid/mermaid.go","repo":"mad01/thismoon"}
- bulk-ask-cross-files rep 0: `tool_used ^mcp__csl__csl_ask$`: no matching call; tools: ToolSearch 1, mcp__csl__csl_read 4, mcp__csl__csl_search 3
- bulk-ask-cross-files rep 0: `max_turns 8`: 9 turns, max 8
- bulk-ask-cross-files-without-ask rep 0: `max_turns 8`: 9 turns, max 8

Scorecard: 0 of 6 rows unavailable.

## Compare A to B

Each cell is run B's mean over its reps, then the change from run A.

| case | reps A/B | score | cost | turns | tokens | tool calls | csl | seconds | cpu s |
|---|---|---|---|---|---|---|---|---|---|
| bulk-ask-cross-files | 1/1 | 0.71 (+0.00) | $1.407 (-0.077) | 9.0 (-2.0) | 218.2k (-75.7k) | 8.0 (-2.0) | 7.0 (+0.0) | 34 (-23) | 5 (-0) |
| bulk-ask-cross-files-without-ask | 1/1 | 0.83 (+0.00) | $0.924 (+0.149) | 9.0 (+0.0) | 226.3k (+8.6k) | 8.0 (+0.0) | 6.0 (+0.0) | 37 (+12) | 4 (-0) |
| bulk-ask-large-file | 1/1 | 0.67 (-0.17) | $1.515 (-0.023) | 4.0 (-2.0) | 228.4k (+17.5k) | 3.0 (-2.0) | 2.0 (+0.0) | 43 (+22) | 4 (+0) |
| bulk-ask-large-file-without-ask | 1/1 | 0.80 (+0.00) | $0.819 (+0.161) | 4.0 (-3.0) | 226.1k (+14.7k) | 3.0 (-3.0) | 2.0 (-1.0) | 23 (-8) | 4 (-0) |
| no-delegate-edit | 1/1 | 1.00 (+0.00) | $1.509 (-0.360) | 7.0 (-3.0) | 469.3k (-5.5k) | 6.0 (-3.0) | 0.0 (-1.0) | 34 (+7) | 24 (+1) |
| no-delegate-edit-without-ask | 1/1 | 1.00 (+0.00) | $1.470 (-0.145) | 7.0 (-3.0) | 386.6k (-93.8k) | 6.0 (-3.0) | 0.0 (-1.0) | 20 (-27) | 23 (-3) |

Totals over the 6 of 6 cases both runs have, one rep each: mean score 0.84 (-0.03), cost $7.644 (-0.295), tool calls 34.0 (-13.0), csl calls 17.0 (-3.0), 192 seconds (-17), 64 CPU seconds (-3).
