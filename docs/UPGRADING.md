# Upgrading

Actions a change needs on a consuming machine, newest first. `ralph up` pulls main and rebuilds, but it cannot run an import, add a hook entry to a private settings template, or order two merges. Each entry names the component, the PR, what to run, and how to verify it. The entry lands in the same PR as the change (root `CLAUDE.md`, "Shipping a change to machines"), and a doctor check backs it when the missing action is detectable.

Entries name this repo and the consuming repo's Claude settings template only; the machine-private wiring itself stays in the consuming repo (docs/adr/0006). After any `ralph up`, check the service's `/version`: ralph can report ok while an old binary keeps running.

## 2026-11-13 every Go component: rebuild on Go 1.27.4

Change: thismoon #489. Go 1.27.4 fixes GO-2026-6859, an HTTP/1 server crash in `net/http` that most services reach, and `go.mod` now requires it. A machine with an older Go downloads the 1.27.4 toolchain for the build, so this change and the consuming repo's Go pin can land in either order. A plain `ralph up` rebuilds only the components whose own directory changed, so most binaries would keep the old Go.

Run, in order:

1. After the consuming repo's Go pin moves to 1.27.4, `ralph up`, so code outside this repo builds on it too.
2. `ralph up --reset-builds`, so every component rebuilds.
3. Start new Claude Code sessions: a running session keeps its MCP servers on the old binaries.

Verify: `go version -m ~/code/bin/csl | head -1` prints `go1.27.4`, and so does any other thismoon binary. `zsh -lic 'go version'` run from the home directory prints `go1.27.4`.

## 2026-11-13 present: a share warns when the shared instance's client code is older

Change: thismoon #486. A share sends the page's render level, and the shared instance answers with its own. A shared instance built before this change answers none, so every share to it carries a warning that the copy may draw wrong there. Nothing is refused and the `Page` custom resource definition is unchanged, so the local presents and the instance can roll in either order.

Run: roll the shared instance to an image built from a main that includes this change (a `main` or `sha-*` tag), through the consuming repo's deployment.

Verify: `present share <id>` prints the link with no `warning:` line on stderr. The Share modal shows no warning after the status.

## 2026-11-13 belt: the memory store skip reads only the settings Claude Code reads

Change: thismoon #453. `belt hint session-start --agent claude-code` now reads `autoMemoryDirectory` from the files Claude Code reads it from: `~/.claude/settings.json`, the session's project files and the managed settings files. It reads `~/.claude/settings.local.json` only for a session started in the home directory, since Claude Code treats that file as local scope. It also keeps a store while auto memory is off. A machine that sets `autoMemoryDirectory` only in `~/.claude/settings.local.json` gets that store from belt again: Claude Code never loaded it in project sessions, so its facts reached no session.

Run: to have Claude Code load that store as auto memory, move `autoMemoryDirectory` from the consuming repo's rendering of `~/.claude/settings.local.json` into its `~/.claude/settings.json` template, then `ralph up`. Without the move, nothing is needed: belt injects the store.

Verify: run `belt doctor` from a project directory. With the key moved, the agent memory section names the store and says `autoMemoryDirectory in user settings`, followed by the full path of `~/.claude/settings.json`. With the key left in `settings.local.json`, doctor prints no such line and a new session shows the store's facts in its `belt[agent-memory]` block.

## 2026-11-13 belt: a custom guard with an empty command is a config error

Change: thismoon #435. A `custom_guards.<name>.command` that is empty now fails config validation. This holds for a disabled entry too. A config that fails validation makes every `belt hook` call deny, so fix or remove such an entry before `ralph up` installs this belt.

Run: `belt doctor` after `ralph up`.

Verify: `belt doctor` shows no config error.

## 2026-11-12 belt: `git-c-read` answers allow for read-only `git -C` calls

Change: thismoon #429. belt gains its first allow (docs/adr/0032). With it on, `belt hook bash` answers allow for a Bash call made only of read-only `git -C <dir> <subcommand>` commands whose dir sits inside the session's repo or a listed root, so the call skips the permission prompt. It is off until a machine's belt config turns it on: `ralph up` alone changes nothing.

The roots list is the safety boundary. The read commands still run code a repo's own `.git/config` names, such as `core.fsmonitor`, so list only trees of clones and repos you made. A call is approved only when the repo's work tree and git directory both sit under roots, so a linked worktree needs its folder and its main checkout listed. `/`, home and anything above home are dropped.

Run, in order:

