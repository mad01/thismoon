# harvest, session knowledge to the brain inbox (CLI)

Go CLI behind two Claude Code hooks. `harvest session-end` is a SessionEnd
hook: it starts a background run over the session that just ended. The run
cuts the transcript down to what the user and the assistant wrote. A model
then reads that digest and names what is worth keeping, and harvest saves it
as raw captures in the brain vault inbox. `harvest hint` is a belt custom
hint: once a day it tells the session how many captures wait for review.

The model gets no tools. A run is three steps, each its own process, and
`harvest run` starts them. The step that acts on model output and writes the
vault has no network, and the provider key reaches only the model step.

## Module layout

```
harvest/
  cmd/harvest/      entrypoint
  embed.go          operating doc embed
  facts.go          component facts and the defaults the docs render
  internal/
    cli/            cobra commands, the hidden step group, doctor checks
    settings/       the HARVEST_* environment, parsed once
    state/          the state dir: log, per-session marks, hint stamp, job dirs, locks
    digest/         transcript JSONL to digest text (pure)
    run/            the orchestrator and the prepare, complete and apply steps
    reply/          the model's reply to checked captures (pure)
    sandbox/        how each step starts: argv and env (pure), and the exec
    hook/           SessionEnd payload, guards, detached start of `harvest run`
    hint/           the once-a-day nudge from `brain list`
  service-info.yaml catalog entry and spec.access, the sandbox manifest
  Makefile          part of module github.com/mad01/thismoon (no own go.mod)
```

## How it works

### session-end

`hook.Handle` reads the payload (`transcript_path`, `session_id`, `cwd`,
`agent_id`). It returns early with no log line when `HARVEST_CHILD` or
`HARVEST_OFF` is set or `agent_id` is present. It logs a skip when the
payload is not JSON, when `state.CheckSession` refuses the session id
(checked before the id reaches any log line), or when it names no
transcript file. Otherwise it starts
`harvest run -- <transcript> <session-id> <cwd>` through `hook.Detached`, which
calls setsid, gives the child the null device for stdin, stdout and stderr,
and does not wait for it. SessionEnd
hooks share about a second and a half, so nothing heavy happens here. The
command always exits 0.

### run

`run.Session` is the orchestrator. It runs outside the sandbox and reads no
session text and no model output. It takes the session's lock
(`state.Lock`, a flock on `<state>/locks/<session>.lock`) and holds it to
the end; when another run holds it, it logs
`skip <session>: a harvest of this session is running` and stops. Then
`state.SweepJobs` removes every job dir whose lock no process holds, which
is what a killed run leaves. It resets the job dir
`<state>/jobs/<session>`, copies the skill file (`HARVEST_SKILL`) to
`system.md` in it, and starts the steps through `run.StepFunc`. The CLI's
StepFunc is `sandbox.Run`; tests run the steps in-process.

1. **prepare** (`harvest step prepare <transcript> <session-id> <cwd>`)
   reads the mark, builds the digest of the lines after it with
   `digest.Build`, and logs a skip when nothing is new or the digest is
   under `HARVEST_MIN_BYTES`. Otherwise it writes `user.md` (the
   `Session id:`, `Working directory:` and `Date:` lines, then the digest in
   a `<digest>` block) and the counts (`lines`, `digest-bytes`).
2. No `user.md` means a skip: the run removes the job dir and stops.
3. **complete** (`harvest step complete <session-id>`) selects the backend
   with `llm.Select` and calls it once under a 10 minute timeout, with
   `system.md` as the system prompt and `user.md` as the user text. It
   writes `reply.txt` unread and `call.json` (backend, model, cost, tokens).
4. **apply** (`harvest step apply <session-id>`) parses the reply with
   `reply.Parse`, writes each capture with `brain capture`, moves the mark to
   `lines`, and logs the `done` line with the report under it.

The run removes the job dir at the end, on success and on failure, and
then releases the lock. A failed step logs `fail <session>: <step>: <err>`
and moves no mark. err is what the step printed to stderr from the last
line that starts with `harvest: ` to the end (an error from kit/llm can
span lines), collapsed to one line and cut to 300 bytes; with no such
line, the last line.

### the sandbox

`sandbox.Build` is pure: the argv and environment of one step. A confined
step runs `<launcher> <self> step <name> -- <args...>` from `/` with a
clean environment: `HOME`, `PATH`, `TMPDIR`, the `HARVEST_*` settings that
are set, and `HARVEST_CHILD=1`. Only complete also gets the set
`llm.ProviderEnv` variables, `HARVEST_BACKEND` forced to the backend the run
selected, and `SRT_RUN_ALLOW_DOMAINS` with the provider's host. A LiteLLM
host with a comma or whitespace in it is refused: the step gets no allowed
host and fails. The run learns the backend by calling `llm.Select` itself,
which makes no call.

The launcher is `recipes/harvest/harvest-step.sh`, linked to
`~/code/bin/harvest-step`: it execs srt-run with harvest's settings file and
the arguments it was handed. It is a script, not Go, because
`recipes/srtspec/srt-settings-test.sh` checks that something in the recipe
launches through srt-run with the shipped settings.

