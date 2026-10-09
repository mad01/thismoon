# Judge eval

A rerunnable measurement of the `humanizer judge` pass. Every labeled case
under `cases/` goes through the production `judge.Run` path on each
configured arm, several times over. The run then reports accuracy and
stability per arm, with latency and cost beside them. The rubric and the
model both change over time, and the CLAUDE.md note asks for a spread
re-measure whenever they do. Run this before changing the default model,
the rubric, or the confidence bands.

## Running it

```
cd tools/humanizer
source ~/.secrets.sh                 # exports OPENROUTER_API_KEY
go run ./evals/judge -list           # the sign-off document, no network
go run ./evals/judge -oracle         # grading sanity check, must score 100%
go run ./evals/judge -null likely_ai # grading sanity check, must score the base rate
go run ./evals/judge -arms haiku55 -reps 1 -filter slop   # one paid call
go run ./evals/judge -arms haiku55 -reps 3 -split train   # one tuning round
go run ./evals/judge                 # the full matrix, both splits
```

A run writes `runs/<timestamp>/`. Inside: `summary.md`, `results.jsonl`
with one row per case, arm, and rep, and `errors.jsonl` for attempts that
produced no verdict, each with a failure class. `run.json` records the arms,
the reps, and the rubric with its hash. `traces/` keeps every raw model
reply, including the `confidence_basis` the parser drops. Git ignores
`runs/`. When a run settles a decision, copy its `summary.md` into `results/`
under a dated name so the next run has something to compare against.

All arms go through OpenRouter. `run.go` defines them: Haiku 4.5 pinned to
temperature 0 (the production shape before MAD-391), Haiku 4.5 at the
provider default, Haiku 5.5 at the provider default, and Haiku 5.5 with
`reasoning_effort` low. Add an arm there when a new model or knob needs a
number.

## Cases

One markdown file per case, frontmatter then the passage:

```
---
id: human-totem-readme
label: likely_human
bucket: human
source: github.com/mad01/totem README.md, commit abc123 (2018-09-07)
license: owned by the repo author
generator: ""
words: 180
notes: why it is here and what makes it hard
---
The passage, 60 to 350 words.
```

`label` is the expected verdict, `likely_human` or `likely_ai`, or `unknown`
for the ambiguous bucket. `split` is `train` (the default when omitted) or
`test`. `-split test` selects the held-out cases that no rubric change was
tuned on. The summary shows the split beside each case's bucket. The `human` and `ai` buckets carry clean
provenance. Human text was written before LLM assistants existed. AI text
was generated on a recorded date by a recorded model, never the model under
test. Anchors and the known false-positive shapes go in `hard`. The
`ambiguous` bucket holds humanized drafts whose provenance is
AI but whose reading is open; those run and get reported, never graded. The
word count in the frontmatter is informational, since the runner recounts.

Two rules for new cases. Never label a case by what the current judge says
about it, because that trains the eval to reward the incumbent. And never
add a passage that can't live in a public repository.

## Reading the summary

The arm table carries the headline per arm. Accuracy counts `mixed` as a
miss, because the skill treats it as a rewrite target. Recall on AI rows,
specificity on human rows, and the false-positive rate stay in separate
columns so a trade between them stays visible. The stability numbers (flip
rate, agreement, confidence spread) are computed per case across reps,
which is what a paired comparison between arms should read. Latency is the
final HTTP call only. Tokens and cost come from the provider's usage block,
summed over every call the row made, so a contract retry is paid for in the
number.

Twenty scored cases at five reps put the accuracy noise floor near ten
points, and differences inside that band aren't findings. When tuning the
rubric, run rounds on the train split only and read the held-out split once
at the end, on every arm at the full rep count. A rubric that wins on
train and loses on test was fitted to the cases, not to the problem. The per-case grid
is where the real comparison happens. Look for cases whose majority verdict
differs between arms, and for cases whose reps disagree.
