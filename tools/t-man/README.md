# t-man

t-man (task-manager) is an idempotent service manager for macOS that
provides declarative, hash-based service management through launchd, with
first-class seatbelt sandboxing for the services it runs.

## How it works

t-man improves upon existing service managers like [serviceman](https://github.com/therootcompany/serviceman)
by providing:

- True idempotency: hash-based change detection updates a service only when its configuration actually changes
- Seatbelt sandboxing: `--sandbox-profile` launches a service through `sandbox-exec` with a custom profile. No other launchd manager (serviceman, `brew services`) can confine what it runs
- Read-compare-apply pattern: consistent reconciliation logic that reads current state, compares with desired state, and only applies necessary changes
- Type safety: written in Go with proper error handling and validation
- Drop-in compatibility: compatible with serviceman CLI for easy migration
- Transparent management: auto-detects managed services with embedded metadata
- Full test coverage: unit and integration tests

Feature summary:

- Declarative service definitions with automatic reconciliation
- SHA256 hash-based change detection
- Per-service seatbelt sandboxing via `--sandbox-profile` (profile content feeds the change-detection hash)
- Support for both user agents (`~/Library/LaunchAgents`) and system daemons (`/Library/LaunchDaemons`)
- Environment variable management
- Custom working directories and log paths
- Service control: start, stop, restart, status
- Scheduled jobs: `--schedule`, `--calendar`, and `--every` map to launchd's `StartCalendarInterval` and `StartInterval`; `list` and `status` show next run, last run, and last exit code; `run` fires a job once on demand
- Resource monitoring: `list --resources` columns and a live `top` view (PID, RSS, CPU%, uptime)
- Log viewing with tail support
- Dry-run mode for safe testing
- serviceman CLI compatibility for migration

### Hash-based change detection

t-man uses SHA256 hashing to detect configuration changes:

1. When you add a service, t-man calculates a hash of the complete service definition
2. t-man stores this hash in the launchd plist file as metadata
3. When you run `add` again, t-man:
   - Reads the current service definition and hash
   - Calculates hash of the desired configuration
   - Compares hashes
   - Only updates if hashes differ

This ensures true idempotency: no unnecessary service restarts.

### Service metadata

Each t-man managed service includes metadata in its plist file:

```xml
<key>TManMetadata</key>
<dict>
    <key>Hash</key>
    <string>a3f5b8c...</string>
    <key>ManagedBy</key>
    <string>t-man</string>
    <key>Version</key>
    <string>1.0.0</string>
</dict>
```

This allows t-man to:
- Auto-detect which services it manages
- Track configuration changes
- Version service definitions

### Log management

t-man creates logs automatically in:
- User mode: `~/Library/Logs/<service-name>/`
- System mode: `/var/log/<service-name>/`

Each service has two log files:
- `stdout.log` - Standard output
- `stderr.log` - Standard error

## Install

### Requirements

- macOS 10.15 or later
- Go 1.21+ (for building from source)

### Build from source

```bash
# Clone the monorepo
git clone https://github.com/mad01/thismoon.git
cd thismoon/tools/t-man

# Build the binary
make build

# Install to ~/code/bin (optional)
make install
```

`make build` writes the binary to this directory as `./t-man`. `make install`
copies it to `~/code/bin` and re-signs it on macOS, which recent macOS
releases require for a copied Go binary to launch.

### Manual build

```bash
go build -o t-man ./cmd/t-man
sudo mv t-man /usr/local/bin/
```

## Usage

Add a simple service that runs at startup:

```bash
# Add a service
t-man add --name myapp -- /usr/local/bin/myapp --port 8080

# List all services
t-man list

# Check service status
t-man status myapp

# View logs
t-man logs myapp

# Remove service
t-man remove myapp
```

### Global flags

- `--agent`: Run as user agent (LaunchAgent) - default
- `--daemon`: Run as system daemon (LaunchDaemon) - requires sudo
- `--dryrun`: Show what would be done without applying changes

`--agent` and `--daemon` are the same choice spelled two ways, so
`--agent=false` also selects daemon mode. Passing both is allowed when they
agree; a pair that says the same thing twice is an error.

### Add or update a service

The `add` command creates or updates a service. It's idempotent - running it twice with the same configuration shows "No changes needed".

Basic syntax:

```bash
t-man add --name <service-name> [flags] -- <command> [args...]
```

The command path may start with `~/`, which t-man expands to your home directory even when the shell didn't (a quoted or config-supplied path).

Flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--name` | _(required)_ | Service name |
| `--desc` | | Service description |
| `--workdir` | | Working directory |
| `--env KEY=VALUE` | | Environment variables (repeatable) |
| `--path` | | Additional PATH entries (colon-separated) |
| `--logs` | `~/Library/Logs/<name>/` (agent) or `/var/log/<name>/` (daemon) | Log directory |
| `--sandbox-profile` | | Seatbelt profile (`.sb`); wraps the service in `sandbox-exec -f <profile>` |
| `--extra-log NAME=PATH` | | Additional named log file (repeatable); view with `logs --source NAME` |
| `--schedule TIMES` | | Run as a scheduled job at `HH:MM` daily; comma list, optional day prefix (`07:30,fri@17:00`, `mon-fri@08:50`, `weekdays@`, `weekend@`) |
| `--calendar FIELDS` | | Run as a scheduled job on launchd calendar fields (`day=1,hour=6,minute=0`; a field may be a range, `hour=9-17`; repeatable) |
| `--every DURATION` | | Run as a scheduled job every interval (`1h`, `30m`) |

Examples:

```bash
# Simple service
t-man add --name myapp -- /usr/local/bin/myapp

# With arguments
t-man add --name webapp -- /usr/local/bin/node /var/www/app.js

# With description and working directory
t-man add --name myservice \
  --desc "My background service" \
  --workdir /var/myapp \
  -- /usr/local/bin/myapp

# With environment variables
t-man add --name myapp \
  --env PORT=8080 \
  --env NODE_ENV=production \
  -- /usr/local/bin/node server.js

# With custom PATH
t-man add --name myapp \
  --path /usr/local/bin:/opt/homebrew/bin \
  -- node server.js

# With custom log directory
t-man add --name myapp \
  --logs /var/log/myapp \
  -- /usr/local/bin/myapp

# System daemon (requires sudo)
sudo t-man add --daemon --name myservice -- /usr/sbin/myservice

# Sandboxed service (macOS seatbelt)
t-man add --name speak-tts \
  --env VIRTUAL_ENV=$HOME/.local/share/speak/venv \
  --sandbox-profile ~/path/to/speak-tts.sb \
  -- $HOME/.local/share/speak/venv/bin/python -m mlx_audio.server --port 8765

# Register extra log files written outside stdout/stderr (e.g. by a
# sandbox-denial watcher), so 'logs --source sandbox' can find them
t-man add --name speak-tts \
  --extra-log sandbox=~/.local/share/speak/logs/sandbox-notifications.log \
  -- $HOME/.local/share/speak/venv/bin/python -m mlx_audio.server --port 8765
```

Sandboxed services: with `--sandbox-profile`, the generated plist launches
the command through `/usr/bin/sandbox-exec -D HOME=<home> -f <profile>`.
Profiles can use `(param "HOME")`; t-man always supplies it. `sandbox-exec`
execs the target in place (same PID), so KeepAlive and restart behavior are
unchanged. Both the profile path and its content feed the idempotency hash:
editing the `.sb` file makes the next `t-man add` re-render the plist and
bounce the service; an unchanged re-add stays a no-op.

Idempotency in action:

```bash
# First run - creates service
$ t-man add --name myapp -- /usr/local/bin/myapp
✓ Service 'myapp' created

# Second run - no changes needed
$ t-man add --name myapp -- /usr/local/bin/myapp
✓ Service 'myapp' already up to date

# Update configuration - detects change
$ t-man add --name myapp --env PORT=8080 -- /usr/local/bin/myapp
✓ Service 'myapp' updated
```

### Scheduled jobs

A service added without a schedule is long-lived: launchd starts it at load
and restarts it whenever it exits. Add `--schedule`, `--calendar`, or
`--every` and it becomes a scheduled job instead. launchd starts it at the
given times and lets it exit. Nothing restarts it before the next trigger: a
scheduled job gets `RunAtLoad` and `KeepAlive` set to false. `add` rejects a
schedule combined with `KeepAlive`, which would relaunch the job the moment
it finished.

Three flags describe when it runs. `--schedule` and `--calendar` both add
calendar entries and may be combined; `--every` is the interval form and
excludes them.

| Flag | Form | launchd key |
|------|------|-------------|
| `--schedule` | `HH:MM` daily, a comma list, an optional day prefix: `07:30`, `07:30,fri@17:00`, `mon-fri@08:50`, `weekdays@08:50` | `StartCalendarInterval` |
| `--calendar` | launchd's own fields, comma-separated, repeatable, with ranges: `day=1,hour=6,minute=0`, `hour=9-17,weekday=1-5,minute=0` | `StartCalendarInterval` |
| `--every` | a Go duration in whole seconds: `1h`, `30m`, `90s` | `StartInterval` |

The day prefix of `--schedule` is a weekday name (`mon`, or `monday`), a
range (`mon-fri`), or a set. The sets are `weekdays` (mon-fri), `weekend`
(sat-sun), and `daily` (the same as no prefix). A range may wrap, so
`fri-mon` is Friday through Monday. launchd takes one weekday per calendar
entry, so a set or range expands to one entry per day. A `--calendar` field
may be a range too (`hour=9-17`, `day=1-7`, `weekday=1-5`, and `weekday=5-1`
wraps). Ranged fields expand to their cross product, so
`hour=9-17,weekday=1-5` is 45 entries. One job may carry at most 200
entries, and `add` refuses a larger expansion with the count it would have
produced.

```bash
# One headless pass every morning
t-man add --name work-digest --schedule 07:30 -- $HOME/code/bin/digest --yesterday

# Weekly rollup on Monday mornings
t-man add --name weekly-rollup --schedule mon@08:00 -- $HOME/code/bin/rollup --week

# Office days only, two spellings of the same schedule
t-man add --name standup-notes --schedule weekdays@08:50 -- $HOME/code/bin/standup
t-man add --name standup-notes --schedule mon-fri@08:50 -- $HOME/code/bin/standup

# Weekend mornings, and a range that wraps past Sunday
t-man add --name weekend-sync --schedule weekend@09:00 -- $HOME/code/bin/sync
t-man add --name long-weekend --schedule fri-mon@09:00 -- $HOME/code/bin/sync

# Every hour on the hour during office hours: 45 calendar entries
t-man add --name poll --calendar hour=9-17,weekday=1-5,minute=0 -- $HOME/code/bin/poll

# First of the month at 06:00; day and month are only reachable with --calendar
t-man add --name monthly-audit --calendar day=1,hour=6,minute=0 -- $HOME/code/bin/audit

# Every hour, counted from when launchd loaded the job
t-man add --name hourly-sync --every 1h -- $HOME/code/bin/sync
```

Calendar fields: `minute` 0-59, `hour` 0-23, `day` 1-31, `weekday` 0-7 (0 and
7 are both Sunday), `month` 1-12. A field left out is a wildcard, and every
field given must match. So `day=13,weekday=5` means Friday the 13th, where
cron would read it as the 13th or any Friday. An entry that sets no field is
rejected, since it would fire every minute; use `--every 1m` for that.

After expansion the entries are sorted and deduplicated, with Sunday always
written as 0. The same schedule written two ways therefore produces the
same plist and the same hash: `weekdays@07:30`, `mon-fri@07:30`, five
listed days, and `--calendar weekday=1-5,hour=7,minute=30` all land on one
hash. Re-adding with the other spelling is a no-op. `status` folds the entries back for display. The
five entries from `weekdays@07:30` read as `mon-fri 07:30`, and the 45 from
the office-hours example read as `mon-fri hour=9-17,minute=0`.

Between runs a scheduled job is not running, and that is its healthy state.
`t-man list` reads it as `scheduled` and, as soon as one scheduled job
exists, adds three columns:

```
NAME                           STATUS       NEXT RUN         LAST RUN         EXIT  COMMAND
----------------------------------------------------------------------------------------------------------------------------
present                        running      -                -                   -  /usr/local/bin/present serve --port 7423
work-digest                    scheduled    2026-10-02 07:30 2026-10-01 07:31    0  /Users/you/code/bin/digest --yesterday
```

`NEXT RUN` is computed from the calendar entries. An interval job shows its
cadence (`within 1h00m`) because only launchd knows when its timer started.
`LAST RUN` is the newest write to the job's stdout or stderr log, the closest
thing to a run timestamp launchd offers; a run that prints nothing leaves it
unchanged. `EXIT` is the last exit code launchd reported. `t-man status
<name>` prints the same four facts as lines.

To run a job once outside its schedule, for testing:

```bash
t-man run work-digest
t-man logs work-digest -f
```

`run` refuses a long-lived service and points to `start`. `start`, `stop`,
`restart`, and `remove` work on scheduled jobs the same way they do on
services; `restart` re-registers the job, which resets an interval timer.

Two launchd behaviors to design a job around. If the machine is asleep when
a calendar slot passes, launchd runs the job once after wake. Several missed
slots collapse into that single run, so a job has to tolerate running late
and can't count on one run per slot. And launchd counts an interval from
load rather than from the previous run, so `--every 1h` starts a fresh hour
at every login. Keep scheduled jobs idempotent.

### List services

List all managed services:

```bash
t-man list
# or
t-man ls
```

Output:
```
NAME                           STATUS          COMMAND
-----------------------------------------------------------------------------------------------
myapp                          running         /usr/local/bin/myapp
webapp                         stopped         /usr/local/bin/node /var/www/app.js
```

Add `--resources` for PID, RSS, CPU%, and uptime columns:

```bash
t-man list --resources
```

Output:
```
NAME                           STATUS          PID      RSS   CPU%   UPTIME  COMMAND
----------------------------------------------------------------------------------------------------------------------
myapp                          running       53009    20.3M    0.0    7m17s  /usr/local/bin/myapp
webapp                         stopped           -        -      -        -  /usr/local/bin/node /var/www/app.js
```

PIDs come from one `launchctl list` call and the stats from one `ps` call, so
the flag adds no per-service overhead.

### Monitor resources live

`top` is a live-updating resource view scoped to t-man-managed services,
sorted by RSS descending. It refreshes every 2 seconds by default; press
Ctrl-C to quit.

```bash
t-man top
# custom refresh interval
t-man top --interval 5s
```

Output (refreshed in place):
```
t-man top - 2/2 running - total RSS 238.1M - refresh 2s - 18:27:33  (Ctrl-C to quit)

NAME                              PID      RSS   CPU%   UPTIME
--------------------------------------------------------------
webapp                          53355   218.8M    0.0    7m23s
myapp                           53009    20.3M    0.0    7m28s
```

### Remove a service

```bash
t-man remove myapp
# or
t-man rm myapp
# or
t-man delete myapp
```

### Control services

Start, stop, or restart services:

```bash
# Start a service
t-man start myapp

# Stop a service
t-man stop myapp

# Restart a service
t-man restart myapp

# Fire a scheduled job once, now (refused for a long-lived service)
t-man run work-digest
```

### Check service status

Get detailed information about a service:

```bash
t-man status myapp
```

Output:
```
Service: myapp
Status: running
Command: /usr/local/bin/myapp --port 8080
Working Directory: /var/myapp
Environment:
  PORT=8080
  NODE_ENV=production
Stdout: /Users/you/Library/Logs/myapp/stdout.log
Stderr: /Users/you/Library/Logs/myapp/stderr.log
Run at load: true
Keep alive: true
```

For a scheduled job the schedule facts follow the log paths:

```
Schedule: mon-fri 07:30
Next run: 2026-10-02 07:30
Last run: 2026-10-01 07:31 (newest log write)
Last exit code: 0
Run at load: false
Keep alive: false
```

### View logs

By default `logs` shows the last lines of stdout and stderr combined, with a
tail-style `==> source <==` header marking which file each block came from:

```bash
# Last 50 lines of stdout + stderr
t-man logs myapp

# Last 100 lines
t-man logs myapp -n 100
t-man logs myapp --lines 100

# A single stream
t-man logs myapp --stdout
t-man logs myapp --stderr

# A named extra log registered with 'add --extra-log NAME=PATH'
t-man logs myapp --source sandbox

# Follow new output (like tail -f; handles truncation/rotation)
t-man logs myapp -f
t-man logs myapp --stderr --follow
```

`--stdout`, `--stderr`, and `--source` are mutually exclusive. `--source`
also accepts `stdout` and `stderr`, so scripts can treat every log uniformly.
Extra log paths show up under `Extra logs:` in `t-man status <name>`.

### Sandbox logs

`t-man logs sandbox` collects extra log sources named `sandbox` or
`sandbox-*`, by convention the seatbelt denial ledger and notification
mirror written by a sandbox watcher:

```bash
# All sandbox logs across every managed service (same file registered on
# several services is shown once, headed by its path)
t-man logs sandbox

# One service's sandbox logs
t-man logs sandbox speak-tts

# Follow new denials
t-man logs sandbox -f
```

Register the files at add time: `--extra-log sandbox=/path/to/denials.log`.
Note: the subcommand shadows `t-man logs <svc>` for a service literally
named "sandbox"; use `t-man logs sandbox sandbox` in that case.

### Dry run mode

Test changes without applying them:

```bash
t-man --dryrun add --name myapp -- /usr/local/bin/myapp
```

Output:
```
Dry run mode - would perform the following:
Service: myapp
Command: /usr/local/bin/myapp
Logs: /Users/you/Library/Logs/myapp/stdout.log, /Users/you/Library/Logs/myapp/stderr.log
```

### Migrating from serviceman

t-man provides CLI compatibility with serviceman for easy migration.

#### Drop-in replacement

The `add` command syntax is compatible with serviceman:

```bash
# serviceman syntax
serviceman add --name myapp -- /usr/local/bin/myapp

# t-man syntax (identical)
t-man add --name myapp -- /usr/local/bin/myapp
```

#### Update your dotfiles

Replace `serviceman` with `t-man` in your installation scripts:

Before:
```bash
serviceman add --name myapp \
  --workdir /var/myapp \
  -- /usr/local/bin/myapp
```

After:
```bash
t-man add --name myapp \
  --workdir /var/myapp \
  -- /usr/local/bin/myapp
```

#### Managing existing serviceman services

t-man auto-detects its own services using embedded metadata. To migrate
existing serviceman services:

1. Remove the old serviceman service:
   ```bash
   serviceman remove myapp
   ```

2. Add with t-man:
   ```bash
   t-man add --name myapp -- /usr/local/bin/myapp
   ```

Alternatively, t-man will simply manage alongside serviceman services - they won't conflict as long as service names are different.

### Verification steps

#### Test basic operations

```bash
# 1. Create a test service
t-man add --name test-echo -- /bin/echo "Hello from t-man"

# 2. Verify it was created
t-man list | grep test-echo

# 3. Check status
t-man status test-echo

# 4. View logs
t-man logs test-echo

# 5. Remove service
t-man remove test-echo
```

#### Verify idempotency

```bash
# 1. Add a service
t-man add --name myapp -- /usr/local/bin/myapp
# Output: ✓ Service 'myapp' created

# 2. Run same command again
t-man add --name myapp -- /usr/local/bin/myapp
# Output: ✓ Service 'myapp' already up to date

# 3. Change configuration
t-man add --name myapp --env PORT=8080 -- /usr/local/bin/myapp
# Output: ✓ Service 'myapp' updated

# 4. Run again with same config
t-man add --name myapp --env PORT=8080 -- /usr/local/bin/myapp
# Output: ✓ Service 'myapp' already up to date
```

#### Test with dry run

```bash
# Preview changes without applying
t-man --dryrun add --name test -- /bin/echo "test"

# Should show what would be done without creating the service
t-man list | grep test
# (should not be found)
```

### Troubleshooting

#### Service won't start

1. Check the service exists:
   ```bash
   t-man list
   ```

2. Check service status:
   ```bash
   t-man status myapp
   ```

3. View logs for errors:
   ```bash
   t-man logs myapp --stderr
   ```

4. Verify the command is executable:
   ```bash
   ls -l /path/to/command
   ```

#### Permission denied (system services)

System daemons require sudo:

```bash
sudo t-man add --daemon --name myservice -- /usr/sbin/myservice
```

#### Service not found

If t-man can't find a service you created with serviceman, it's because t-man only manages services with its own metadata. Either:

1. Remove and re-add with t-man, or
2. Continue using serviceman for that service

#### Logs are empty

Services may take a moment to start and generate logs. Also check:

1. The service is running: `t-man status myapp`
2. The log directory exists: `ls ~/Library/Logs/myapp/`
3. The command actually produces output

## Where things live

t-man follows a clean architecture pattern:

```
cmd/t-man/              # CLI entry point
internal/
  cli/                  # Cobra CLI commands
  service/              # Service definition and manager interface
  platform/launchd/     # macOS launchd implementation
  reconcile/            # Reconciliation engine
```

Key components:

- Service definition: type-safe service configuration with validation
- Manager interface: platform-agnostic service management
- Launchd implementation: macOS-specific launchd integration
- Reconciler: read-compare-apply reconciliation logic
- CLI: user-facing command-line interface

Deeper docs:

- [`docs/architecture.md`](docs/architecture.md): the layers, the reconcile
  loop, and how t-man wraps launchd
- [`docs/agents-and-daemons.md`](docs/agents-and-daemons.md): agent vs daemon,
  the one-time daemon setup, and the `--port` health-check convention
- [`docs/troubleshooting.md`](docs/troubleshooting.md): a service that won't
  stay up, and debugging with `launchctl` directly
- [`docs/working-on-t-man.md`](docs/working-on-t-man.md): build, run, test, and
  debug from source

## Develop

### Running tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package tests
go test ./internal/service/
```

### Building

```bash
# Build binary
make build

# Run tests
make test

# Install locally
make install
```

### Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Ensure all tests pass: `go test ./...`
5. Submit a pull request

## License

BSD 3-Clause License - see the LICENSE file at the repository root

## Acknowledgments

- Inspired by [serviceman](https://github.com/therootcompany/serviceman)
- Built with [cobra](https://github.com/spf13/cobra) CLI framework
- Uses [howett.net/plist](https://github.com/DHowett/go-plist) for plist handling

## Docs

- [architecture](architecture.md): internal structure and data flow
- [operating](operating.md): runtime behavior, failure modes, first moves
- [why](why.md): why this component exists
- [config](config.md): configuration reference
