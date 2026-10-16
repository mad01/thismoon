# loom, one worktree per session, one weaver for the merges

Two or more agent sessions editing the same checkout cannot each run git:
staging interleaves and a stray `git add -A` sweeps another session's
half-finished work into an unrelated commit. The old answer was a commit
queue: every session described its commit as a request file and one
committer session landed them serially. Worktrees remove the shared tree
instead: each session owns an isolated checkout of its own branch and commits
locally whenever it likes. The only thing left to coordinate is which
branch lands on the default branch, in what order. That sequencing role is
the **weaver**.

Git is the whole state store. There is no queue directory, no status
frontmatter, nothing to garbage-collect: `git worktree list` says what
exists, `git status` says what is in flight, `git log main..branch` says
what a branch offers, `git merge-tree` says whether main already holds it.
Every question the
weaver asks is answered live from git. Messages and belt's ledger only say
where to look: an owner's ready message is a claim, and git is the proof.

## Layout

Worktrees live under `~/.worktrees/<repo>/<slug>`:

- `<repo>` is the repo directory's basename (`thismoon`, not the org).
- `<slug>` is the branch name with `/` replaced by `-`, so
  `alexander/opener-multi-url` claims `~/.worktrees/thismoon/alexander-opener-multi-url`.

The directory sits outside `~/code` on purpose: code indexers and repo
walkers that sweep the checkout roots never see it, so worktrees do not show
up as duplicate repos. The flip side: searches over the canonical checkout
do not see a worktree's uncommitted work either.

Claiming is always explicit and always from the canonical checkout:

```
git -C <canonical> worktree add ~/.worktrees/<repo>/<slug> -b <branch>
```

Drop `-b` to claim an existing branch. Nothing auto-creates worktrees; a
session claims one when it starts a task and the claim ends when the branch
lands.

## States

The weaver classifies each worktree from git alone:

- **In flight**: the working tree is dirty (`git -C <wt> status
  --porcelain` has output). Someone is mid-task; leave it alone.
- **Ready**: clean, and ahead of the default branch
  (`git -C <wt> log --oneline <default>..HEAD` is non-empty), not
  integrated, and its owner has sent a ready message (or, with no weaver
  session, told the user). Cleanliness alone is not readiness: only the
  owner knows whether the last commit was the last commit. The message
  alone is not readiness either: git has to show what it claims.
- **Integrated**: clean, at least one commit beyond the default branch
  (`git log <default>..<branch>` is non-empty), and the default branch
  already holds the changes, by a squash or rebase merge. Both halves
  count. A merged tree equal to the default branch's tree is the proof
  (see the integrated check below). A branch with no commits beyond the
  default branch passes that proof trivially. The worktree is a leftover;
  remove it.
- **Abandoned**: clean or dirty, untouched for a long time, owner gone.
  Report it to the user with its branch and last-commit age; never remove
  or resolve it without their say-so.

A clean worktree with no commits beyond the default branch is none of
these. It is a fresh claim or a fast-forward leftover, and git cannot tell
the two apart. Report it, and remove it only on its owner's ready message
or the user's go-ahead.

Check in flight first, then integrated, then ready. A squash merge leaves
the branch's commits unreachable from the default branch, so
`git log <default>..<branch>` still lists them and `git branch --merged`
never lists the branch.

## Integrated check

The same check factory-band runs for a ready worker, from the canonical
checkout after a fetch (`main` stands for the default branch):

```
git -C <canonical> merge-tree --write-tree origin/main <branch>  # the tree a merge would write
git -C <canonical> rev-parse 'origin/main^{tree}'               # the default branch's own tree
```

Two equal tree ids: integrated, when the branch also has commits beyond
main (see States). Read the printed output, not the exit code: anything
but a first line equal to the default branch's tree id is no proof. A
different tree means a merge would still change the default branch. A
conflict exits 1 and lists the conflicted files under the tree id, but
exit 1 also means a bad ref (`not something we can merge`), and unrelated
histories exit 128. `merge-tree --write-tree` needs git 2.38 or later; an
older git has no proof, so every branch reads as not integrated there.
`git cherry` does not close the gap: its per-commit patch ids match a
rebase merge or a one-commit squash only.