1. `ralph up`, so `~/code/bin/belt` has the allow. A belt older than this change ignores the `allows` key, so a config that sets it first does nothing yet.
2. In the consuming repo's rendering of the belt config for each machine class that wants it, add the section below with that machine's roots, then `ralph up` again.

   ```yaml
   allows:
     git-c-read:
       enabled: true
       roots:
         - ~/code/src
         - ~/.worktrees
   ```

Verify: `belt doctor` lists `git-c-read` as `enabled` with its roots and no warning about them (a dropped root, or a root inside a git work tree). From a repo inside a root, `printf '{"tool_name":"Bash","tool_input":{"command":"git -C %s status"},"cwd":"%s"}' "$PWD" "$PWD" | belt hook bash` prints `"permissionDecision":"allow"` with a `belt[git-c-read]:` reason.

## 2026-11-12 belt: `--agent claude-code` skips a memory store Claude Code loads itself

Change: thismoon #427. `belt hint session-start --agent claude-code` leaves out an agent-memory store whose directory Claude Code's `autoMemoryDirectory` setting names. Claude Code loads that `MEMORY.md` itself, so the index stops loading twice per session. Without the flag nothing changes: Codex, pi and every machine whose settings entry is unchanged get both stores as before.

Order matters: install the new belt on every machine first, change the settings entry second. A belt that predates this change fails on the unknown flag, and the session then starts with no session-start hints at all.

Run, in order:

1. `ralph up` on every machine, so `~/code/bin/belt` knows the flag.
2. On each machine, gate on the binary: `belt hint --help` must list `--agent`.
3. Only after step 2 passes on every machine, change the Claude Code SessionStart entry in the consuming repo's Claude settings template from `belt hint session-start` to `belt hint session-start --agent claude-code`, then `ralph up` again. Leave Codex and pi entries without the flag.

Verify: on a machine where `autoMemoryDirectory` points at a store, the agent memory section of `belt doctor` prints a line naming that store and the settings file. The line must not end in "so sessions here still load it twice"; that ending means the SessionStart entry still lacks the flag. In a new Claude Code session the store's facts appear once.

## 2026-11-12 belt: the large-read guard refuses whole-file reads of large files

Change: thismoon #428. `belt hook read` runs a new guard, large-read, before every Read. It refuses a Read with no `offset` and no `limit` of a text file of 300 lines or more in a csl-indexed repo and hands over the `csl_ask` call for that file; a ranged read passes. It applies to subagents too. Nothing changes until the consuming repo's Claude settings template runs the new command, and `belt doctor` does not check for the entry.

Run, in order:

1. `ralph up` on every machine, so `~/code/bin/belt` knows the `read` event. An older belt rejects `belt hook read` as an unknown event, so a Read hook added first errors on every Read.
2. Add a `hooks.PreToolUse` entry to the consuming repo's Claude settings template, beside the other `belt hook` entries: matcher `Read`, command `~/code/bin/belt hook read`. Then `ralph up` again.
3. Optional: set the top-level `large_file_lines` in the consuming repo's belt config to move the threshold for the guard and the read-size hint together. To switch the guard off on a machine class, set `guards.large-read.enabled: false` there.

Verify: `belt doctor` lists `large-read  read  enabled  (large_file_lines: 300)`. `belt check read --file <a file of 300+ lines in an indexed repo> --transcript /dev/null` prints `DENY  large-read` and exits 1. In a new session, a whole-file Read of such a file is refused with `belt[large-read]:` and the `csl_ask` call.

## 2026-11-10 toss-bin: `--temp-only` for agent scratch cleanup

Change: thismoon #415. `toss-bin --temp-only` refuses the whole run unless every path sits strictly inside `/tmp`, `/var/tmp` or `~/.tmp`, so a Claude Code allow rule for that one invocation can replace the `rm` prompt for temp cleanup. The roots are fixed in the binary. With the rule in place, an agent can trash anything under those roots with no prompt, backups parked in `~/.tmp` included; it stays recoverable from `~/.Trash/my-trash/`.

Order matters: update the binary on every machine first, add the allow rule second. A toss-bin that predates this change ignores `--temp-only` as an unknown flag and trashes any path, without safe mode.

Run, in order:

1. `ralph up` on every machine, so `~/.local/bin/toss-bin` has the flag.
2. On each machine, gate on the binary: `toss-bin --temp-only --validate ~/notes.txt` must print `BLOCKED:` and exit 1. An old binary ignores the flag and prints `OK:`. `--version` cannot tell them apart: both print 1.3.1 until the release bumps it.
3. Only after step 2 passes on every machine, add `Bash(toss-bin --temp-only *)` to `permissions.allow` in the consuming repo's Claude settings template, then `ralph up` again.

