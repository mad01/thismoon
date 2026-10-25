# `speak` recipe

Builds and installs the `speak` binary, registers the `speak-web` agent behind
`http://speak.this/`, links the `srt-run` sandbox launcher, and registers the
`sandbox-watch` denial notifier. Source lives in this repo: `services/speak/` (see
`services/speak/CLAUDE.md`).

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`), and
ralph merges the recipe with the identity `thismoon/speak`. The package
`working_dir` therefore names the source cache
(`~/.config/ralph/sources/thismoon/services/speak`), not a dev checkout.

## What this recipe does

- **`packages.speak`** — `make build` + `make install` → `~/code/bin/speak`
  (codesigned). Build metadata (version, commit, tag, build time) is injected
  via ldflags from the repo-root `buildinfo.mk`.
- **`packages.speak.service`** — restarts `speak-web` only when the installed
  binary's content changed (ralph hashes `install_paths`). The `t-man status`
  guard skips the restart on first install (before registration).
- **`hooks.builds.speak_web_service`** — registration only: `t-man add …
  speak-env.sh serve --port 7425`. The document audio routes present pages
  register with and play from, the CORS-guarded `/v1/audio/speech`, and a
  static landing page, all through the provider `~/.config/speak/config.yaml`
  selects. On the default local provider, a down `speak-tts` engine agent
  leaves the landing page serving but the speech endpoints failing with that
  reason.
- **`speak-env.sh`** — the spawn wrapper for speak-web and for a consuming
  repo's speak MCP registration. It runs `speak config env` (the names of the
  variables the active provider reads; nothing for the local engine), pulls
  exactly those from `~/.config/ralph/secrets.sh` (legacy `~/.secrets.sh`) in
  a subshell, and execs `~/code/bin/speak` with its arguments. This is the same
  one-variable-at-a-time pattern as present's `present-shared-env.sh`. Select
  the provider with the config file or `SPEAK_PROVIDER`: a `--provider` flag
  after the subcommand is invisible to the wrapper's query.
- **`dotfiles.srt_run`**: symlinks `srt-run.sh` to `~/code/bin/srt-run`, the
  launcher every sandboxed MCP server and t-man service execs through:
  `srt-run <settings-file> <argv...>`. It runs the command under srt
  (anthropics/sandbox-runtime, a mise tool the consuming repo declares) as
  `env SRT_RUN=<label> <argv...>`, where the label is the settings file's
  name (`~/.config/srt/present.json` gives `present`). It also merges
  `SRT_RUN_ALLOW_DOMAINS` into the allow-list at launch and sets the child's
  `TMPDIR`. The settings files themselves come from the consuming repo.
- **`hooks.builds.sandbox_watch_service`** — registers `sandbox-watch`, which
  execs `sandbox-watch.sh` straight from this directory in the sources cache.
  Turns sandbox denials into Notification Center banners (batched per 60s)
  and appends them to `~/.local/share/speak/logs/sandbox-denials.log`. Each
  banner is mirrored to `sandbox-notifications.log` (same dir) with the
  batch's unique denials — banners can't be copied, so read that file to see
  what a notification was about. Both files are registered as t-man extra
  logs (`sandbox`, `sandbox-notifications`): `t-man logs sandbox` shows them
  fleet-wide, `t-man logs sandbox speak-tts` scopes to one service.
- **`hooks.builds.speak_chmod_scripts`** — keeps `sandbox-watch.sh`,
  `sandbox-audit.sh`, `speak-env.sh` and `srt-run.sh` executable inside the
  sources cache.
- **`pre_uninstall`** — removes both agents before cleanup deletes the binary.

## What stays in the consuming repo (private overlay)

Machine-specific wiring is deliberately not here (see `docs/adr/0006`):

- the `[[recipe_sources]]` stanza itself (each machine picks its pin)
- **the `speak-tts` engine recipe** — the mlx-audio Kokoro venv, its sandboxed
  install script, the `speak-tts` agent, and its sandbox settings. The engine
  is a runtime dependency, not part of this repo's releases.
- any read-aloud MCP registration (host-gated where the consumer registers it)
- the `speak.this` route (`speak` → 7425 in the consumer's d-man routes overlay)
- the provider config (`~/.config/speak/config.yaml`) for machines that use a
  remote provider, and the key it names in the secrets file

## sandbox-watch

`sandbox-watch` is the fleet-wide notifier for **every** process launched
through `srt-run`, which is why it lives with speak-web rather than with the
engine. It is wanted on every machine that runs a sandboxed process.

It attributes a denial by its tag, not by process name. srt appends
`CMD64_<base64 of the command's first 100 chars>_END_<session>_SBX` to every
denial it raises, and the command `srt-run` hands srt starts with
`env SRT_RUN=<label>`. A legacy seatbelt profile under t-man's
`--sandbox-profile` (speak-tts, which needs `iokit-open` for Metal and so
can't move to srt) tags each deny rule with `(with message
"SRT_RUN=<label>")`, which the log appends as a plain marker. The watcher
routes each line:

| Line | Route |
|------|-------|
| srt tag decodes to a command holding `SRT_RUN=<label>` | fleet denial: `ALERT`, or `IGNORED` (`ignore-list`) when a pattern matches |
| no srt tag, message carries `SRT_RUN=<label>` (the last one wins) | legacy fleet denial, routed the same way |
| srt tag without the marker | an agent's own bash sandbox (Claude Code and pi use srt too): `IGNORED` (`foreign`), never notifies |
| neither | a system daemon's own profile: dropped |

So there is no process list to maintain: a new sandboxed process is covered
the moment it launches through `srt-run`, or once its legacy profile tags
its denies. Each banner is also posted to
events.this (source `sandbox-watch`, tags `command` = the label, `process`,
`op`, `unique`, `status`).

### Triaging a denial

When `sandbox-watch` notifies (or you read `sandbox-denials.log`), the line is
`label · process(pid) · operation · target`; ledger lines are tab-separated
`time  status  summary  [reason]`. Decide between three actions:

1. **Real breakage**: the denied access is something the process
   legitimately needs and it is failing. Add the narrowest allow to its srt
   settings file (`~/.config/srt/<label>.json`, kept in the consuming repo),
   restart the process, confirm it works.
2. **Expected/benign**: an app probing a credential/config path it never uses
   (dotenv loaders, AWS/GCP SDK credential chains, `~/.netrc` lookups all scan
   every candidate as discovery). The block is correct; the notification is
   noise. Add an ERE pattern to the ignore list. Fleet-wide defaults live in
   `sandbox-ignore.conf` beside the script (edit via a commit to this repo).
   Machine-specific silences go in `~/.config/sandbox-watch/ignore.conf`
   (never edit the sources-cache copy in place, local edits dirty the clone
   and die on the next sync). Patterns match the denial message with the srt
   tag stripped and the summary line, so `^present · ` scopes one to a
   label. The next denial logs matches `IGNORED` and stops notifying. Keep
   the list tight and comment every entry with *why it's benign*; a silenced
   credential read you didn't expect is exactly what an attacker wants
   ignored.
3. **Real issue**: an unexpected read of a secret (`~/.ssh`, keychains) or a
   network attempt from a process that should be offline. Do NOT add an allow
   or silence it. Investigate: check the pin bump / dependency change that
   introduced it, diff the venv, treat as a potential supply-chain compromise.

Rule of thumb: **never silence or allow a denial you can't explain.** The safe
default is to leave it alerting until you understand it.

To reproduce a denial, run the process under `srt-run` from a shell with
`SRT_DEBUG=1` and `tail -f` the ledger, or run `sandbox-audit.sh stream`,
which prints each srt-tagged denial with its decoded label.

**Notification banner setup:** notifications from the launchd agent are
attributed to "Script Editor" — allow once in System Settings → Notifications
→ Script Editor → Allow Notifications, or banners silently never appear.

## Working with it

```bash
ralph up                    # sync the source, build + install, register agents
t-man status speak-web      # the web front-end
t-man status sandbox-watch  # the denial notifier
t-man logs sandbox          # fleet-wide denial logs (extra-log convention)
curl http://speak.this/     # via the d-man route
```

## See also

- Source + module notes: `services/speak/CLAUDE.md`
- Triage helper: `sandbox-audit.sh` (this dir)
- The srt pattern, a worked settings file, and the full triage guide:
  `docs/SANDBOXING.md`
