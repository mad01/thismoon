# Ask eval

A rerunnable measurement of the worker model behind `csl_ask`, the bulk
reader. The bulk reader takes a question over files that a lexical search
selected and has a cheaper model answer it, so the files never enter the
calling agent's context. The question the eval answers is whether that
worker gets the facts right, and at what size and cost. Run it before
picking the worker model, and again when the prompt or the model changes.

## Running it

```
cd services/csl
go run ./evals/ask -list             # the sign-off document, no network
go run ./evals/ask -oracle           # grading gates, no network
go run ./evals/ask -arms haiku55 -reps 1 -filter validmode   # one paid call
go run ./evals/ask -arms haiku55 -reps 3 -split train        # one tuning round
go run ./evals/ask -arms haiku55,sonnet55                    # one matrix, both splits
go run ./evals/ask -via claude -arms opus55-low              # one arm through the claude CLI
go run ./evals/ask -regrade evals/ask/runs/<ts>              # rescore stored answers, no network
```

Both offline commands still run `csl search` to resolve each case's corpus,
so csl must be on PATH with the repo indexed. The corpus files are read from
the checkout csl indexes, the same place `csl_read` reads them.

A run writes `runs/<timestamp>/`. Inside: `summary.md`, and `results.jsonl`
with one row per case, arm and rep, the answer and its grade included.
`errors.jsonl` holds the attempts that produced no answer. `run.json`
records the arms, the reps, the prompt hash and every case's resolved
corpus. `traces/` keeps every reply with the provider's usage. Git ignores
`runs/`. When a run settles a decision, copy its `summary.md` into
`results/` under a dated name.

## Transports

The runner picks where the calls go, in this order, unless `-via` names
one. A LiteLLM proxy when `LITELLM_BASE_URL` is set (`LITELLM_API_KEY` is
optional). Then the `claude` CLI when it is on PATH. Then OpenRouter when
`OPENROUTER_API_KEY` is set. OpenRouter is last on purpose: a work machine
has LiteLLM or Claude Enterprise, a personal one has the Claude
subscription, and the metered key is the fallback for both. The runner
reads those variables from its own environment, and the shell does not
source the secrets file. Run `source ~/.config/ralph/secrets.sh` first to
reach LiteLLM or OpenRouter. Without it, auto lands on the claude CLI.

The `claude` transport runs `claude -p` headless, with no tools, hooks or
settings, on whatever account the CLI is signed into. On a subscription its
rows cost nothing extra. The cost column is the list price of the tokens
from the price table on every transport, with no cache discount, so arms
compare fairly whatever order they ran in. The CLI caches the prompt, and
its own cost figure falls for every arm after the first; that figure stays
in the trace. The summary header names the transport a run used. Its
latency includes the CLI's startup, so compare p50 within one transport
only.

Arms name models by their canonical id (`claude-haiku-5-5` and so on) and
`run.go` defines them, with their effort, beside the price table. `haiku55`
is the candidate worker, `sonnet55` and `opus55-low` the ceilings. No local
model is an arm. The worker is meant to run beside other hosted agents, and
a local model would measure a machine, not the tool.

## Cases

One markdown file per case: YAML frontmatter, then the question.

```
---
id: belt-validmode
title: What validMode accepts
split: train
query: 'sym:validMode'
anchors: [tools/belt/internal/config/config.go]
max_files: 1
max_answer_chars: 700
expected:
  - '\bhard\b'
  - '\b1044\b'
forbidden:
  - '(?i)\bmedium\b'
oracle: |
  - validMode (tools/belt/internal/config/config.go:1044) accepts "", "hard" and "soft".
notes: why it is here and what makes it hard
---
Which mode values does validMode accept, and on which line is it defined?
```

`query` is a zoekt query. The files it matches, in rank order and up to
`max_files`, are the corpus; `file` narrows it with a path regex the way
`--file` does. Lexical search is the driver on purpose: the corpus a case
sees is the corpus `csl_ask` would build from the same query. `files` is an
explicit path list for an ad-hoc run and skips the search. `anchors` are
paths the resolved corpus must include, so a query that drifts fails loudly.

`expected` are regexes the answer must match, one fact each. `forbidden` are
regexes a correct answer never matches: the names and values a worker
invents when it stops reading. `max_answer_chars` is the compression budget,
and an answer over it fails whatever it contains. That is what rejects a
worker that echoes the corpus back. `oracle` is the hand-written ideal
answer. It is the sign-off document and the grading sanity check.

Two rules for new cases. Write every expected fact from the file, with the
line open, never from a worker's answer. And give each case something to
get wrong: a line number, a vocabulary with near-misses, or a function that
does not exist.

## Gates

`-oracle` runs three checks per case before anything is paid for, and
`go test` runs the same three offline from the anchors. The oracle must
pass with no wrong line claim. An empty answer must match no fact. The
corpus echoed back must fail the budget.

## Reading the summary

The arm table carries the headline per arm. Pass needs every expected fact,
no forbidden match, and an answer inside the budget. The mean score is the
share of facts found, which separates a near miss from a blank. The
forbidden and over-budget rates stay in their own columns so a trade
between them is visible. Compression is answer chars over corpus chars, the
share of the files the caller's context pays for. Line claims count the
`path:line` references the worker made and how many name an identifier that
is not on that line. That number is the invented line reference, measured.
Tokens come from the provider's usage block, and cost from the price table
on every transport.

Eight cases at three reps is a small sample, and a difference of one case
between arms is noise. The per-case grid and the misses section are where
the comparison happens. A fact every rep misses is either a worker gap or a
pattern written too tightly, and the trace says which.