Verify: `command -v toss-bin` prints `~/.local/bin/toss-bin`. `toss-bin --temp-only --validate ~/notes.txt` prints `BLOCKED:` and exits 1, and `toss-bin --temp-only --validate /tmp/x` prints `OK: /tmp/x`.

## 2026-11-06 moondb: Codex sessions through Codex's own hooks

Change: thismoon #408. `moondb record` takes Codex CLI's hook payloads, and `moondb trace` reads Codex rollout files. Capture is opt-in: nothing changes until Codex runs the hooks.

Run, in order:

1. `ralph up`, so `~/code/bin/moondb` has the change.
2. Add UserPromptSubmit and Stop entries to the consuming repo's Codex hooks file, each running `MOONDB_HARNESS=codex ~/code/bin/moondb record` (moondb README, "Codex capture"), then `ralph up` again.
3. Open `codex`, run `/hooks`, and trust the two new hooks. Codex skips a new or changed hook until it is trusted.

Verify: after one Codex prompt, `moondb sessions --since 1h` lists the session with the harness `codex`, and `moondb trace <session-id>` shows its model spans with tokens.

## 2026-11-05 brain: `brain serve` runs sandboxed, so legacy indexes leave `~/.config/brain`

Change: thismoon #401. brain gains `brain serve`, the read-only web view at brain.this on port 7427. The new `recipes/brain/` builds brain as `packages.brain` and registers the t-man agent `brain-serve`, which runs serve under srt. The sandbox writes only `~/.local/state/brain`. brain keeps its state in `~/.config/brain` while `~/.config/brain/index/state.json` exists, so an install that still has its indexes there sets them aside first.

Run, in order:

1. End the agent sessions that run the brain MCP server. An MCP server on the old binary rebuilds `~/.config/brain/index/state.json` on its next search, which sends brain back to the old state directory.
2. Merge this change, then the consuming repo's brain change right after it, with no `ralph up` in between: until both are in, both repos declare `packages.brain`. The consuming repo's change stops building brain itself and ships the srt expansion layer `~/.config/srt/brain-serve.local.json`, which grants the vault and the target of the `config.yaml` link.
3. Set the legacy indexes aside. Nothing in them carries over, because both indexes rebuild after this upgrade (see below). Renaming `index` also removes the probe file. Skip a line whose directory does not exist.

   ```sh
   mv ~/.config/brain/index ~/.config/brain/index.legacy
   mv ~/.config/brain/semantic ~/.config/brain/semantic.legacy
   ```

   Delete the two `.legacy` directories once `brain doctor` passes. A config that sets `semantic.index_dir` must drop the key, or the semantic store stays outside the sandbox's write grant; the consuming repo's change drops it from its config.
4. `ralph up`, then `t-man restart brain-serve`, so serve starts on the new state directory.
5. Start new agent sessions, so the MCP server runs the new binary.

brain now commits each change it makes to a vault that is a git repo, staging only the paths it touched. It never pushes. `autocommit: false` on a vault entry turns the commits off.

After the upgrade, two rebuilds run once on their own, and neither needs a command:

- Every literal index rebuilds on its first refresh, because the vault fingerprint now counts mtimes in nanoseconds.
- The first semantic refresh re-embeds every note, because the semantic manifest moved to version 2.

Verify:

1. `curl -s 127.0.0.1:7427/version` reports the merged commit.
2. `brain doctor` exits 0. Its `index-dir` line names `~/.local/state/brain/index`, and `serve-reachable`, `version-skew`, `serve-vaults` and `serve-state-dir` are ok. A `serve-vaults` failure on an unreadable vault means the expansion layer lacks that path; `srtspec explain brain-serve` prints what serve may read.

## 2026-11-05 moondb: Stop waits for the turn's last lines, and a turn keeps up to 15 MiB

Change: thismoon #395. Claude Code writes the transcript asynchronously, so at Stop the file usually still lacked the turn's last response, and `moondb record` stored the turn without it. The Stop hook now waits up to 1 s for the transcript to hold the payload's final message before it reads the turn's lines. A turn's transcript chunk now keeps up to 15 MiB instead of 4 MiB, so a long turn keeps its head. An agent transcript keeps up to 15 MiB instead of 8 MiB.

Run, in order:

