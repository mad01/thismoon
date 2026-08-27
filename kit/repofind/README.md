# repofind

Shared git repo discovery and remote parsing. This package is the common
file-tree layer under the suspenders pre-commit guard and the belt write-time
firewall: both derive their internal-name block lists by walking checkouts,
and the two lists only agree because both tools walk and parse identically.
That agreement is the package's reason to exist: treat its semantics as a
compatibility surface, not an implementation detail.

## What it does

- `Find(dirs, excludes)` walks every directory concurrently (32 workers),
  records each git repo it hits, and returns them sorted as
  `Repo{Path, Name, Remote}`. `Name` is the `org/repo` extracted from the
  origin remote; `excludes` are globs matched against that name.
- `ParseRemote` handles both remote shapes, ssh (`git@host:org/repo.git`)
  and https (`https://host/org/repo.git`), and strips the `.git` suffix.
- `IsRepo`, `InsideWorkTree`, and `ExpandHome` are the small helpers the
  consumers share.

## The name-derivation contract

Consumers turn discovered repos into blocked names the same way: each repo
contributes its **org segment**, its **repo segment**, and its **checkout
directory basename** as three separate names — never the combined
`org/repo`. Segments are lowercased and deduplicated. suspenders applies
this in its guard (`tools/suspenders/internal/guard`), belt in
`write-internal-names` (`tools/belt/internal/guard/writenames.go`, which
adds a minimum-length floor). Changing how this package walks or parses
changes both tools' derived block sets at once — which is the point, and
also the caution.

## Who uses it

| Consumer | Walks | For |
|---|---|---|
| suspenders `hook install --all` | config `dirs` | which repos get git hooks |
| suspenders guard | `guard.workspace_dirs` | the internal-name block list |
| belt `write-internal-names` | `internal_names.workspace_dirs` | the same block list, at Write/Edit time |

csl deliberately does **not** use this walker. Indexing scans every checkout
on the machine often, so csl carries its own subprocess-free BFS
(`services/csl/internal/repo/finder`) that reads `.git/config` directly and
also captures the host; it imports only `ExpandHome` from here. The two
walkers agree on the shape of the result (stop at the first `.git`, name
from the origin remote, directory-name fallback) without sharing code — see
`services/csl/docs/architecture.md`, "Repo discovery".

Everything above is walk-time discovery, run while a tool builds a name set
or an index. Per-invocation code (belt's hooks, the pre-commit script) never
walks; it resolves the repo it is standing in via the origin remote of the
nearest enclosing checkout.
