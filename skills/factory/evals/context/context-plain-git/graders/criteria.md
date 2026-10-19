---
type: llm
weight: 2
---
The reply is the next message from a factory worker that got the worker brief at start. You see only the reply, so the fixture is repeated here.

Fixture: step 2 of the brief says "Run pwd. A ~ in the task header stands for your home directory. If pwd prints your worktree path, run plain commands there. If not, run git -C <worktree> <subcommand>, make -C <worktree>/<dir> <target> and go test -C <worktree>/<dir> ./..., and name files by their absolute path under your worktree. Put no option other than -C between git and its subcommand." Task: unit adr-links/catalog, worktree ~/.worktrees/thismoon/alexander-adr-links-catalog, branch alexander/adr-links-catalog, model sonnet, effort low. Its report format asks for a ready message that lists the files changed and each git command under the label git commands. Facts: pwd printed the canonical checkout ~/code/src/github.com/mad01/thismoon, and each Bash call starts there again. The link in services/catalog/README.md is fixed; `humanizer detect services/catalog/README.md` printed "no findings"; that one file was staged and committed as 4b7e0c2..e91d3a5; the worktree is clean.

PASS if all of these hold:

1. The reply is a ready message for unit adr-links/catalog, with needs_reply: false.
2. It lists services/catalog/README.md, the gate with "no findings", and the range 4b7e0c2..e91d3a5.
3. A git commands field lists at least the add and the commit of services/catalog/README.md.
4. Each git command runs in the worktree: it is written git -C ~/.worktrees/thismoon/alexander-adr-links-catalog <subcommand>.
5. No git command puts another option between git and its subcommand, for example -c <key>=<value> or --no-pager.

FAIL if any of these hold:

- a git command has an option other than -C before its subcommand
- a git command has no -C with the worktree path, so it would run in the canonical checkout
- a git command line starts with cd, which the brief forbids
- a fact is wrong or missing
- the reply claims a push, a merge or a pull request

Do not fail for any of these:

- the worktree path written with ~ or as an absolute path
- several git commands joined with && on one line, each with -C
- extra read-only git commands, such as git status, git diff or git log, when they also carry -C with the worktree path
- any commit message text
- the commands with or without backticks
- a short message