1. Merge thismoon #394 first, then this change, then `ralph up`. The hooks run the new `moondb record` from their next call.
2. With no Claude Code session active, run `moondb import --since 30d --force`. Run it again on a machine that already ran it for #394: that build kept at most 4 MiB of each turn.

Verify:

1. `moondb version -o json` and serve's `/version` report the merged commit.
2. For a session with a turn over 4 MiB of transcript, `moondb trace <id> --json | jq .totals.model_calls` matches the transcript's main-thread calls, counted with the `jq` line in the #394 entry below.
3. On a build with the `cut-turns` doctor check, `moondb doctor` prints `ok cut-turns` with no note. A note counts the turns the old cap still cuts and names the import that repairs the ones whose transcript is still on disk.

## 2026-11-05 moondb: a forced import repairs turns that kept too few model calls

Change: thismoon #394. A prompt typed while a turn runs fires UserPromptSubmit under the running turn's prompt id. `moondb record` moved the turn's transcript offset to that point. The Stop then kept only the lines after the queued prompt, and the trace lost the turn's earlier model calls and their cost. record now keeps the turn's first offset. A forced import now also replaces a turn's transcript chunk when the transcript holds other model calls for that turn. That repairs the turns recorded before this change, and turns whose Stop read the transcript before the harness wrote its last lines.

Run, in order:

1. Merge this change, then `ralph up`. The hooks run the new `moondb record` from their next call, and the import repair needs the new serve.
2. With no Claude Code session active, run `moondb import --since 30d --force`. A forced import closes in-flight turns, so the machine must be quiet. Sessions older than 30 days need a wider `--since`.

Verify:

1. `moondb version -o json` and serve's `/version` report the merged commit.
2. For a session from before the change whose transcript holds `queue-operation` lines, `moondb trace <id> --json | jq .totals.model_calls` matches the transcript's main-thread calls: `jq -r 'select(.type=="assistant" and (.isSidechain|not)) | .message.id' <transcript> | sort -u | wc -l`. A turn that ran past 4 MiB of transcript still loses its head to the cap, so such a session stays lower.

## 2026-11-01 belt: read-size also fires on `csl_read`, and its matcher must name the tool

Change: thismoon #363. belt's read-size hint now also fires after a `csl_read` call that reads a file of 300 lines or more with no `start_line` and no `end_line`. The advice is the same as after a Read. belt only sees the calls its PostToolUse matcher routes to `belt hint read`, so the consuming repo's matcher has to name the MCP tool too. A Read and a `csl_read` of one file now share one nudge per agent per session. The seen-file marker now names the file by repo and repo-relative path. A session that spans the upgrade can therefore get the hint once more for a file it already saw it on.

Run, in order:

1. Merge this change, then `ralph up` to rebuild belt.
2. In the consuming repo's Claude settings template, change the matcher of the `belt hint read` PostToolUse entry from `Read` to `Read|mcp__csl__csl_read`. If the csl MCP server is registered under another name, use that name in the middle segment. Merge that change and run `ralph up` again so agent-sync writes it.
3. Start a new agent session, so Claude Code loads the new matcher.

A machine that skips step 2 keeps the Read half of the hint and stays silent on `csl_read`. `belt doctor` reports the gap: its hook wiring section prints a `WARNING` line naming the matcher to set. That line leaves the exit status alone, since the hint is advisory.

Verify:

1. `belt version -o json` reports the merged commit.
2. `belt doctor` lists the `belt hint read` matcher under hook wiring, with no `WARNING` line in that section.
3. Optionally, hand belt a `csl_read` payload. It fires only where csl indexes `mad01/thismoon`, and only once per file per session, so each run takes a fresh session id. belt then prints a JSON object whose `additionalContext` starts with `belt[read-size]: mad01/thismoon/tools/belt/docs/hooks.md` and says the file "was read without a range through csl_read":

```sh
printf '{"session_id":"%s","tool_name":"mcp__csl__csl_read","tool_input":{"repo":"thismoon","file":"tools/belt/docs/hooks.md"},"tool_response":[{"type":"text","text":"mad01/thismoon/tools/belt/docs/hooks.md lines 1-500 of 1250"}]}' "$(uuidgen)" | belt hint read
```

In a new session, a `csl_read` of a file that long with no range carries the same `belt[read-size]:` reminder next to its result.

## 2026-11-01 csl: `csl_ask` needs the MCP server started through `csl-env.sh`

