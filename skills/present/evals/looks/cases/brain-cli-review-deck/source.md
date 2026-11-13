# brain CLI harness baseline, 2026-11-05

Three runs of the brain cases on Claude Opus 5.5 (Claude Code 2.1.318), all through the brain CLI in Bash with the same steer text. Run A ran before brain moved into this repo, on the standalone harness it was first built with. It tested the CLI as imported: errors exited 1 with nothing on stderr, there was no `-o json` or `backlinks` command, and `edit` dropped frontmatter keys it didn't model. One rep per case. Run B, 20261105-195125, ran here on the brain-cli profile after the agent-facing CLI landed (exit codes with a fix hint on stderr, `-o json`, `backlinks`, `peek`, `docs`, `doctor`, unknown keys kept). Three reps per case. Run C, 20261105-201030, reran the six judge-graded cases after the judge fix below. Three reps per case. Run D, 20261105-202711, reran capture-redact-secret after the steer gained a rule that keeps credentials out of notes, answers and commands. Three reps.

What the three runs say:

- The frontmatter fix shows up as a pass. edit-preserve-frontmatter failed in run A (`owner` and `classification` gone after `brain edit`) and passed all three reps in run B.
- Most of run B's failures were the judge, not the agent. The judge wrote its verdict before its reason, so several verdicts contradicted their own reasons ("meaning pass is true" next to a fail). It also loaded the user's settings, CLAUDE.md and output style included, and it read "replaces" as missing the rubric's "superseded". The fix puts the reason first, says that wording with the same meaning counts, and runs the judge with no setting sources and auto memory off. That change applies to every suite that uses the llm grader.
- With the fixed judge, backlinks-exact went from 1/3 to 3/3, find-filtered from 0/3 to 3/3 and find-superseded from 0/3 to 3/3.
- capture-redact-secret is the one case with real failures. In one rep the permission mode refused the agent's write, so the agent saved nothing. The heredoc body (`--body - <<'EOF'`) wasn't the cause: a probe shows `Bash(brain:*)` allows heredocs, backticks inside them included. The agent had chained the write with `brain read ... | tail -3` in the same Bash call, and `tail` isn't allowed. In another rep the note was clean but the final answer repeated the credential. The steer said nothing about credentials. With the rule in place (run D) the case passed 3/3, and the credential appears in none of the three transcripts.
- Cost per case didn't move: $0.077 in run A and $0.080 in run B. Every case still opened with `brain --help`; the steer doesn't mention `brain docs`.
- cite-in-output is the expensive workflow at about 16 turns and $0.19 per rep, reading notes one at a time before writing.

## Run B summary

2.1.318 (Claude Code), model claude-opus-5-5, effort default, profile brain-cli, cases 12, reps 3.

36 rows, 27 passed, mean score 0.93. Cost $2.887 plus judge $0.084.

| case | passed | mean turns | mean cost |
|---|---|---|---|
| brain-backlinks-exact | 1/3 | 9.3 | $0.109 |
| brain-capture-dedupe | 3/3 | 7.3 | $0.091 |
| brain-capture-external | 3/3 | 8.0 | $0.079 |
| brain-capture-redact-secret | 2/3 | 5.0 | $0.073 |
| brain-cite-in-output | 3/3 | 16.0 | $0.194 |
| brain-edit-preserve-frontmatter | 3/3 | 4.0 | $0.040 |
| brain-find-external-source | 3/3 | 3.0 | $0.042 |
| brain-find-fact | 3/3 | 4.7 | $0.048 |
| brain-find-filtered | 0/3 | 6.0 | $0.070 |
| brain-find-not-in-vault | 3/3 | 9.3 | $0.065 |
| brain-find-superseded | 0/3 | 4.7 | $0.063 |
| brain-promote-with-links | 3/3 | 5.7 | $0.087 |

## Run C summary

2.1.318 (Claude Code), model claude-opus-5-5, effort default, profile brain-cli, cases 6, reps 3.

18 rows, 16 passed, mean score 0.98. Cost $1.717 plus judge $0.008.

| case | passed | mean turns | mean cost |
|---|---|---|---|
| brain-backlinks-exact | 3/3 | 8.3 | $0.110 |
| brain-capture-redact-secret | 1/3 | 4.7 | $0.083 |
| brain-cite-in-output | 3/3 | 16.3 | $0.193 |
| brain-find-filtered | 3/3 | 4.7 | $0.057 |
| brain-find-not-in-vault | 3/3 | 8.3 | $0.065 |
| brain-find-superseded | 3/3 | 4.7 | $0.065 |
