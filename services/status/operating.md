# operating status

status is the fleet status page: one process, `status serve`, discovers every
t-man-managed launchd job, probes it once a minute, and serves a dashboard
with a 30-day uptime strip per service (default {{.BaseURL}}). It observes
only; starting, restarting, and removing services stays with t-man.

## how it runs

There is no service list to configure. Each cycle scans the launchd plist
directories (~/Library/LaunchAgents and /Library/LaunchDaemons) for jobs with
`TManMetadata.ManagedBy == "t-man"`, so a new `t-man add` appears on the next
cycle. A plist with `--port N` in its arguments gets an HTTP probe of
`http://127.0.0.1:N/` (any response below 500 counts as up); a job without a
port gets a `launchctl print` PID check. Every 10 minutes the poller also
fetches each up service's `/version` and runs the on-disk binary's own
`version` command, comparing the running build against the installed one.
Machines usually also route http://status.this to the dashboard via the local
domain front door; if the localhost port answers but the .this host does not,
the router is the problem, not this service.

## where config and state live

No config file: flags and environment are the whole surface (`--port`, env
STATUS_PORT; `--workdir`, env STATUS_WORKDIR; `--interval`, `--routes`,
`--restart-window`, `--restart-threshold`). Uptime history is one file,
`history.json` under {{.StorePath}}: day-bucketed ok/fail counts, written
atomically each cycle, pruned at 90 days. Everything else (current snapshot,
version metadata, drift and crash-loop counters) is in memory and resets on
restart. status also reads d-man's `~/.config/d-man/routes.toml` to link each
web service to its `.this` page; it never writes that file.

## failure modes

Status page unreachable: the status process itself is down. t-man supervises
it: `t-man list`, then `t-man restart status`. For a quick look without
t-man, `status serve` in a spare terminal also works.

A service shown down that is actually up: the probe target does not match how
the service really listens. The HTTP probe hits the `--port` value found in
the plist, so a port moved in machine-private config, or a service that
answers its real routes but returns 500 on `/`, reads as down. Check the
plist's ProgramArguments against the port the service binds.

A service shown "Stale binary": the running process serves an older sha than
the binary installed on disk, the install-without-restart class. Verify with
the service's own `/version`, then restart it via t-man. The mismatch must
hold for 2 consecutive cycles before it shows, so a flap seen mid-install
clears on its own. Drift detection skips a sibling whose plain `version`
output is not a bare token, so such a service never shows as stale.

`t-man stop` will not show as Down: KeepAlive relaunches the job within a
second, faster than the poll interval. Down means a real failure: crash
loop, hung process, port not answering, or a removed job.

Gray days in the uptime strip: no checks were recorded that day, usually
because status itself was not running. Absence of data is not downtime.

## version skew

`status version -o json` reports the build of the binary on PATH. `GET
{{.BaseURL}}/version` reports the build the running serve process came from.
When the `commit` values differ, an old process is still serving after an
upgrade: `t-man restart status` and compare again.

## first moves

1. `curl -s -o /dev/null -w '%{http_code}' {{.BaseURL}}/healthz` (204 means serve is up)
2. If unreachable: `t-man list`, then `t-man restart status`
3. `curl -s {{.BaseURL}}/api/status` to see the snapshot the dashboard renders
4. For a suspect service, compare its own `/version` with what the dashboard shows
5. Check that `history.json` in the workdir updated within the last few minutes