Change: thismoon #359. csl gains `csl_ask` and `csl ask`, a bulk reader: a question over the files a zoekt search selects, answered by a hosted worker model so the files never enter the agent's context. The worker is a LiteLLM proxy when `LITELLM_BASE_URL` is set, else the `claude` CLI on `PATH`, else OpenRouter when `OPENROUTER_API_KEY` is set. An MCP host starts `csl mcp` without a login shell's environment, so the csl recipe now ships `recipes/csl/csl-env.sh`. It pulls exactly those variables from `~/.config/ralph/secrets.sh` and execs csl. Search works as before without any of it; only `csl_ask` needs a worker.

Run, in order:

1. Merge this change, then `ralph up`. It rebuilds csl and keeps `csl-env.sh` executable in the sources cache.
2. Point the consuming repo's MCP registration for `csl` at the launcher: the command becomes `~/.config/ralph/sources/thismoon/recipes/csl/csl-env.sh` with the argument `mcp`, in place of `csl mcp`. Merge that change and run `ralph up` again so agent-sync writes it.
3. Start a new agent session, so the client spawns the server through the launcher.

A machine that skips step 2 keeps a working csl server. There `csl_ask` reaches the `claude` CLI when it is on the host's `PATH`, and otherwise fails naming what to set.

Verify: `csl doctor` prints an `ask-backend` line naming the backend and model. The `csl_doctor` tool lists `ask-backend` among its checks. `csl ask "which flags does csl search accept" -q 'sym:searchCmd' --repo thismoon --max-files 2` prints an answer on stdout and a `corpus:` footer on stderr.

## 2026-10-30 standalone installs: the Homebrew tap is gone, mise is the one-tool path

Change: thismoon #351. The `mad01/homebrew-tap` repository is deleted, so `brew tap mad01/tap` and every `mad01/tap/<name>` formula stop working. The install docs now send a one-tool install, and the ralph install, through mise. A fleet machine that never tapped `mad01/tap` needs nothing: `ralph up` builds every component from source as before.

Run, on a machine that ran `brew tap mad01/tap`:

1. `brew list --full-name | grep '^mad01/tap/'` lists the formulas installed from the tap. For each one, run `brew services stop mad01/tap/<name>` if it ran as a service, then `brew uninstall mad01/tap/<name>`.
2. `brew untap mad01/tap`.
3. Reinstall each tool through mise: `mise tool-alias set <name> github:mad01/thismoon`, then `mise use -g "<name>[version_prefix=<name>/]"`. ralph installs with `mise use -g github:mad01/ralph`. A service that ran under `brew services` moves to a t-man agent on the mise shim path, for example `t-man add --name csl-web -- "$HOME/.local/share/mise/shims/csl" web --port 7424`.

Verify: `brew tap` no longer lists `mad01/tap`. `<name> version` prints a release version, and `which <name>` points under `~/.local/share/mise`.

## 2026-10-29 moondb: the last parent message before a subagent's edit needs a forced import for past agents

Change: thismoon #347 (MAD-524). moondb now keeps each message a subagent's parent sent it after its spawn brief, such as a lead's follow-up to a teammate or a SendMessage to a running subagent, with the time it arrived and its sender. `why` shows the last one that arrived before the edit under the spawn brief: in the CLI as a `last message:` line, in the `moondb_why` tool as `last_message`, and on the session page. `moondb_turn` and a turn's page show each subagent's last message there. The SubagentStop hook entry stays as it is: the messages come from the transcript the hook already reads. The `agent-briefs` doctor check now also counts agents that have a brief but lack the later messages their count names, and `GET /api/doctor` reports that count as `agents_to_fill`. For this release it also repeats it under the old key `agents_without_brief`, so a CLI and a serve from different builds agree.

Run: `ralph up`, then check that `curl http://127.0.0.1:7433/version` reports the merged commit. Agents recorded before this build kept no later messages, and `moondb doctor` notes `agent-briefs` with `N agents without a spawn brief or later messages; run moondb import --since 30d --force`. To fill them, run `moondb import --since 30d --force` while no other session is working, and let serve drain the spool. The import keeps each stored agent and adds only what it lacks. An agent whose transcript file is gone keeps only its count.

Verify: `moondb doctor` shows `agent-briefs` with no count. Pick a line a teammate wrote after its lead sent it a follow-up message. `moondb why <path>:<line>` on it prints a `last message:` line under the `brief:` line, quoting that follow-up with its time and sender.

## 2026-10-29 csl: `csl search --json` prints an object, not an array

