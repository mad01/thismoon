# why status

## The problem

The `*.this` platform runs a growing set of services as launchd agents under
t-man, and nothing on the machine says whether they are actually up. A service
can crash-loop quietly, hang while its port stops answering, or hit the worst
case this platform has documented: ralph installs a new binary, reports ok,
and the old process keeps serving. Each failure surfaces only when a page
fails to load, and by then there is no history to say when it started. status
puts every t-man-managed service on one dashboard at `http://status.this/`,
with a 30-day uptime strip per service.

## Why its own service

Monitoring cannot live inside the things being monitored: a wedged service
will not report itself wedged, and the stale-binary case is invisible from
inside the process by definition. It is also not a t-man feature, because
t-man's job is lifecycle (start, restart, KeepAlive) and it succeeds at that
even while an outdated process runs; catching drift requires an outside
observer comparing what runs against what is installed. Off-the-shelf uptime
monitors watch remote endpoints and assume incidents, on-call, and a cloud
account. This is one machine and loopback ports.

## Why this shape

Discovery is zero-config: every cycle scans the launchd plist directories for
`TManMetadata.ManagedBy == "t-man"`, so a new `t-man add` appears on the next
cycle and no service list exists to go stale. Version drift is detected by
comparing the running process's `/version` sha with the sha the on-disk
binary reports, and a mismatch must hold for two consecutive cycles before it
counts, so a mismatch observed mid-install clears instead of alerting.
History is a day-bucketed JSON file under `~/.local/share/status`, written
atomically each cycle and pruned at 90 days — local uptime bars do not need a
time-series database. The dashboard is a chrome-only shell rendered
client-side from `/api/status` with webkit primitives (docs/adr/0005).

## Non-goals

No incident tracking: the page shows running state and history, not outage
writeups. No alert routing beyond a coalesced macOS banner and an event on
events.this. No remote hosts, no probing anything that is not a local
t-man-managed service. status observes but never manages: starting,
restarting, and removing services stays with t-man. A day without recorded
checks renders gray, not red; absence of data is not treated as downtime.
