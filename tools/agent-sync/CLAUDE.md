# agent-sync, sync coding agent config into the live files (CLI)

Go CLI that brings the coding agent config a data repo carries onto a
machine. The config file has one section per kind of data. `settings`
merges a template settings.json into Claude Code's settings. `mods`
installs and enables Claude Code mods through `claude plugin`, and `mcp`
registers MCP servers through `claude mcp`. It replaces the dotfiles repo's
three Python sync scripts. Each section keeps its script's stdout, exit
codes and written bytes, apart from the deviations listed below, so a ralph
recipe can swap one command for the other.

## Module layout

```
agent-sync/
  cmd/agent-sync/      entrypoint: os.Exit(cli.Main(...))
  embed.go, facts.go   operating doc embed, agentdoc Facts, shared defaults
  internal/
    apply/             shared: the three modes, exit codes, Streams, Main
    config/            shared: config file location, top-level parse, Section helpers
    pyjson/            shared: JSON parse and print that match Python's json module,
                       bool(), str() and repr() of the decoded values, and the str
                       methods the scripts parsed claude's output with
    pypath/            shared: pathlib normalizing, expanduser, realpath, glob, mkdir,
                       OSError text
    gate/              shared: the machine (host, profile tags) and the hosts/profiles gates
    claudecode/        shared: the claude CLI lookup and Claude Code's config directory
    settings/          the settings section (config, merge, report, sync, doctor)
    mods/              the mods section (config, desired, gate, state, plan, doctor)
    mcp/               the mcp section (config, servers, entry, claude, doctor)
    cli/               cobra command tree
  Makefile             package path github.com/mad01/thismoon/tools/agent-sync (monorepo module, no own go.mod)
```

## How it works

`agent-sync <section> sync|plan|check` loads the config file, hands the
section's YAML to the section package, and runs it in the named mode. The
section package parses its own keys, does the work, prints its report on
stdout and returns an error or `apply.Exit(code)`. `cli.Main` turns that
into the exit code and prints any error once as `error: <message>`.

### Shared conventions

- Modes: `apply.Sync` writes; `apply.Plan` prints what Sync would do and
  writes nothing; `apply.Check` writes nothing, prints a one-line summary
  first (the line `ralph doctor` shows) and returns
  `apply.Exit(apply.CodeDrift)` on drift.
- Exit codes: `CodeOK` 0, `CodeDrift` 1, `CodeError` 1 (any plain
  error), `CodeExternal` 2 (`apply.Fail(apply.CodeExternal, err)` when the
  claude CLI fails, as the scripts did).
- stderr: `error: ` comes from `apply.Main`; warnings go through
  `apply.Warn(out.Err, ...)`. Never print an error and also return it.
- Config: a section reads its YAML with `Section.Fields(allowed...)`,
  then `Section.String`, `Section.Bool`, `Section.Path` (absolute or `~/`,
  expanded), `Section.PathOr` (the same, or a default when the key is
  absent) and `Section.Errorf`, so every error reads `config
  <file>:<line>: <section>.<key>: ...`. Section defaults are constants in
  the section package, or in `gate` when two sections share one;
  `facts.go` holds only the component-wide ones.
- JSON: read with `pyjson.ReadFile` (ordered objects, Python errors),
  compare with `pyjson.Equal` (Python `==`: `true == 1 == 1.0`), print with
  `pyjson.Compact` (`json.dumps(v)`), `pyjson.Indent(v, 2)` (`indent=2`) and
  `pyjson.SortKeys` (`sort_keys=True`). Never `encoding/json` for data a
  Python script used to write: it reorders keys and escapes differently.
- Python values: `pyjson.Truthy` is `bool(v)`, `pyjson.Str` is `str(v)`,
  `pyjson.Repr` is `repr(v)`, and `pyjson.Describe` names a value's type
  in an error message.
- Python strings: `pyjson.Text` decodes subprocess output as `text=True`
  does (universal newlines), and `pyjson.Strip`, `pyjson.Fields` and
  `pyjson.SplitLines` are `str.strip()`, `str.split()` and
  `str.splitlines()`. Never `strings.TrimSpace` or `strings.Fields` where a
  script used those: Python's whitespace set is wider.
- Paths: `pypath.Clean` and `pypath.Join` normalize as pathlib does,
  `pypath.ExpandUser` is `Path.expanduser`, `pypath.RealPath` is Python
  3.13's `os.path.realpath`, `pypath.Glob` is a sorted `Path.glob`,
  `pypath.MkdirParents` is `Path.mkdir(parents=True, exist_ok=True)`, and
  `pypath.ErrorText` is `str()` of an OSError (`[Errno 2] ...: '/path'`).