Change: thismoon #342 (MAD-515). `csl search --json` now prints one JSON object instead of a bare array of matches. The matches sit under `matches`. After an empty first search the object also carries `relaxed_query` and `dropped_terms`, or `zero_result_hint`. Text and `--toon` output keep their shape.

Run: `ralph up` to rebuild the binary. A script that decodes the old array reads `.matches[]` now, for example `csl search <q> --json | jq '.matches[]'` in place of `jq '.[]'`.

Verify: `csl search <q> --json | jq '.matches | length'` prints a number. On the old binary, jq fails because an array has no `matches` key.

## 2026-10-29 moondb: subagent spawn briefs need a forced import for past agents

Change: thismoon #343 (MAD-496). moondb now keeps each subagent's spawn brief, the prompt it was spawned with, read from the first user line of its own transcript. It also counts the messages its parent sent it after that brief. `why` and a turn show both beside the turn's prompt, in the CLI, in the `moondb_why` and `moondb_turn` tools and on the session page. The SubagentStop hook entry stays as it is: the brief comes from the transcript the hook already reads. `moondb doctor` gains an `agent-briefs` check that counts the agents still without a brief.

Run: `ralph up`, then check that `curl http://127.0.0.1:7433/version` reports the merged commit. Agents recorded before this build have no brief and show it as unknown, and `moondb doctor` prints `ok agent-briefs (N agents without a brief; run moondb import --since 30d --force)`. To fill them, run `moondb import --since 30d --force` while no other session is working, and let serve drain the spool. The import keeps each stored agent and adds only the missing brief and its message count. An agent whose transcript file is gone keeps an unknown brief.

Verify: `moondb doctor` prints `ok agent-briefs` with no count. `moondb why <path>:<line>` on a line a subagent wrote prints an `agent:` line and a `brief:` line with the quoted brief, not "unknown". `moondb_turn` on a turn that ran a subagent returns `briefs`, and the session page shows a "Subagent spawn briefs" section under that turn's prompt.

## 2026-10-29 belt: the search and prompt hint events are gone

Change: thismoon #344 (MAD-517). belt drops its `search` and `prompt` hint events. No hint has run on either since keeper-of-facts left, so their hook entries spawned a process on every csl search and every prompt for no output. The pi package stops calling them in the same change. belt now rejects both as unknown events, so an entry that still calls one errors on each call.

Run, in order:

1. Merge the consuming repo's change that drops the PostToolUse `belt hint search` entry and the UserPromptSubmit `belt hint prompt` entry from its Claude settings template, and the `belt hint search` entry from its Codex hooks file. The old belt runs fine without them, so this can land first.
2. `ralph up`. It rebuilds belt and syncs the template into `~/.claude/settings.json`. agent-sync replaces the live hook arrays with the template's, so no prune entry is needed.

A machine that gets this belt before its template drops the entries shows a hook error on every csl search and every prompt until it does. The errors do not block the call or the prompt.

Verify: `belt hint search </dev/null` exits 1 with `unknown hint event "search"`. `grep -E 'belt hint (search|prompt)' ~/.claude/settings.json ~/.codex/hooks.json` prints nothing.

## 2026-10-29 csl and belt: one csl rules file, the CLI for fleet search

Change: thismoon #338 (MAD-514, MAD-518). The agent-facing csl rules now live in one file, `services/csl/claude-md.md`. It sends fleet-wide searches and anything that would page to the `csl` CLI, and keeps the MCP tools for answers that fit one call. The csl recipe links the file to `~/.claude/CLAUDE.csl.md`. `examples/dotfiles/CLAUDE.csl.md` is gone, and the eval harness's default `-steer-file` is the new file. belt's `prefer-csl` hint now leads with the `csl search` command. It names the repo the command searched, after a `cd`, a `git -C` or an absolute path, instead of the session's. It stays quiet on a grep of one or two named files.

Run: `ralph up`. It links `~/.claude/CLAUDE.csl.md` and rebuilds belt. If a regular file already sits at that path, ralph moves it to `~/.claude/CLAUDE.csl.md.bak.<timestamp>` and links in its place. The global CLAUDE.md line that imports the file, `@~/.claude/CLAUDE.csl.md`, lands separately in the consuming repo (MAD-519). Until it does, the link exists but no session reads it.

Verify: `ls -l ~/.claude/CLAUDE.csl.md` shows a symlink ending in `services/csl/claude-md.md` inside the thismoon sources cache. In a session whose cwd is one indexed repo, `cd <another indexed repo> && grep -rn foo .` gets a `belt[prefer-csl]` hint that starts with `csl search` and names the other repo.