The default branch may have changed the branch's lines again since the
squash. The merge into its tip then conflicts although the branch landed.
Only when the tip check conflicts, run the same check against each
first-parent commit since the fork point; the squash commit answers with
equal trees:

```
base=$(git -C <canonical> merge-base origin/main <branch>)
if [ -n "$base" ]; then
  for c in $(git -C <canonical> rev-list --first-parent --reverse "$base"..origin/main); do
    merged=$(git -C <canonical> merge-tree --write-tree "$c" <branch> 2>/dev/null)
    tree=$(git -C <canonical> rev-parse "$c^{tree}")
    [ -n "$merged" ] && [ "$merged" = "$tree" ] && { echo "integrated at $c: $merged $tree"; break; }
  done
fi
```

That line, with its two equal tree ids, is the proof: the branch landed at
that commit, even if the default branch later changed or reverted those
lines. No line printed means not integrated. No fork point means no walk:
the branch shares no history with the default branch.

## Survey

The weaver gathers the claims first, then checks each one against git.

1. Ready messages. Owners send them over SendMessage, so they arrive in the
   weaver's own conversation. The factory skill owns the format; the weaver
   reads three body fields: files changed, gates run (command and last
   line), and commit range.
2. The ledger. belt records each subagent's start and stop, auto-mode
   denials and notifications under the `belt-ledger` source, tagged with
   the repo, worktree and branch. A stop with no ready message, or a
   denial, points at a worker that ended or got stuck without claiming
   ready:

   ```
   events list --source belt-ledger --q <canonical> --limit 50
   ```

3. The proof, from the canonical checkout (`main` stands for the default
   branch):

   ```
   git -C <canonical> fetch origin
   git -C <canonical> worktree list --porcelain
   git -C <wt> status --porcelain                    # empty: clean
   git -C <canonical> log --oneline origin/main..<branch>  # the commits on offer
   git -C <canonical> merge-tree --write-tree origin/main <branch>  # commits on offer and equal to the next line: integrated
   git -C <canonical> rev-parse 'origin/main^{tree}'
   git -C <canonical> diff --stat origin/main...<branch>   # the files they touch
   ```

A ready claim holds when the tree is clean, the branch is not integrated,
the commit range ends at the branch tip (`git rev-parse <branch>`), and
the files changed match the diff stat. When it doesn't, the branch is not
ready: tell its owner what differs.

## Integration

Default order is oldest ready branch first. When one branch builds on
another, the dependent branch is rebased onto its parent by its owner and
becomes ready only after the parent lands. Dependencies are stated by the
user, not stored anywhere.

Per ready branch, in its own worktree:

1. Update the default branch in the canonical checkout
   (`git -C <canonical> pull --ff-only`).
2. Rebase the branch onto the fresh default tip. Rebase, not merge
   commits: linear history keeps each path-scoped conventional commit
   meaningful to release tooling that reads the default branch
   (release-please here).
3. Re-run the repo's gates in the worktree. The rebase may have composed
   this branch with work it never saw.
4. Land it by repo policy:
   - **PR-only repos** (this one): push the branch, open a PR, merge it
     when the user directs. After the merge, pull the canonical checkout.
   - **Direct-main repos**: `git -C <canonical> merge --ff-only <branch>`,
     then push if the repo has a remote.

A rebase conflict goes back to the branch's owner, who resolves it in
their worktree and reports ready again; the weaver moves on to the next
branch meanwhile. Only when the owner session is gone may the weaver
resolve it, and only with the user's go-ahead.

## Teardown

After a branch lands, and for every worktree the survey found integrated:

```
git -C <canonical> worktree remove ~/.worktrees/<repo>/<slug>
git -C <canonical> merge-tree --write-tree origin/main <branch>  # the proof: this line and the next
git -C <canonical> rev-parse 'origin/main^{tree}'               # print the same tree id
git -C <canonical> branch -d <branch>
```

Show the proof's output on the lines above the delete. `branch -d` comes
first. It refuses a squash-merged branch: the branch's commits are not
reachable from the default branch, and its upstream is usually gone after
the merge. Only then may the weaver run `git branch -D <branch>`, and only
when the proof above it printed two equal tree ids, from the tip check or
from the first-parent walk. A branch integrated only through the walk
prints unequal trees on the two tip lines. Its proof is the walk's
`integrated at` line: run the walk and print that line above the delete.
Without that proof `-D` would eat work: keep the branch and report it. On
git before 2.38 there is no proof, so `-d` is the only delete.
`git worktree prune` clears any stale administrative entries left by a
worktree removed by hand.

