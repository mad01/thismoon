# `speak` recipe

Builds and installs the `speak` binary, registers the `speak-web` agent behind
`http://speak.this/`, and registers the `sandbox-watch` seatbelt-denial
notifier. Source lives in this repo: `services/speak/` (see
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
  speak-env.sh serve --port 7425`. The upload-a-markdown page + CORS-guarded
  `/v1/audio/speech` through the provider `~/.config/speak/config.yaml`
  selects. On the default local provider, a down `speak-tts` engine agent
  leaves the page serving but the speech endpoints failing with that reason.
- **`speak-env.sh`** — the spawn wrapper for speak-web and for a consuming
  repo's speak MCP registration. It runs `speak config env` (the names of the
  variables the active provider reads; nothing for the local engine), pulls
  exactly those from `~/.config/ralph/secrets.sh` (legacy `~/.secrets.sh`) in
  a subshell, and execs `~/code/bin/speak` with its arguments, the same
  one-variable-at-a-time pattern as present's `present-shared-env.sh`. Select
  the provider with the config file or `SPEAK_PROVIDER`: a `--provider` flag
  after the subcommand is invisible to the wrapper's query.
- **`hooks.builds.sandbox_watch_service`** — registers `sandbox-watch`, which
  execs `sandbox-watch.sh` straight from this directory in the sources cache.
  Turns seatbelt denials into Notification Center banners (batched per 60s)
  and appends them to `~/.local/share/speak/logs/sandbox-denials.log`. Each
  banner is mirrored to `sandbox-notifications.log` (same dir) with the
  batch's unique denials — banners can't be copied, so read that file to see
  what a notification was about. Both files are registered as t-man extra
  logs (`sandbox`, `sandbox-notifications`): `t-man logs sandbox` shows them
  fleet-wide, `t-man logs sandbox speak-tts` scopes to one service.
- **`hooks.builds.speak_chmod_scripts`** — keeps `sandbox-watch.sh`,
  `sandbox-audit.sh` and `speak-env.sh` executable inside the sources cache.
- **`pre_uninstall`** — removes both agents before cleanup deletes the binary.

## What stays in the consuming repo (private overlay)

Machine-specific wiring is deliberately not here (see `docs/adr/0006`):

- the `[[recipe_sources]]` stanza itself (each machine picks its pin)
- **the `speak-tts` engine recipe** — the mlx-audio Kokoro venv, its sandboxed
  install script, the `speak-tts` agent, and its seatbelt profiles. The engine
  is a runtime dependency, not part of this repo's releases.
- any read-aloud MCP registration (host-gated where the consumer registers it)
- the `speak.this` route (`speak` → 7425 in the consumer's d-man routes overlay)
- the provider config (`~/.config/speak/config.yaml`) for machines that use a
  remote provider, and the key it names in the secrets file

## sandbox-watch

`sandbox-watch` is the fleet-wide notifier for **every** sandboxed t-man
service, which is why it lives with speak-web rather than with the engine: it
is wanted on every machine that runs a sandboxed service. One watcher covers
all of them — when another t-man service gets a `--sandbox-profile`, append
its process name to `SANDBOX_WATCH_PROCESSES` in this recipe and give the
service an `--extra-log sandbox=...` pointing at the denials log.

The watch list is deliberately a plain hardcoded default: extra names on a
machine that never runs those services are harmless (they simply never match).
Each entry is matched as a **prefix of the denial's process name** — `python`
catches `python3.14`, and nothing matches on the denial's target path (an
earlier version substring-matched the whole message, so unrelated system
daemons leaked in whenever a watched name appeared inside a path or property
name).

### Triaging a denial

When `sandbox-watch` notifies (or you read `sandbox-denials.log`), the line is
`process(pid) · operation · target`. Decide between three actions:

1. **Real breakage** — the denied access is something the service legitimately
   needs and it is failing. Add the minimal allow to that service's seatbelt
   profile (wherever the consuming repo keeps it), reload, confirm it works.
2. **Expected/benign** — an app probing a credential/config path it never uses
   (dotenv loaders, AWS/GCP SDK credential chains, `~/.netrc` lookups all scan
   every candidate as discovery). The block is correct; the notification is
   noise. Add an ERE pattern to the ignore list — fleet-wide defaults live in
   `sandbox-ignore.conf` beside the script (edit via a commit to this repo);
   machine-specific silences go in `~/.config/sandbox-watch/ignore.conf`
   (never edit the sources-cache copy in place — local edits dirty the clone
   and die on the next sync). The next poll logs matches `IGNORED` and stops
   notifying. Keep the list tight and comment every entry
   with *why it's benign*; a silenced credential read you didn't expect is
   exactly what an attacker wants ignored.
3. **Real issue** — an unexpected read of a secret (`~/.ssh`, keychains) or a
   network attempt from a service that should be offline. Do NOT add an allow
   or silence it. Investigate: check the pin bump / dependency change that
   introduced it, diff the venv, treat as a potential supply-chain compromise.

Rule of thumb: **never silence or allow a denial you can't explain.** The safe
default is to leave it alerting until you understand it. `(with no-report)` in
a profile is the heavier alternative to `sandbox-ignore.conf` — it stops the
denial reaching the log at all, so prefer the ignore-list (keeps an `IGNORED`
audit line) unless a denial is so frequent it floods the log.

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