## 2026-10-28 catalog and keeper-of-facts: removed

Change: thismoon #332. The catalog and keeper-of-facts (kof) services leave the repo with their recipes. deps keeps reading `~/.config/catalog/registry.yaml`, which the consuming repo still installs. moondb's scorecard drops its `kof` source and the `hints.kof` block and now reports `version` 2. belt loses its kof-consult, kof-assertions and kof-deposit hints and its kof doctor probe.

Run, in order:

1. In the consuming repo, drop the `kof` and `catalog` routes from the d-man routes file. In its belt config, delete every `hints.kof-*` block (kof-consult, kof-assertions, kof-deposit). The new belt still runs its guards and hooks with an unknown hint id in the config, but `belt doctor` reports it and exits 1.
2. `ralph up --enable-cleanup`. The first apply after a recipe removal needs the flag. It runs the recipes' `pre_uninstall` hooks, which stop both agents. It also removes the binaries and applies the route and config edits.
3. If `t-man list` still shows them, run `t-man remove catalog-web` and `t-man remove keeper-of-facts`.
4. Delete the srt links `~/.config/srt/catalog-web.json` and `~/.config/srt/keeper-of-facts.json`, and any copies of them under `~/.config/srt/merged/`.
5. Run `claude mcp remove --scope user kof`. agent-sync's `mcp.prune` is off by default, and with it off, dropping kof from `servers.json` leaves the server registered. On Codex, delete the `[mcp_servers.kof]` table from `~/.codex/config.toml`.

The assertion store stays at `~/.local/share/kof`, and the t-man logs stay under `~/Library/Logs/keeper-of-facts` and `~/Library/Logs/catalog-web`. Nothing reads them now. Whether to delete them is your call.

Verify: `t-man list` shows neither `catalog-web` nor `keeper-of-facts`. d-man's managed block no longer names the routes, so `grep -E 'kof\.this|catalog\.this' /etc/hosts` prints nothing. `claude mcp list` has no `kof` entry. `belt doctor` reports no kof probe and no unknown-hint warning. `moondb scorecard <session-id> --json` reports `"version": 2`.

## 2026-10-28 moondb: sessions keep their cwd repo, entrypoint and tags

Change: thismoon #320. A session now records the repository its cwd sits in, the harness entrypoint (`cli`, or `sdk-cli` for `claude -p`) and the tags a launcher sets in `MOONDB_SESSION_TAGS`. A sessions row lists the repos the session worked in without editing them (`cwd_repos`): its own cwd's and each subagent's cwd repo. The repo filter and chips count the session under its edited repos and its `cwd_repos`. The REPO column shows the first of them as a badge, hollow for a worked-in repo, and folds the rest into "+N"; the session page shows a badge for each. The list also marks headless and tagged sessions. The eval runner tags its sessions `eval`.

Run: `ralph up`. The subagents' repos in `cwd_repos` need no import, since recorded agents already carry their location. A session's own cwd location and its entrypoint still need `moondb import --since 30d --force` on sessions recorded before this build; run it while no other session is working. A held session with no edit and no subagent is now imported too, for this. A session whose transcript file is gone keeps neither. A forced import fills a location only when the session's cwd still exists or can be placed, so eval sessions that ran in a throwaway `worktree:` workspace under `~/.cache/harness-eval` get none. Tags cannot be backfilled: they exist only in the hook's environment.

Verify: on `http://moondb.this/`, an older eval session such as "Where is the Go function `CanonicalRepo` defined" shows a hollow `mad01/thismoon` badge in the REPO column instead of "no repo". A `headless` badge sits beside its title, and its session page lists `sdk-cli` as the entrypoint. A session started from a scratch directory whose subagents only read in a repo shows that repo as a hollow badge and appears under `?repo=` for it.

## 2026-10-26 belt: kof-consult runs on the prompt, not at session start

Change: thismoon #309 (MAD-492). belt's `kof-consult` hint now runs on UserPromptSubmit and shows the repo's assertions that the prompt is about, in place of the whole repo's list at session start. The kof hints' events now carry `assertions` and `pins` tags.

Run: nothing in the settings template. Its UserPromptSubmit entry already runs `belt hint prompt`, so `ralph up` is enough. Global instruction files that call `belt[kof-consult]` a session-start injection are now out of date. Reword them when convenient, for example to "each prompt surfaces the cwd repo's assertions that it is about".

Verify: `belt doctor` lists `kof-consult` on the `prompt` event. In a new session inside a repo with assertions, a prompt that names one of them gets a `belt[kof-consult]:` block, and the session start gets none.

