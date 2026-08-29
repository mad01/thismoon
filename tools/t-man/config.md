# t-man configuration

## Where config lives

t-man has no config file. It reads no YAML, TOML, or JSON on startup, and it
keeps no on-disk state of its own: every run gets its desired state from
command-line flags, and the launchd plist it writes doubles as both the
service definition and the record of what was last applied.

- **Per-invocation flags** are the whole configuration surface. `t-man add`
  builds a `service.Definition` in memory from its flags, hands it to the
  reconciler, and forgets it once the command exits. There is nothing to
  edit between runs; the next `add` for the same service supplies the flags
  again (typically from a ralph recipe or an install script), and t-man
  diffs the result against the plist on disk.
- **The generated plist** is the one persisted artifact:
  `~/Library/LaunchAgents/<name>.plist` for agents (the default), or
  `/Library/LaunchDaemons/<name>.plist` for daemons added with `--daemon`.
  It carries a `TManMetadata` dict with a SHA256 hash of the definition, so
  an unchanged `add` is a no-op and a changed one rewrites the plist. The
  file is marked `DO NOT EDIT MANUALLY`: hand edits are not read back into
  anything and would just be overwritten (or silently ignored, if the
  `TManMetadata` marker gets stripped so t-man stops recognizing the plist
  as its own).
- **Precedence**: for any given service, flags passed to the current `add`
  invocation always win. There is no merging with a previous run's flags;
  t-man only compares the *result* (the fully assembled Definition) against
  what the existing plist decodes back into.

## Keys

### Global flags (all commands)

- `--agent` (bool, default `true`): operate on per-user LaunchAgents in
  `~/Library/LaunchAgents`. This is the default mode and needs no elevated
  privileges. The flag exists for serviceman compatibility; `--agent=false`
  is another way of asking for daemon mode.
- `--daemon` (bool, default `false`): operate on system LaunchDaemons in
  `/Library/LaunchDaemons` instead. Every command needs `sudo` in this mode,
  not just `add`.

`--agent` and `--daemon` are one choice spelled two ways, so t-man resolves
them together rather than treating them as unrelated switches: passing
either one alone picks the mode it names, and passing both is fine as long
as they agree (`--agent=false --daemon`). A pair that says the same thing twice
(`--agent --daemon`, or `--agent=false --daemon=false`) is rejected with an
error naming both values.
- `--dryrun` (bool, default `false`): print what `add` would do without
  writing a plist or calling `launchctl`.

### `add` flags

- `--name` (string, required): the service name. Becomes the plist `Label`
  and the plist's file name (`<name>.plist`). Allowed characters: letters,
  digits, `.`, `-`, `_`.
- `--desc` (string, default `""`): a human-readable description. Accepted
  and validated but not currently rendered into the plist or shown by any
  command.
- `--workdir` (string, default `""`): working directory for the service
  process. Must be an absolute path to an existing directory when set.