The claude CLI backend cannot log in inside srt: its login is in the macOS
keychain. So complete on that backend runs outside srt, with the parent's
environment. That is safe because kit/llm runs `claude -p` with no tools, no
hooks and no settings.

`HARVEST_SANDBOX=off` runs every step directly, still with the clean
environment. With the sandbox on and the launcher missing, `sandbox.Run`
fails before anything starts. A launcher that finds no srt-run or settings
file fails the step. Neither falls back to running a step unconfined.

### reply

`reply.Parse` decodes one object with `json.Decoder` from the first `{` and
ignores what follows, so a code fence or prose around the object is fine.
It keeps at most five captures, each with a title of at most
`MaxTitleBytes` and a body of at most `MaxBodyBytes`; it drops the others
with a reason, by title. It lowercases the tags, keeps up to three made
only of `a-z`, `0-9`, `/` and `-`, and adds `harvest`. A source must be an
http or https URL. Of the model's own drops it keeps only `Declined`: the
count and the first `MaxReasons` reasons, cut to `MaxReasonBytes`. The
candidate text is never kept, since the model drops secrets. Titles and
reasons are collapsed to one line. apply passes every value to brain as
one `--flag=value` argument through exec, never a shell, and the body on
stdin. A capture brain refuses for a secret is logged without its title.

### digest

`digest.Build` reads lines with no length limit (a tool result can run to
many megabytes). It keeps `user` and `assistant` lines that are not
`isSidechain` or `isMeta`, takes the content string or the `text` blocks of
a content array, drops empty text, and renders `## <type>\n<text>\n\n`. It
keeps the last `HARVEST_MAX_BYTES` bytes, cut on a rune boundary. Only
newline-terminated lines count; an undecodable line counts and is skipped.

### hint

`hint.Line` returns "" when the stamp says it spoke today. Otherwise it runs
`brain list inbox --tag harvest --status raw -o json` and speaks when
`total` is above zero, writing the stamp first. Any failure is silence on
stdout; the cause goes to stderr, which belt does not read.

## Data model & storage

The state dir (`~/.local/state/harvest`, or `HARVEST_STATE_DIR`):

```
harvest.log               one timestamped line per outcome (skip, fail, done)
sessions/<id>.lines       transcript lines the last successful harvest read
hint-day                  the day the hint last spoke, YYYY-MM-DD
jobs/<id>/                one run's job dir, removed when the run ends
locks/<id>.lock           the flock a run of the session holds; the file stays
```

Marks, the stamp and the counts are written with temp-and-rename. Log
entries are one append each. A session id that is empty, `.` or `..`, or
holds a `/` or a control character is refused before it names a file or
reaches a log line. Everything in the state dir is readable by its owner
only: dirs 0700, files 0600. `Ensure` and the log write narrow a wider mode
an older version left.

## Build / install / test

```bash
make build    # ./harvest (build metadata via ldflags, from ../../buildinfo.mk)
make install  # build + cp to ~/code/bin/harvest + codesign
make test     # go test ./...
make lint     # golangci-lint run ./...
```

After a change to `spec.access` in `service-info.yaml`, run
`make sandbox-settings` at the repo root to render
`recipes/harvest/harvest.srt.json`.

## Commands

```
harvest session-end                          # SessionEnd hook: payload JSON on stdin
harvest run <transcript> <session-id> <cwd>  # what the hook starts; run by hand to retry one session
harvest step prepare|complete|apply ...      # hidden: the steps harvest run starts
harvest hint                                 # belt custom hint
harvest doctor                               # checks, then the last log lines
harvest docs                                 # the operating doc
harvest version [-o json]
```

## Gotchas

- **Tests never run a model or touch the vault.** `run` and `cli` tests
  inject a fake `llm.Backend` and run the steps in-process; `run` and `hint`
  tests write a fake `brain` script into a temp dir; the CLI tests swap the
  detached start for an in-process call. `sandbox` tests run shell scripts
  in place of the launcher and the binary.
- **`session-end` must stay fast and exit 0.** Anything slow belongs in
  `harvest run`, behind the detached start.
- **Settings are environment variables only.** A hook gets no flags. The
  names and defaults are in `config.md`. A step reads them again from the
  environment the run gives it.
- **The sandbox workdir is the default state dir.** A machine that sets
  `HARVEST_STATE_DIR` elsewhere must grant that path in
  `~/.config/srt/harvest.local.json`, or every confined step fails.
- **Version probe convention.** `harvest version -o json` returns the shared
  four-key build metadata object from `github.com/mad01/thismoon/buildinfo`;
  plain `harvest version` stays a bare token. There is no `/version`
  endpoint.

## See also

- Skill: `skills/harvest/SKILL.md` (the `/harvest` skill, and the system
  prompt of the model step)
- Recipe: `recipes/harvest/CLAUDE.md` (what the consuming repo adds)
- Settings: `config.md`
- Runtime behavior and failure modes: `operating.md` (`harvest docs`)
- Sandboxing: `docs/SANDBOXING.md`
