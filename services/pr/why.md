# why pr

## The problem

Open pull requests scatter across repos on more than one GitHub host:
github.com plus GitHub Enterprise instances, each with its own notification
page and its own login. Keeping up with review duty means visiting every host
and every watched repo in turn, and nothing on the machine shows the whole
queue at once. pr aggregates open PRs from a configured watch list into one
local dashboard at `http://pr.this/`, with rendered diffs and the common
review actions (approve, request changes, squash-merge) on the page.

## Why its own service

The multi-host part is the point: GitHub's own UI stops at its host boundary,
so a cross-host queue has to be assembled locally. Doing that well needs a
background poller and a cache, which means a long-running service rather than
a CLI invoked on demand. It does not belong in another component either:
catalog indexes what exists on disk and status watches local processes, while
pr watches remote review state, a different data source on a different clock.
And rather than becoming another GitHub client with its own token store, it
rides the authentication the machine already has.

## Why this shape

Every GitHub call shells out to `gh api` with `GH_HOST` set per source. There
is no direct HTTP client and no token handling in pr itself; if `gh` is
authenticated against a host, pr can watch it, and multi-host support falls
out of one environment variable. The direct cost is that the service runs
unsandboxed, since a seatbelt profile would block the subprocess. The list
and detail pages are chrome-only shells rendered client-side from JSON APIs
with webkit primitives, the platform-wide model (docs/adr/0005). The watch
list itself is machine-private: which repos a given machine follows lives as
a companion recipe in the consuming repo, while this repo's recipe stays
portable — the layering docs/adr/0006 describes, with pr's config as its
cited example.

## Non-goals

pr is a review dashboard, not a code-review platform. Actions are whole-PR:
approve, request changes with a message, squash-merge; there is no line-level
commenting, no PR authoring, and no branch management. It does not receive
webhooks or push updates — a poller refreshes sources on an interval, and
freshness is bounded by `poll_interval`. It does not replace `gh`: anything
beyond the dashboard's surface is a `gh` command away, using the same
authentication.