- `--path` (string, default `""`): colon-separated PATH entries to prepend
  to the service's `PATH` environment variable. Combines with `--env
  PATH=...` if both are given, with `--path` taking the prefix position.
- `--env` (repeatable `KEY=VALUE`, default none): environment variables set
  on the service process.
- `--logs` (string, default: computed): directory for `stdout.log` and
  `stderr.log`. When omitted, defaults to `~/Library/Logs/<name>` in agent
  mode or `/var/log/<name>` in daemon mode. The directory is created if it
  does not exist.
- `--sandbox-profile` (string, default `""`): path to a seatbelt profile
  (`.sb` file). When set, the service's `ProgramArguments` wrap the command
  in `/usr/bin/sandbox-exec -D HOME=<home> -f <profile>`. Both the resolved
  path and a SHA256 digest of the profile's content feed the reconcile hash,
  so editing the profile file (without changing any flag) still triggers a
  plist rewrite and service bounce on the next `add`.
- `--extra-log` (repeatable `NAME=PATH`, default none): registers an
  additional named log file for `t-man logs --source NAME`, for output a
  service writes outside stdout/stderr (for example, a sandbox denial
  ledger). The name must match the same character set as service names and
  cannot be `stdout` or `stderr`.

`RunAtLoad` and `KeepAlive` are not configurable; every service t-man
manages gets both set to `true`.

### `list` flags

- `--resources` (bool, default `false`): add PID, RSS, CPU%, and uptime
  columns, sourced from one `launchctl list` call plus one `ps` call.

### `top` flags

- `--interval` (duration, default `2s`): refresh interval for the live
  resource view.

### `logs` flags

- `--follow` / `-f` (bool, default `false`): keep reading as the log file
  grows, like `tail -f`; handles truncation and rotation.
- `--lines` / `-n` (int, default `50`): number of lines to show from the end
  of each source.
- `--stdout` (bool, default `false`): show only stdout.
- `--stderr` (bool, default `false`): show only stderr.
- `--source` (string, default `""`): show a single named source: `stdout`,
  `stderr`, or an extra log name registered with `add --extra-log`.

`--stdout`, `--stderr`, and `--source` are mutually exclusive.

### `version` flags

- `--output` / `-o` (string, default `"text"`): `text` prints the bare
  version token; `json` prints the shared four-key build metadata object
  (`version`, `commit`, `tag`, `build_time`).

### Generated plist fields (not user-edited)

These are the fields t-man writes into every managed plist and reads back
on the next `add` to decide whether anything changed. They are output, not
input: nothing in this section is meant to be hand-authored.

- `Label` (string): the service name.
- `ProgramArguments` (list of strings): the sandbox-exec wrapper (if any)
  followed by the command and its arguments.
- `WorkingDirectory` (string, omitted if empty): from `--workdir`.
- `EnvironmentVariables` (map, omitted if empty): from `--env` and
  `--path`.
- `RunAtLoad` (bool): always `true`.
- `KeepAlive` (bool): always `true`.
- `StandardOutPath` / `StandardErrorPath` (string): the resolved log paths.
- `TManMetadata.Hash` (string): SHA256 of the JSON-marshalled Definition;
  the idempotency check compares this against a freshly computed hash.
- `TManMetadata.ManagedBy` (string): always `t-man`; how `list` and `status`
  recognize a plist as theirs.
- `TManMetadata.Version` (string): the t-man build that last wrote the
  plist.
- `TManMetadata.SandboxProfile` / `SandboxProfileSHA256` (string, omitted if
  empty): round-tripped so reconcile can detect profile content edits.
- `TManMetadata.ExtraLogs` (map, omitted if empty): round-tripped so
  reconcile can detect changes to `--extra-log` registrations.

## Environment variables

- `EVENTS_BASE_URL` (string, default `http://127.0.0.1:7430`): base URL of
  the local events service that `add` posts reconcile events to
  (`POST /api/events`). Emission is fire-and-forget and best-effort: a
  missing or unreachable events service is silently ignored, and no events
  are emitted at all during `go test`.

## Example

There is no config file to show, so the closest equivalent is a full `add`
invocation exercising most of the flag surface, and the plist it produces.

```bash
t-man add \
  --name speak-tts \
  --desc "local text-to-speech server" \
  --workdir "$HOME/.local/share/speak" \
  --env VIRTUAL_ENV="$HOME/.local/share/speak/venv" \
  --env PORT=8765 \
  --path /opt/homebrew/bin \
  --logs "$HOME/Library/Logs/speak-tts" \
  --sandbox-profile "$HOME/.config/t-man/profiles/speak-tts.sb" \
  --extra-log sandbox="$HOME/.local/share/speak/logs/sandbox-notifications.log" \
  -- "$HOME/.local/share/speak/venv/bin/python" -m mlx_audio.server --port 8765
```

This writes `~/Library/LaunchAgents/speak-tts.plist`, roughly:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!-- Managed by t-man - DO NOT EDIT MANUALLY -->
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>speak-tts</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/bin/sandbox-exec</string>
        <string>-D</string>
        <string>HOME=/Users/you</string>
        <string>-f</string>
        <string>/Users/you/.config/t-man/profiles/speak-tts.sb</string>
        <string>/Users/you/.local/share/speak/venv/bin/python</string>
        <string>-m</string>
        <string>mlx_audio.server</string>
        <string>--port</string>
        <string>8765</string>
    </array>
    <key>WorkingDirectory</key>
    <string>/Users/you/.local/share/speak</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/opt/homebrew/bin</string>
        <key>PORT</key>
        <string>8765</string>
        <key>VIRTUAL_ENV</key>
        <string>/Users/you/.local/share/speak/venv</string>
    </dict>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/Users/you/Library/Logs/speak-tts/stdout.log</string>
    <key>StandardErrorPath</key>
    <string>/Users/you/Library/Logs/speak-tts/stderr.log</string>
    <key>TManMetadata</key>
    <dict>
        <key>Hash</key>
        <string>a3f5b8c9...</string>
        <key>ManagedBy</key>
        <string>t-man</string>
        <key>Version</key>
        <string>t-man/v1.4.0</string>
        <key>SandboxProfile</key>
        <string>/Users/you/.config/t-man/profiles/speak-tts.sb</string>
        <key>SandboxProfileSHA256</key>
        <string>7c9e2b4a...</string>
        <key>ExtraLogs</key>
        <dict>
            <key>sandbox</key>
            <string>/Users/you/.local/share/speak/logs/sandbox-notifications.log</string>
        </dict>
    </dict>
</dict>
</plist>
```

Running the same `add` again with unchanged flags and an unchanged profile
file prints `already up to date` and touches nothing on disk.