## 2026-10-26 belt and moondb: in-flight files at session start need moondb first

Change: thismoon #311 (MAD-493). A new belt session-start hint, `moondb-inflight`, asks moondb's new `GET /api/inflight` route which files of the checkout other sessions edited and no commit has taken yet.

Run: nothing in the settings template, since `belt hint session-start` is already wired. Install moondb before belt: `ralph up`, then check that `curl http://127.0.0.1:7433/version` reports the merged commit. A belt on an older moondb gets a 404 from the route, and the hint stays silent until moondb catches up.

Verify: `belt doctor` reports moondb serve as `reachable`, not as older than this belt. `moondb inflight` in a checkout prints a table or "no files".

## 2026-10-26 belt: read-size hint needs a Read hook entry

Change: thismoon, belt `read-size` hint (#308, MAD-494). The hint fires when a Read with no offset or limit pulls in 300 lines or more from a csl-indexed repo, for a file type `csl_outline` has symbols for. It points at `csl_outline` or a ranged read, once per file per agent per session.

Run: merge this change and `ralph up` first, so the installed belt knows the `read` event. Then, in the consuming repo's Claude settings template, add a PostToolUse group with matcher `Read` running `~/code/bin/belt hint read`. Order matters: an older belt fails `belt hint read` at argument parsing, and Claude Code reports that hook error on every Read. The hint is on by default; `hints.read-size.enabled: false` in the belt config switches it off.

Verify: `belt doctor` lists `read-size` on the `read` event. In a new session, Read a file of 300 lines or more from an indexed repo with no range; the result carries a `belt[read-size]` note naming `csl_outline`.

## 2026-10-26 moondb: subagent cost needs a SubagentStop hook entry

Change: thismoon #298. moondb records each subagent when it stops, from its own transcript. The turn and the session then add the subagent's cost to their own.

Run: in the consuming repo's Claude settings template, add a `SubagentStop` hook group with no matcher, running `~/code/bin/moondb record` asynchronously. Without a matcher, Claude Code's internal agents count too. Merge this service first and `ralph up`, then the hook entry. Then run `moondb import --since 30d --force` so the last month's sessions get their subagent cost. Without `--force` the import skips every session the store already holds. A session where no file was edited is imported too when it ran a subagent, so review and research sessions get their agents; one with neither stays skipped. Run it while no other session is working: it closes an open turn until that turn's Stop arrives.

Verify: `moondb doctor` reports `hooks-wired` ok. A new session that spawns one subagent shows it in the Agents section on `http://moondb.this/`, with a cost.

## 2026-10-25 moondb: Bash capture needs a setting and a hook entry

Change: thismoon #281 and #288. The Bash hook records the files a Bash command changed, from the `bashEditDiff` that Claude Code attaches to the tool response.

Run: in the consuming repo's Claude settings template, set `bashEditDiffEnabled` to `true` and add a PostToolUse group with matcher `Bash` running `~/code/bin/moondb record` asynchronously; then `ralph up` in a login shell. Claude Code 2.1.269 or later.

Verify: `jq .bashEditDiffEnabled ~/.claude/settings.json` prints `true`. In a new session, change a file with `sed` in Bash; the turn lists the file on `http://moondb.this/`.

## 2026-10-25 moondb: import the transcript history once per machine

Change: thismoon #279. `moondb import` reads the Claude Code transcripts on the machine and records their sessions, turns and edits. Live capture starts once the hooks run, so a machine's earlier history stays empty until the import runs.

Run: `moondb doctor` green, then `moondb import --since 30d --dry-run`, read the counts, then `moondb import --since 30d`. Wait for `~/.local/state/moondb/spool` to drain. Re-runnable: a replayed record changes nothing.

Verify: the session count on `http://moondb.this/` matches the dry run. Older history: `--since 60d` or `90d`.

## 2026-10-25 moondb: first install, service before hooks

Change: thismoon #272 and #273. New service on port 7433 with a web UI, CLI and MCP server, fed by Claude Code hooks.

Run: the consuming repo registers the companion recipe, the MCP server, the `moondb.this` route and the hook entries (UserPromptSubmit and Stop asynchronous, PostToolUse `Edit|Write|MultiEdit|NotebookEdit` synchronous). Merge the service first and `ralph up`, then the hook entries: a hook that runs before the binary exists logs a missing command in every session.

Verify: `moondb doctor` all ok; `curl http://127.0.0.1:7433/version` reports the merged commit.