- Gates: `gate.Current(profilesFile)` reads the lowercased host name and
  the profile tags (`profiles = [...]` in ralph's `config.local.toml`).
  `gate.Values` reads a gate's JSON the way both scripts iterated it. A
  list counts by element, an object by key and a string by character, each
  through `str()`; a falsy value is no gate. `Machine.HostAllowed` and
  `Machine.ProfileAllowed` take the result. Each section words its own
  errors, and mcp adds one rule (a hosts list must hold strings).
- Claude Code: `claudecode.LookPath` finds the claude CLI for mods, mcp and
  both doctors, and `claudecode.ConfigDirEnv` and
  `claudecode.DefaultConfigDir` name Claude Code's config directory. A
  claude found first through a relative PATH entry (`.`, or an empty one)
  counts as missing; the scripts' `shutil.which` returned that relative
  path. No fixture covers it, because the case tests cannot set the
  working directory; `claudecode`'s own test does.
- Doctor: each section exports `DoctorChecks(config.Section)
  []doctor.Check`; `cli/doctor.go` runs the config check, then every
  section's checks in order.
- Fixtures: each section runs `testdata/cases/` through `cli.Main` and
  compares stdout, stderr, the exit code and any claude calls with the
  reference script's. A case that deviates on purpose says why in its
  case.json `deviation` and holds agent-sync's output in `want/*.deviation`
  files, which win over the reference's. Never hand-edit a reference want
  file.

### settings

Ports `sync_settings.py`. The template wins over live: objects merge key by
key, arrays and scalars replace, live-only keys are kept. `prune` key paths
are deleted from live first, and a prune path the template still sets is
an error. The overlay merges last with the four permission lists unioned.
The diff compares the unpruned live file against the merge, so pruned keys
count as drift. `testdata/oracle.py` regenerates `want/` from the script
at dotfiles commit 659e352, and `internal/cli/settings_test.go` runs the
cases.

Deliberate deviations:

- A symlinked live file already in sync is rewritten as a regular file,
  where the script deleted it (`symlink-in-sync`).
- The first sync on a new machine merges the overlay and prints the
  `merged local overlay` line, in plan too. The script wrote the template
  alone, so check reported drift until a second sync (`fresh-install`).
- The live file is written as a temp file beside it, then renamed into
  place. A failed write leaves it as it was, and a reader never sees
  partial JSON. The script truncated it in place, or deleted a link first.
  A regular file keeps its mode, a broken link takes its target's mode,
  and a new file is 0600 because settings can hold env secrets. The
  script gave the last two the umask's mode (`settings/sync_test.go`).
- A dangling symlink at the live path is reported as a symlink and
  replaced by a regular file; its target is never created. The script
  wrote through it and created the target (`symlink-dangling`).
- The live path defaults to `$CLAUDE_CONFIG_DIR/settings.json` when that
  is set, as Claude Code and sync_mods.py do; the script always used
  `~/.claude`. The overlay default follows the live file's directory.
- Two NaNs are equal. The script compared values under an object key with
  `!=`, so a NaN the template sets was drift on every run (`nan-value`).
- `plan` never creates the live file's directory.
- The prune list lives in the config, so its errors name `settings.prune`
  and the config line.

### mods

Ports `sync_mods.py`. The base mods file (`file`), then each `*.json` in
`dropins` in sorted order, names one directory marketplace and its mods.
The sync diffs them against `known_marketplaces.json`,
`installed_plugins.json` and `enabledPlugins`, and runs `claude plugin
<verb> --scope user` to close the gap. It never writes those files.
`CLAUDE_CONFIG_DIR` moves all three, as it does for Claude Code.

- The update rule: for an enabled mod, an install record version that
  differs from the plugin.json version in its marketplace folder runs
  `claude plugin update`, and one run settles it.
- The retire rule: `"installed": false` uninstalls the mod even when the
  marketplace no longer lists it; `"enabled": true` beside it is an error.
- A gate keeps a mod off a machine, and a gated-off mod is left alone.

`testdata/generate.py <dotfiles>/recipes/claude-mods` writes
`testdata/cases/`, and `internal/cli/mods_test.go` runs the cases with
`testdata/fake-claude.sh` first on PATH. No step may write under HOME.

Deliberate deviations:

- A missing mods file says `mods file not found: <path>`; the script said
  `<path>: top level must be an object` (`mods-file-missing`).
- A `hosts` or `profiles` gate that is a nonzero number or `true` is an
  error; the script crashed with a TypeError (`gate-not-a-list`). `0` and
  `false` mean no gate, as in the script.
- A claude found first through a relative PATH entry counts as missing
  (see Claude Code above), so sync fails and plan prints the bare name; the
  script ran that claude.

### mcp

Ports `sync_mcp.py`. The desired set is `servers`, with `dropins/*.json`
merged over it by name. Names starting with `_` are skipped, and so are
entries whose `hosts` or `profiles` gate excludes this machine (profiles
come from `profiles_file`). Missing servers are added at user scope. A
server whose `claude mcp get` spec differs is removed and added again;
remote (http, sse) servers are compared by url alone.

`prune` removes every name `claude mcp list` shows outside the set, but
only through `claude mcp remove --scope user`. A name that is not a
user-scope server fails the run with exit 2. Such names are a server in
another scope, a `claude.ai X` connector, and `plugin`, which a
plugin-provided server's `plugin:x:y` list line parses to. That matches
the script.

A stamp (the sha256 of the desired set plus the host name) lets sync and
check skip the per-server get calls while it matches. Plan always runs
them, and `--force` or `prune` makes sync and check run them too. So with
prune on, the env and space-in-argument bugs below re-add those servers on
every sync, and check exits 1 on every run.

`testdata/reference.py <sync_mcp.py>` regenerates `want/` with
`testdata/fake-claude.sh` as claude, and `internal/cli/mcp_test.go` runs
the cases.

Deliberate deviations:

- Warnings start `warning:`; the script printed `warn:` (`overlays`,
  `stamp-unwritable`).
- Every entry field is type-checked up front. The script split a string
  `args` into characters (`args-string`) and crashed on an entry that is
  not an object (`entry-not-object`).
- The `hosts` and `profiles` gates read as the script read them
  (`gate-types`), except where it crashed. A hosts list item that is not a
  string is an error (`gate-host-not-string`; the script called
  `h.lower()`), and so is a gate that is a nonzero number or `true`
  (`gate-not-a-list`).
- A claude found first through a relative PATH entry counts as missing
  (see Claude Code above); the script ran that claude.
- `check` is a new mode. Its want files are written by hand.

Script bugs kept for parity until the scripts are deleted, seen with claude
2.1.306:

- `--header` goes before the name and url, which claude rejects, so a
  server with static headers and no headersHelper cannot be added.
- `claude mcp get` now prints Environment one variable per line, so a
  server with env is re-added on every full check.
- An argument that contains a space is re-added on every full check, for
  the same reason: Args is split at whitespace.

## Build / install / test

```bash
make build    # ./agent-sync (build metadata via ldflags, from ../../buildinfo.mk)
make install  # build + cp to ~/code/bin/agent-sync + adhoc codesign
make test     # go test ./...
make lint     # golangci-lint run ./...
```

## Commands

```
agent-sync settings sync|plan|check   # merge template + overlay into the live settings
agent-sync mods sync|plan|check       # reconcile mods through claude plugin
agent-sync mcp sync|plan|check        # reconcile MCP servers through claude mcp (--force on sync, check)
agent-sync doctor                     # config valid, data readable, live writable, claude on PATH
agent-sync docs                       # print the embedded operating doc
agent-sync version [-o json]
```

Every command takes `--config` (default `$AGENT_SYNC_CONFIG`, then
`~/.config/agent-sync/config.yaml`).

## Gotchas

- Runtime debugging lives in `operating.md` (embedded, printed by
  `agent-sync docs`). Keep failure modes there, not here.
- Output strings are a contract. ralph shows check's first line, and
  the report lines match the Python scripts byte for byte, em dash
  included. The source spells those characters as `string(rune(...))`
  constants (`settings/report.go`, `mcp/mcp.go`), so no literal em dash
  enters the repo.
- Tests never touch the real `~/.claude`. They set `HOME` and clear
  `CLAUDE_CONFIG_DIR` and `XDG_CONFIG_HOME` per test. Keep it that way.
- Goldens come from Python, and no generator touches a `.deviation`
  file. `internal/pyjson/testdata/golden.py` regenerates `python.json`;
  edit the script, not the JSON. The three case generators differ:
  - settings: `python3 -I internal/settings/testdata/oracle.py <dotfiles
    checkout> [--out DIR] [CASE ...]` reads the script at the pinned
    commit with git show and rewrites each case's `want/`; `--out DIR`
    writes `DIR/<case>/want/` instead, for a parity diff.
  - mods: `python3 -I internal/mods/testdata/generate.py <dotfiles>/recipes/claude-mods`
    imports `sync_mods.py` and `test_sync_mods.py` from that directory as
    checked out. The cases are defined in generate.py, which rewrites all
    of `cases/` (homes, case.json, want) and deletes any case it does not
    define. It takes no case list and no `--out`: copy `testdata/` and run
    the copy for a parity diff.
  - mcp: `python3 -I internal/mcp/testdata/reference.py <path to
    sync_mcp.py> [CASE ...]` runs the given script. Case inputs and
    case.json are hand-written; it rewrites only `want/` and leaves check
    steps' wants alone. No `--out`: copy `testdata/` and run the copy.
- The fake claude scripts run as fresh executables; macOS can take seconds
  to launch one the first time, so the case tests launch them once,
  untimed, before the first case.
- `agent-sync version -o json` returns the shared four-key build metadata
  object; plain `version` stays a bare token.

## See also

- Recipe: `recipes/agent-sync/recipe.toml` (build and install only)
- Running a sync is the consuming repo's job (docs/adr/0006): it ships the
  data and the config, and calls `agent-sync <section> sync` with `check`
  as the verify step.
