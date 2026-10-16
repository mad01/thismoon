# operating factory-eval

{{.Bin}} is an experimental eval. It runs the factory (one
orchestrator session with a worker subagent per unit) and one single agent
on the same docs-sweep cases at the same dollar cap. Then it scores both
arms and applies the pass rule. One binary, no service, no store: each batch
writes one results file and exits.

## how it runs

`{{.Bin}} run` reads cases.yaml from the working directory. It works
through every run index, tier and arm. Each run clones the source repo into
a fresh temp dir at the pinned commit, removes the clone's origin remote,
and starts one headless `claude -p` session there. The session gets a
dollar cap, the dontAsk permission mode with prompts turned into denials,
and only the humanizer MCP server. Runs go one at a time. After each session {{.Bin}} checks every
case branch: branch present, only allowed paths changed, outline order,
humanizer findings, `make build test`. Then one Haiku judge call scores
each case's prose. A passing run's clone is deleted. A failing run's clone
is kept, and its path goes into the results as kept_dir.

`--dry-run` makes no model call. Stand-ins replace claude and the judge;
the checkouts, checks and results file are real.

## failure modes

"the cases file is not signed off": deliberate. A paid run needs
sign_off.by and sign_off.date in cases.yaml, set after the user approves
the cases. A dry run needs neither.

"the factory arm cannot run": the factory skill, the loom skill, one of the
three factory-worker definitions in ~/.claude/agents, or a SubagentStart
hook that runs belt is missing. The message lists each one. Run
`--arms single` for a baseline only, or install the missing pieces first.

"worst case exceeds the batch ceiling": every arm run at its tier cap plus
every judge call at its cap is above batch_ceiling_usd. Pass fewer
`--runs` or `--tiers`. Never raise the ceiling without the user.

stop=error_max_budget_usd in a run: the session hit its dollar cap before it
finished. The cases it did not reach fail the branch check. That counts as
a result, not a bug in {{.Bin}}.

stop=error with "no result message": claude printed no JSON. Usually it is
a login or flag problem. The error carries the last stderr lines; compare a
rejected flag with `claude --help`.

Every case fails the humanizer check with rule names you did not expect:
the installed humanizer pack differs from the one the cases were measured
with. Run `{{.Bin}} measure` and compare with the deviation block in
cases.yaml.

## first moves

1. `{{.Bin}} run --dry-run` from tools/factory-eval: prerequisites, the
   exact argv of both arms and the judge, and a results file
2. `{{.Bin}} measure`: the deviation table behind the case choice
3. `{{.Bin}} version -o json` to confirm which build is running
