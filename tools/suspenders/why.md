# why suspenders

## The problem

A secret is easiest to stop on the machine where it gets committed; a
server-side scanner finds it after the remote already holds it. On a single
developer machine two things need to be blocked at commit time: checked-in
tokens, keys, and certificates, and internal org and repo names leaking into
public repos on a machine where internal and public checkouts sit side by
side. There is also the after-the-fact case: a secret committed and deleted
long ago is invisible to any working-tree scan, yet still sits in every
clone's history.

## Why its own tool

No single existing tool combines the three jobs: offline secret scanning
(with the rule set validated against GitLeaks, GitHub Secret Scanning, and
TruffleHog), the internal-reference guard, and hook orchestration so one
config owns pre-commit and post-merge across every discovered repo. The
guard in particular can only exist as a local tool: its block list is
derived per run from the repos actually checked out under `workspace_dirs`
and never persisted, because a config file enumerating internal names would
itself be the leak it is meant to prevent. And per the history-rewrite
decision (tools/suspenders/docs/adr/0002), a standalone offline tool with no runtime
dependencies is the point — everything runs against local git data, nothing
phones home.

## Why this shape

Staged scans read content from the git index via `git show :<path>`, not the
working tree, so the scan sees exactly what would be committed; a staged
file that cannot be read fails the scan instead of being skipped, because
the unreadable file is exactly the one that could carry a secret unchecked
(tools/suspenders/docs/adr/0001). History clean is a native `git fast-export | transform |
git fast-import` pipeline rather than a wrapper around git-filter-repo:
wrapping would have been less code, but it drags in a Python dependency;
a mandatory backup bundle, a round-trip test, and end-to-end reachability
tests contain the correctness risk of the in-house transform
(tools/suspenders/docs/adr/0002). Hook scripts are thin dispatchers that back up a
foreign hook and chain to it, so suspenders can own the hook events in every
repo without destroying what was already installed.

## Non-goals

suspenders makes no network calls and does no server-side or CI enforcement;
it runs where the commit happens. It does not guard the agent session before
anything reaches git — that is belt's layer, which derives its name list the
same way from its own config. It does not try to stop a user who deliberately bypasses it:
`git commit --no-verify` is the explicit, auditable override, preferred over
silently weakening the scan. Coverage of the derived block list ends at what
is checked out locally; names that exist only elsewhere must be listed as
`blocked_words`.