Before removing a worktree, harvest anything durable that points into it:
evidence-pinned assertions (kof) record absolute paths, so a pin made in a
worktree dies with it. Re-pin against the canonical checkout.

## Sweep

The Agent tool's isolation mode makes worktrees outside this layout:
`<canonical>/.claude/worktrees/agent-<id>` on branch `worktree-agent-<id>`.
Git locks each one with the session's pid. When the agent finishes, the
tool can leave both the tree and the branch behind; it removes an
unchanged one in some cases. The weaver lists them on every pass.
Removal waits for the user's explicit go-ahead.

List them:

```
git -C <canonical> branch --list 'worktree-agent-*'
git -C <canonical> worktree list --porcelain            # entries under .claude/worktrees/, with any locked line
git -C <canonical> log --oneline origin/main..<branch>  # per branch: empty means nothing beyond main
git -C <canonical> merge-tree --write-tree origin/main <branch>  # per branch with commits: the integrated check
ps -p <pid> -o lstart=                                  # per lock: is its agent still running?
```

The lock reason is the one the Agent tool wrote. Git prints it today as
`locked claude agent agent-<id> (pid <pid> start <time>)`; read the pid and
start time from it, and expect the format to change. A lock whose pid
still runs, with the start time the reason gives, belongs to a live agent.
Leave it. A pid that runs with another start time was reused, and the
agent is gone.

Report one line per branch: its worktree if it has one, whether that is
locked and whether the lock's pid still runs, and its commits beyond main.
Offer a branch for removal when it has nothing beyond main or is
integrated; report any other with its commits.

After the go-ahead, per branch, worktree first (git refuses to delete a
branch a worktree has checked out). The listing may be old by the time the
user answers, so check the lock's pid again right before
`git worktree unlock`:

```
ps -p <pid> -o lstart=                                            # the reason's pid and start time: stop, the agent is live
git -C <canonical> worktree unlock <canonical>/.claude/worktrees/agent-<id>
git -C <canonical> worktree remove <canonical>/.claude/worktrees/agent-<id>
git -C <canonical> log --oneline origin/main..worktree-agent-<id>  # the proof
git -C <canonical> merge-tree --write-tree origin/main worktree-agent-<id>  # with commits listed: the proof goes on
git -C <canonical> rev-parse 'origin/main^{tree}'
git -C <canonical> branch -d worktree-agent-<id>
```

Show the proof's output on the lines above the delete. An empty log means
nothing beyond main can be lost, and `-d` deletes such a branch even with
no upstream. Commits listed with two equal tree ids mean a squash merge:
`-d` refuses it, and `-D` follows under the Teardown rule. Any other
refusal, from `-d` or from `worktree remove` on a dirty tree: keep it and
report it to the user.

## Known frictions

Worktrees are still unusual enough locally that some tooling assumes one
checkout per repo:

- Path-matched write guards (belt's `write-internal-names`) match the
  canonical checkout path, so a write that is exempt there can be denied in
  a worktree. That is machine-config territory; a repo file cannot fix a
  guard (ADR-0012).
- `suspenders history clean` refuses to run while linked worktrees exist.
  Tear the loom down first.
- suspenders' pre-commit hook is safe: hooks resolve through
  `git rev-parse --git-path hooks`, which is shared across worktrees, so
  one `suspenders hook install` covers them all.
- The Agent tool's isolation mode puts its worktrees inside the checkout,
  at `.claude/worktrees/agent-<id>`, rather than under
  `~/.worktrees/<repo>/`, and can leave them behind. The Sweep section
  above cleans up after it.
- csl (code-search-local) may index a linked worktree under the repo as a
  second repo. Check with `csl_repo_lookup` and exclude it if so; the
  `~/.worktrees` layout exists to keep worktrees out of its walk.
- A nested spawn without its own isolation runs in its parent's worktree.
  Two agents then share one tree and one index, the problem worktrees
  exist to remove. Give every agent that edits files its own worktree.
