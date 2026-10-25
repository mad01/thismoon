# moondb configuration

## Where config lives

moondb resolves each setting from a flag, then an environment variable, then
the config file, then a compiled-in default. The flags are persistent, so
every subcommand takes them. `serve` binds the port, `record` writes under
the workdir, and every other command uses the port to reach serve.

The config file is TOML and optional:

1. `--config <path>`
2. `$MOONDB_CONFIG`
3. `$XDG_CONFIG_HOME/moondb/config.toml`, when that variable holds an
   absolute path
4. `~/.config/moondb/config.toml` (default)

A leading `~` is expanded before the file is opened. A home directory that
cannot be resolved is an error, never a path relative to the working
directory. That matters most for `record`, which runs inside a hook with
whatever environment Claude Code gives it.

A missing file is not an error: the defaults apply. A file that is present
but malformed fails closed. `serve` refuses to start and names the key.
`record` captures nothing, writes one line to stderr (never stdout), and
still exits 0, so the session carries on. `moondb doctor` reports the parse
error. Nothing captures with settings nobody could read, because a broken
`capture.exclude` list could otherwise store a file the user meant to keep
out.

## Flags

- `--port` (int, default `7433`, env `MOONDB_PORT`, config `port`): the port
  `moondb serve` listens on, and the port every other command connects to.
  `serve` binds `127.0.0.1` only: the store holds prompts and source code and
  the API has no authentication.
- `--workdir` (string, default `~/.local/state/moondb`, env
  `MOONDB_WORKDIR`, config `workdir`): the state directory. It holds `log/`,
  `objects/`, `spool/` and `sessions/`. `record`, `import` and `serve` must
  resolve the same workdir, or the spool they write is not the one serve
  drains.
- `--base-url` (string, default `http://localhost:<port>`, env
  `MOONDB_BASE_URL`, config `base_url`): the human-facing URL the MCP server
  returns in tool responses, for example `http://moondb.this`. It is
  display-only: the MCP client always calls `localhost:<port>`.
- `--config` (string, default `~/.config/moondb/config.toml`, env
  `MOONDB_CONFIG`): the config file described above.

An unparseable `MOONDB_PORT` warns once on stderr and falls back to the next
source.

## Config file keys

- `capture.enabled` (bool, default `true`): set it to `false` to turn capture
  off on this machine while leaving the hooks wired. `record` then exits 0
  without writing anything.
- `capture.exclude` (list of globs): paths whose contents are never stored,
  by `record` or by `import`. An excluded file's edit is recorded by path
  only, with no content, patch or observed bytes. A glob matches the file's
  base name or its path relative to the worktree. Setting the key replaces
  the default list, so start from the defaults and add to them:

  ```
  .env  .env.*  *.pem  *.key  id_*  *.p12  *.tfvars  *.tfstate
  credentials*.json  kubeconfig  .npmrc
  ```

## Fixed limits

These are compiled into the binary (`facts.go`) and not configurable:

| Limit | Value | Effect |
|---|---|---|
| record budget | 200 ms | `record` spools what it has gathered when the budget runs out |
| snapshot budget | 2 s | the turn-edge snapshots of one hook; a snapshot cut short is marked truncated and counts the rest as timeout |
| snapshot file cap | 1000 | paths one snapshot lists, the session's touched paths first; the rest count as over the file cap |
| status cap | 4 MiB | `git status` output a snapshot reads; a worktree with more gets no snapshot |
| largest chunk | 1 MiB | a larger file is recorded by path and patch, without content; a snapshot lists it by size |
| transcript tail | 4 MiB | the most transcript a turn keeps |
| log segment | 64 MiB | the size at which `log/` rolls to a new file |

## Hook wiring

The hooks live in the Claude Code settings, which the consuming repo owns.
`record` reads the hook event from the payload, so every line runs the same
command:

```json
{
  "hooks": {
    "UserPromptSubmit": [{ "hooks": [{ "type": "command", "command": "moondb record", "async": true }] }],
    "PostToolUse": [
      { "matcher": "Edit|Write|MultiEdit|NotebookEdit",
        "hooks": [{ "type": "command", "command": "moondb record", "timeout": 3 }] },
      { "matcher": "Bash",
        "hooks": [{ "type": "command", "command": "moondb record", "async": true }] }
    ],
    "Stop": [{ "hooks": [{ "type": "command", "command": "moondb record", "async": true }] }]
  },
  "bashEditDiffEnabled": true
}
```

`bashEditDiffEnabled` is a user or managed setting (Claude Code 2.1.269 or
later). It makes Claude Code add `bashEditDiff` to the Bash payload: up to
five file diffs and up to 200 changed paths. Without it, the Bash hook
records nothing. The Bash line is async: a command has already taken its
time, and the session should not wait on `record` after it.

## Example

```toml
port = 7433
workdir = "~/.local/state/moondb"
base_url = "http://moondb.this"

[capture]
enabled = true
exclude = [
  ".env", ".env.*", "*.pem", "*.key", "id_*", "*.p12",
  "*.tfvars", "*.tfstate", "credentials*.json", "kubeconfig", ".npmrc",
  "secrets/*",
]
```

To see what a given environment resolved to, run `moondb doctor`; an agent
with no shell gets the same report from the `moondb_doctor` MCP tool.
