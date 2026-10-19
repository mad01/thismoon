# operating factory-eval

{{.Bin}} is an experimental eval. It runs the factory (one
orchestrator session with a worker subagent per unit) and one single agent
on the same docs-sweep cases at the same dollar cap. Then it scores both
arms and applies the pass rule. One binary, no service, no store: each batch
writes one results file and exits.

## how it runs

`{{.Bin}} run` reads cases.yaml from the working directory. It works
through every run index, tier and arm. Each run clones the source repo into
a fresh temp dir at the pinned commit, named in resolved form (no symlink
component), the form claude records the session cwd in. The clone keeps no
branch, tag, reflog or remote, and its local config holds the factory-eval
git identity. One headless `claude -p` session starts there with a dollar
cap, the dontAsk permission mode with prompts turned into denials, only the
humanizer MCP server, and a settings file. The file repeats the arm's
allow and deny lists from the command line, since settings-file rules are
documented to reach subagents. No paid run has yet shown that the
factory's workers obey them; a worker's `make -C ... test` passing would,
since only an allow rule admits it. Its own deny rules keep Edit, Write, `git
-C`, `make -C` and `go build -o` out of ~/code, ~/.worktrees and the
source repo; reads stay allowed. That is not a sandbox. {{.Bin}} fingerprints the source checkout
before and after the session, and a change taints the run.

Runs go one at a time. After each session {{.Bin}} checks every case
branch: branch present, only allowed paths changed, outline order,
humanizer findings, `make build test`. Then one Haiku judge call scores
each case's prose, with one retry; a second failure errors the run. A
passing run's clone is deleted. A
failing run's clone is kept, and its path goes into the results as
kept_dir.

`--dry-run` makes no model call. Stand-ins replace claude and the judge;
the checkouts, checks and results file are real.

## failure modes

"the cases file is not signed off": deliberate. A paid run needs
sign_off.by, sign_off.date (YYYY-MM-DD) and sign_off.digest in cases.yaml,
set after the user approves the cases. The dry run prints the digest.
"the file changed after the sign-off" means cases.yaml was edited since:
show the user the change and get a new sign-off. A dry run needs none.

"the factory arm cannot run": the factory skill, the loom skill, one of the
three factory-worker definitions in ~/.claude/agents, or a SubagentStart
hook that runs belt is missing. The message lists each one. Run
`--arms single` for a baseline only, or install the missing pieces first.

"worst case exceeds the batch ceiling": the worst case is above
batch_ceiling_usd. It counts every arm run at its tier cap, plus two judge
calls per case at the judge cap: the call and its retry. Pass fewer
`--runs` or `--tiers`. Never raise the ceiling without the user.

stop=error_max_budget_usd in a run: the session hit its dollar cap before it
finished. The run counts as errored, so its tier is incomplete and fails.
That is a result, not a bug in {{.Bin}}.

"tainted, the source checkout changed" in a run: HEAD, the status, the
worktrees or a ref of the source checkout changed during the session. The
run counts as errored. Read the listed change. A commit in any worktree of
the source repo, loom worktrees included, taints a run, and so does a
fetch. Rerun against a dedicated clone no session works in:
`git clone <source> ~/factory-eval-source`, then
`{{.Bin}} run --repo ~/factory-eval-source`.

"FAIL at tier ...: incomplete": an arm has fewer runs without error than
--runs asked for. The runs array says which run errored and why.

"the session ended with is_error set" in a run: the result said success
but carried is_error, the shape of an API error that ended the turn. The
run counts as errored. Rerun once the API recovers.

"the judge could not score <case>" in a run: the judge failed twice on a
branch that exists. The run counts as errored rather than scoring 0. The
case's judge_error holds the reason.

"provisional, 1 of the cases file's 3 runs per arm": the batch ran fewer
runs than cases.yaml asks for. Treat the decision as a first look.

"temp dir ... is under a path the arms may not edit": --temp-dir points
into ~/code, ~/.worktrees or the source repo. Pick a dir outside them.

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
