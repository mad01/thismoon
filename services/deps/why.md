# why deps

## The problem

The repos on this machine pin hundreds of external packages (Go modules, npm
lockfiles, exact Python pins, Swift resolved files), and a version that was
clean when it was pinned can grow an advisory later. On a single developer
machine there is no CI fleet re-checking old lockfiles, so a vulnerability in
an already-pinned version sits unnoticed until something else surfaces it. The
missing piece is a background check that keeps running after the pin is made
and says something only when a package is actually flagged.

## Why its own service

Advisory checking is a cross-repo concern: the repo set comes from the catalog
registry, so a repo enrolls the moment it is catalogued, and the findings only
make sense as one inventory across all of them. It also needs a long-running
process — a daily scan with wake-from-sleep catch-up and offline backoff is
not something a one-shot tool can provide. Per-ecosystem scanners run one repo
and one ecosystem at a time; deps checks every checkout against OSV.dev, one
API whose ecosystem strings cover Go, npm, PyPI, and Swift with no mapping
layer, and gives the human, the web page, and the agent the same findings from
the same scan.

## Why this shape

The load-bearing decision is single-writer: `deps serve` alone owns the scan
store, reaches the network, and fires notifications, while the CLI and the MCP
tools are thin HTTP clients to it. Every reader sees the same data and no file
locks exist. Acknowledgment is version-scoped: a resolve is keyed by
ecosystem, name, exact version, and advisory id, so an acknowledged finding
resurfaces automatically when the package version changes or a new advisory
lands — silence never becomes permanent. Scheduling checks staleness
rather than counting ticks: a Mac asleep past the daily mark runs the missed
scan shortly after waking, scan failures back off exponentially instead of
hammering an unreachable API, and notifications coalesce into one banner per
scan rather than one per advisory. For Go, import-graph reachability dims
advisories on modules no code compiles in, and it fails open so a real
advisory is never hidden.

## Non-goals

deps does not fix anything: no automatic version bumps, no generated pull
requests, only the fixed version named in each finding. It is not a CI gate
and blocks nothing. Reachability stays module-level and Go-only: there is no
function-level analysis, and npm, PyPI, and Swift packages are always treated
as compiled in. Repos outside the catalog registry are not scanned, and when
the machine is offline it waits rather than caching or guessing advisory
state.
