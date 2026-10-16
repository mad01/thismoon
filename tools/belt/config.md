# belt configuration

## Vocabulary

Four terms recur in this file and in the `belt doctor` output:

- rendering: one fully written belt config file for one machine class.
  Nothing in it is conditional. A rule that should hold on only some
  machines is present in their rendering and absent from the others.
- machine class: personal or work. Decided at provisioning time, never
  at hook time; belt has no runtime switch for it (docs/adr/0010).
- provisioning layer: whatever installs the rendering and the hook
  entries on a machine. In the maintainer's fleet that is ralph reading
  recipes from a private config repo. Any tool that writes two files will do.
- consuming repo: the private repo that carries the renderings and the
  hook registration. belt ships none of it (docs/adr/0006).

## Where config lives

belt's own settings live in `~/.config/belt/config.yaml` (YAML), or
`$XDG_CONFIG_HOME/belt/config.yaml` when that variable holds an absolute
path. `--config <path>` and `$BELT_CONFIG` relocate the file; the flag wins
over the variable, and a leading `~` is expanded. A relocated file is read as
YAML with no legacy fallback.

A legacy `~/.config/belt/config.toml` beside the default YAML file is read
only when the YAML file does not exist at all. If the YAML file exists but
fails to parse, belt does not fall back to the TOML file.

**Absent and invalid are different answers.** No config file means the
built-in defaults: every guard and hint enabled, no rules configured. A
config file that is present but fails to parse or validate is an error, and
every `belt hook` invocation then denies with the reason
`belt[config]: belt cannot read its config …` naming the file. Belt is a
guard: it cannot tell "no rules configured" from "the rules did not load".
Treating the second as the first is how one typo silently disarms
`write-internal-names` and every commit rule at once. The way out is to fix
the file — or move it aside, which is the explicit way to ask for the
defaults. `belt doctor` and `belt config` keep working in that state and
print the built-in defaults, with a line saying those defaults are not what
is being enforced.

Validation rejects the values that parse as YAML and then quietly do
nothing:

- a `custom_guards.<name>.event` that is not `bash` or `write` (including a
  missing one) — the guard would register on an event that never fires;
- a `custom_hints.<name>` entry whose `event` is not `session-start` or
  `subagent-start` (the events custom hints run on), whose `command` is
  empty, or whose name is a built-in hint id, since the entry would shadow
  the built-in;
- a `custom_hints.<name>.agent_types` key on a hint whose `event` is not
  `subagent-start` (only a subagent has a type to match), or one that is
  empty or holds a blank entry;
- a `mode:` anywhere other than `hard` or `soft`;
- `mode: soft` under `guards:` or `hints:` on any id except
  `script-deny-list` and `publish-internal-names`, the two that read a
  toggle mode. Use `enabled: false` to switch a different guard off;
- any list key on a built-in id that does not read it. Each id declares
  the toggle fields it reads (the `GuardFields`/`HintFields` tables in the
  config package), and the error names the fields that do work there.
  Guards exempt repos with `allow_repos`, hints opt them out with
  `exclude_repos`, so each key on the other kind is always rejected. Ids
  the binary does not know (custom guards, or a config targeting a newer
  belt) stay lax on purpose. Rejecting unknown keys would turn ordinary
  config/binary rollout skew into a machine-wide deny.

The `ledger` and `watch` values are never rejected, for the same reason. A
config error denies every guarded tool call, which is far too much for a
typo in a feature that may be off. Out-of-range numbers are clamped and
source names are rewritten the way the events service stores them. `belt
doctor` warns about each adjustment, naming the key and the value in effect,
and `belt config` prints the effective values. Only a value of the wrong YAML
type still fails the file, as in every other section. `ledger.agent_types`
is the one exception: any shape loads, and a malformed entry or value is
dropped with a doctor warning.

Overrides stay in `~/.config/belt/overrides/` (or the `XDG_CONFIG_HOME`
equivalent) whatever `--config` points at: an override is machine state set
from the CLI, not part of a rendered config. Files beginning with a dot in
that directory are ignored rather than read as malformed overrides.

The belt config is standalone: belt never reads another tool's config file,
and there is no machine-profile concept in it (docs/adr/0010 records the
decision). The provisioning layer renders one config per machine class. So
a guard or rule that should exist only on some machines is simply absent
(or disabled) in the other classes' files. `git-push-main`, for example,
ships enabled and denies every push to main until a machine's rendered
config disables it or allowlists a repo.

Exactly one non-belt surface is read, visible in `belt doctor`: the Claude
Code settings. The Bash deny patterns behind `script-deny-list` come from
the `permissions.deny` entries of `~/.claude/settings.json` and
`~/.claude/settings.local.json` on every invocation. They are read live,
never cached and never copied into the belt config, so the guard and the Claude
Code permission system can never drift apart. This read has its own switch,
`claude_settings.enabled` (see Keys); `belt config` shows the patterns as
`claude_deny` in its output, and `belt config --help` prints the full
annotated key reference reproduced in the Example section below.

`internal_names` has no fallback: an absent or empty section means an empty
name set, and `write-internal-names` then has nothing to match (`belt
doctor` calls this out). The section is deliberately shape-compatible with
the `guard:` section of the suspenders config. Since docs/adr/0016 the
three name lists can also live in one shared names file that both tools
list under their own `include` key. That file is where the two tools' name
configs stay in step; neither reads the other's config.

Run `belt config` to see which files loaded (or failed to) and every setting
belt is actually running with, after every default and fallback is applied.
Run `belt doctor` to see the state those settings produce. It lists enabled
guards and hints, active overrides, the ledger settings with the
`ledger.agent_types` map, the watch settings, the events service both depend
on, and the resolved blocked-name list. The report ends
with a warnings section and exits 1 when it is not empty.

That section flags a broken config or include, a script guard with no deny
patterns, an internal-name guard with no names, or a toggle nothing answers
to. It flags an enabled watcher that cannot work, because the events
service is unreachable or the max wait reaches the hook timeout. It also
names each ledger or watch value it had to clamp, rewrite or drop. A machine with
no config at all exits 1 for the script and internal-name cases by design.

## Keys

### direct_main_repos (top-level)

The one shared repo list, named for the workflow fact it states: repos
whose workflow is direct-to-main (single-writer store clones, config repos
that never take PRs). Patterns are canonical `host/owner/repo` or a
trailing `/*` org wildcard. Exactly two checks read it — the two whose
subject is that workflow: `git-push-main` (the push is allowed there) and
the `commit-policy` hint (the branch + PR advice is silenced there). No
other check consults it, so editing this list can never disarm an
unrelated guard. A repo that needs an exemption from anything else uses
that check's own `allow_repos`/`exclude_repos` (docs/adr/0013 records the
reversibility criterion behind this).

### public_repos (top-level)

The repos whose content is public, or headed there: the only places the
internal-name guards block internal names. Purpose-named like
`direct_main_repos` and read by exactly the two checks whose subject is
that fact, `write-internal-names` and `publish-internal-names`; nothing
else consults it. Patterns are canonical `host/owner/repo` or a trailing
`/*` org wildcard.

A repo on the list is guarded unless that guard's own `allow_repos`
carves it out (a private repo inside a listed org). A repo not on it
allows internal names in files, commits, branch names, and PR text alike.
As a result, an internal org that happens to live on github.com never
needs an exemption.

Leaving the key out entirely keeps the rule the guards started with:
every github.com repo is public-bound. So a rendering that predates the key
stays strict rather than silently unguarded; `belt
doctor` prints which of the two modes is in effect. An empty present list
(`public_repos: []`) guards nothing. docs/adr/0015 records the decision.

### internal_names

The name-derivation source for the `write-internal-names` and
`publish-internal-names` guards. Belt-owned and standalone: absent or empty
means an empty name set (see Where config lives).

- `internal_names.include` (list of string, default: empty): paths of
  shared names files, `~` expanded. Each file carries `blocked_words`,
  `allowlist`, and `allow_phrases` and nothing else; any other key is an
  error naming the file. The three lists are appended to the section's own.
  A listed file that is missing or fails to parse is a config error with the
  same consequence as a broken `config.yaml`. Every `belt hook` call denies
  with `belt[config]:` naming the include file until it is fixed or the
  entry is removed. `belt doctor` prints one line per include with its state
  (loaded with counts, MISSING, or BROKEN) and `belt config` lists each in
  its header. suspenders names the same file from its own config; neither
  tool reads the other's config (docs/adr/0016).
- `internal_names.workspace_dirs` (list of string, default: empty): directories
  to search for git repos. Every repo found contributes its origin remote's
  org segment, repo segment, and checkout directory basename as three
  separate blocked names (never the combined `org/repo` form).
- `internal_names.blocked_words` (list of string, default: empty): extra
  words blocked outright, independent of any discovered repo.
- `internal_names.allowlist` (list of string, default: empty): names (or
  their basename) to drop from the derived set even though they would
  otherwise be discovered or listed. The final set is lowercased,
  deduplicated, and drops anything under three characters.
- `internal_names.allow_phrases` (list of string, default: empty): exact
  phrases (case-insensitive) neutralized in the checked content before name
  matching. Use it when a sanctioned compound contains a blocked name, a
  private companion repo's own name, say. So writing the compound passes
  while the bare name anywhere else in the same content still denies.
  Unlike `allowlist`, nothing leaves the blocked set.

### claude_settings

The switch on belt's read of the Claude Code settings files, the only
non-belt config surface belt reads (docs/adr/0010). The only thing belt takes from those files is the
`permissions.deny` Bash entries feeding `script-deny-list`; belt never
writes them.

- `claude_settings.enabled` (bool, default `true`): whether belt opens
  `~/.claude/settings.json` and `~/.claude/settings.local.json` at all.
  Disabling it leaves `script-deny-list` matching only its
  `extra_patterns` — the guard stays on, with a smaller pattern set — and
  both `belt doctor` and `belt config` state that the read is off.

### git_identity

A list of rules; the first rule whose `repos` cover a commit's repo wins. An
empty list means the `git-identity` guard is a no-op.

- `git_identity[].repos` (list of string, default: empty = covers every
  repo): canonical `host/owner/repo` entries, exact or with a trailing `/*`
  org wildcard.
- `git_identity[].email` (string, required): the `git config user.email`
  a matching repo expects.
- `git_identity[].mode` (string, default `"hard"`): `"hard"` denies a
  mismatched `git commit`; `"soft"` allows it and emits a warn event instead.

### commit_guards

A list of work-hours rules; every rule is checked, and the first hard
denial wins (soft rules never deny, only warn). An empty list means the
`commit-guard` guard is a no-op. A rule whose `repos` list is empty never
matches anything (unlike `git_identity`, an empty `repos` here is not a
wildcard).

- `commit_guards[].repos` (list of string, required to match anything):
  canonical `host/owner/repo` entries, exact or with a trailing `/*` org
  wildcard.
- `commit_guards[].always_allow` (list of string, default: empty): repos
  exempt from the hours check even though they match `repos`.
- `commit_guards[].block_hours` (string `"HH:MM-HH:MM"`, required): the local
  time window. A window where the end is earlier than the start is treated as
  crossing midnight and blocks both sides of it. A malformed value fails
  open: the rule blocks nothing rather than misbehaving.
- `commit_guards[].block_days` (list of string, default: `[mon, tue, wed,
  thu, fri]`): three-letter day names, case-insensitive.
- `commit_guards[].mode` (string, default `"soft"`): `"soft"` warns via the
  events service and allows the commit; `"hard"` denies it.
- `commit_guards[].override` (string, default: empty = no override switch):
  names a timed override that suppresses the rule while active (`belt
  override set <name> --reason "..."`, 10 minutes by default, `--for` to
  size it; see Example). The name is freeform — name the exception, like `vacation` —
  and becomes the override's filename verbatim, so it must be a plain path
  segment (no `/`, no leading dot). Several rules may share one name; the
  rule field and the CLI argument connect by exact string match, and a set
  name no rule references does nothing, silently. A hard denial names its
  override in the deny reason (the safe place to copy it from). A
  suppressed block leaves a warn event on the events timeline, and the
  override expires on its own. `belt override extend <name>` pushes it
  forward when the exception outlives the window.

### custom_guards

A map keyed by guard name; each entry registers an externally-implemented
guard that belt execs with the tool-call payload as JSON on stdin. An empty
or absent `command` makes the entry a no-op.

- `custom_guards.<name>.enabled` (bool pointer, default: unset): when set,
  this wins over any `guards.<name>.enabled` toggle of the same name for this
  guard. Left unset, the guard falls back to `guards.<name>.enabled` (which
  itself defaults to enabled). The custom guard's own `enabled` is the
  intended switch since the entry is where the guard is defined.
- `custom_guards.<name>.event` (string, required): `"bash"` or `"write"`,
  which hook event the guard runs on. Validated at load — a missing or
  misspelled event (`Write`) is a config error, because the guard would
  otherwise register on an event that never fires and look healthy in
  `belt doctor`.
- `custom_guards.<name>.command` (list of string, required): the external
  program and its arguments. Runs with the invoking user's full environment.
- `custom_guards.<name>.mode` (string, default `"hard"`): `"hard"` denies on
  the external's exit code 1; `"soft"` downgrades that denial to a warn
  event and allows.
- `custom_guards.<name>.match` (string, default: empty = always runs):
  substring gate on the bash command (bash event) or target file path (write
  event); the external is only exec'd when the gate matches. Exit code 2 or
  higher, a 5-second timeout, or a failure to start the process all allow
  with a warn event, so a broken external guard never blocks work.

### custom_hints

A map keyed by hint name; each entry registers an externally-implemented
hint. belt execs the command with the hook payload fields as JSON on stdin,
and the command's stdout becomes the advice. Where `custom_guards` answers
with an exit code, a custom hint answers with text. belt trims trailing
whitespace, keeps multiple lines and any leading indentation, and emits the
result under the usual `belt[<name>]:` prefix. A non-zero exit, a run past the budget, a failure
to start, or empty stdout all end in silence plus a warn event on the
events service. A broken external never breaks a session. The entry is how
a machine-private hint (a daily journal, or a brief for worker subagents)
rides a start hook under belt's toggles and budget. It lives in that
machine's rendered config, not in belt and not in a second hook.

- `custom_hints.<name>` (the key): any name that is not a built-in hint id.
  A name that shadows one is a config error, since the toggles and `belt
  doctor` already attribute that id to the built-in.
- `custom_hints.<name>.enabled` (bool pointer, default: unset): when set,
  wins over any `hints.<name>.enabled` toggle of the same name. Left unset,
  the hint falls back to that toggle (which itself defaults to enabled).
- `custom_hints.<name>.event` (string, required): `"session-start"` (once
  per session, before its first prompt) or `"subagent-start"` (once per
  subagent, before its first prompt). A subagent-start hint's advice lands
  in the subagent's context. The field is a string so prompt and post-tool
  events can be added later without a schema break. Until then any other
  value, and a missing one, is a config error naming the supported values.
- `custom_hints.<name>.command` (list of string, required): the external
  program and its arguments. Runs with the invoking user's full
  environment; an empty list is a config error. The stdin payload is
  `{"event","cwd","session_id","transcript_path"}`, plus `"agent_id"` and
  `"agent_type"` inside a subagent (every subagent-start run).
- `custom_hints.<name>.timeout_ms` (int, default `400`): the budget for one
  run, the same budget the kof hints give kof serve. Zero means the
  default; a negative value is a config error.
- `custom_hints.<name>.exclude_repos` (list of string, default: empty):
  repos opted out of this hint, matched like every other `exclude_repos`
  against the session cwd's origin remote. A `hints.<name>.exclude_repos`
  list of the same name is honored too; either one opts out.
- `custom_hints.<name>.agent_types` (list of string, default: unset):
  limits a `subagent-start` hint to the subagent types it names, matched
  case-sensitively against the payload's `agent_type`. An entry ending in
  `*` matches every type that starts with the rest of it, and any other
  entry matches exactly. A spawn of any other type skips the hint with no
  process launch and no warn event. Unset, the hint runs for every
  subagent. The key is a config error on any other event. So is an empty
  list (`agent_types: []`, or the key with no value), a blank entry, or an
  entry with a `*` before its end. A named spawn reports its name as
  its `agent_type`, so entries for factory workers end in `*` and the
  factory names workers `factory-worker-<tier>-<slug>`. A factory worker
  brief lists `factory-worker-low*`, `factory-worker-medium*` and
  `factory-worker-high*`. A belt older than the key ignores it and runs the
  hint for every subagent, and one older than `*` matching reads the `*`
  literally and briefs no worker. So run `ralph up` before a config names
  either.

### ledger

What `belt record` posts. With no `ledger` section every lifecycle hook
wired to `belt record` posts one entry under `belt-ledger`: wiring the
hooks is the opt-in, as for hints. A config file belt cannot read still
records, with these defaults.

- `ledger.enabled` (bool pointer, default: unset, meaning on): `false`
  makes every record a no-op without touching the hook entries.
- `ledger.source` (string, default `belt-ledger`): the events-service
  source entries post under. belt rewrites it the way the events service
  stores a source. It is lowercased, every run of characters other than
  letters, digits, `-` and `_` becomes one dash, dashes are trimmed, and it
  keeps at most 64 characters. A name with nothing usable left falls back to the default.
  `belt doctor` warns when the name in effect differs from the one written.
- `ledger.agent_types` (map of agent type pattern to `{model, effort}`,
  default empty): the model and effort a subagent's agent definition
  declares. A subagent-start entry whose payload `agent_type` matches a key
  gets `model` and `effort` tags; any other type gets neither, and no other
  event reads the map. A key ending in `*` matches every type that starts
  with the rest of it, any other key matches exactly, and case counts. The
  exact key wins, then the longest matching `*` key. A named spawn reports
  its name as its `agent_type`, so keys for factory workers end in `*` and
  the factory names workers `factory-worker-<tier>-<slug>`. The map
  restates the definition, so keep it in step with the definition files;
  belt never infers a value. `model` is a string, trimmed, and must not be
  empty. `effort` must be `low`, `medium`, `high`, `xhigh` or `max`, the
  values Claude Code agent frontmatter accepts. A value that fails drops
  its own tag, an entry that is not a map or names neither key is dropped,
  and an unknown key is ignored. A key with a `*` before its end is
  dropped, and an agent type or key stated twice keeps its first value.
  Each drop is a `belt doctor` warning, and none fails the file, a wrong
  YAML type or a duplicate key included. `belt doctor` prints the keys in
  effect as written and `belt config` shows them. A belt older than the key
  ignores it and records no model or effort, so run `ralph up` before a
  machine config names it. The factory worker definitions are the first
  entries:

  ```yaml
  ledger:
    agent_types:
      factory-worker-low*: {model: sonnet, effort: low}
      factory-worker-medium*: {model: sonnet, effort: medium}
      factory-worker-high*: {model: opus, effort: high}
  ```

### watch

`belt watch`, the asyncRewake inbox watcher. It is the one belt feature that
is off until the config turns it on. A watcher holds a process per idle
session for up to its max wait, and a machine class should choose that
rather than inherit it from a hook entry.

- `watch.enabled` (bool, default `false`): with it off, `belt watch` exits 0
  at once and prints nothing.
- `watch.poll_interval_ms` (int, default `2000`): how often the watcher asks
  the events service for items. Zero or a negative value means the default;
  a value below `100` is clamped to `100`, since anything faster is a busy
  loop, and one past `3600000` (an hour) is clamped to it. `belt doctor`
  warns about each adjustment.
- `watch.max_wait_s` (int, default `540`): how long one watcher waits before
  it exits 0 and leaves the next idle to start a fresh one. Zero or a
  negative value means the default; a value past `3600` is clamped to it,
  with a doctor warning. Keep it below the hook's `timeout`, 590 in the
  documented wiring, because Claude Code kills the watcher at the timeout.
  `belt doctor` warns when an enabled watcher's value is 590 or more.
- `watch.source` (string, default `belt-inbox`): the events-service source
  the watcher polls, rewritten like `ledger.source`.

### guards

A map keyed by built-in guard id (`git-push-main`, `git-discard`, `git-identity`,
`commit-guard`, `script-deny-list`, `write-internal-names`,
`publish-internal-names`) or a
`custom_guards` name. Every guard, built-in or custom, defaults to enabled
when the file or its entry is missing. The toggle shape is shared across
guards, but which fields have an effect depends on the guard. `allow_repos`
entries match the same way as `git_identity[].repos` and
`commit_guards[].repos` above: an exact `host/owner/repo`, or a trailing
`/*` org wildcard.

- **git-push-main**
  - `guards.git-push-main.enabled` (bool, default `true`)
  - `guards.git-push-main.allow_repos` (list of string, default: empty):
    canonical `host/owner/repo` entries exempt from the deny (or a trailing
    `/*` org wildcard), matched against the push working directory's origin
    remote. An unresolved repo and an off-allowlist repo both fail closed; a
    machine class where direct pushes are fine disables the guard in its
    rendered config.
- **git-discard**
  - `guards.git-discard.enabled` (bool, default `true`)
  - `guards.git-discard.mode` (string, default `hard`): `soft` downgrades
    every denial to a warn event on the events service and lets the command
    proceed.
  - `guards.git-discard.allow_repos` (list of string, default: empty):
    canonical `host/owner/repo` entries (or a trailing `/*` org wildcard)
    where discarding commands are allowed, matched against the origin remote
    of the directory the command runs in.
- **git-identity**
  - `guards.git-identity.enabled` (bool, default `true`): the only field
    with effect. The rules themselves live in the top-level `git_identity`
    list, not under this key.
- **commit-guard**
  - `guards.commit-guard.enabled` (bool, default `true`): the only field
    with effect. The rules themselves live in the top-level `commit_guards`
    list, not under this key.
- **script-deny-list**
  - `guards.script-deny-list.enabled` (bool, default `true`)
  - `guards.script-deny-list.mode` (string, default `hard`): `soft`
    downgrades every denial to a warn event on the events service and lets
    the command proceed — the rollout setting for tuning new patterns before
    they block.
  - `guards.script-deny-list.extra_patterns` (list of string, default:
    empty): patterns denied inside scripts beyond the live Claude-settings
    deny list, e.g. `rm -rf` (the settings file only lists bare `rm` under
    "ask"). An entry starting `re:` compiles the rest as a case-insensitive
    regex, the escape hatch for flag reordering and argument wildcards the
    literal form cannot express. Anchor it yourself when word boundaries
    matter.
  - `guards.script-deny-list.exclude_paths` (list of string, default:
    empty): script locations to skip. A `~/`- or `/`-prefixed entry matches
    as a directory prefix; any other entry matches as a substring of the
    script path.
- **write-internal-names**
  - `guards.write-internal-names.enabled` (bool, default `true`)
  - `guards.write-internal-names.allow_repos` (list of string, default:
    empty): canonical `host/owner/repo` entries exempt from the guard (or a
    trailing `/*` org wildcard), matched against the write target's origin
    remote. An entry that should hold on only one machine class goes in that
    class's rendered config.
  - `guards.write-internal-names.exclude_paths` (list of string, default:
    empty): target paths where internal references are deliberate. Same
    prefix-or-substring matching as `script-deny-list.exclude_paths`.
- **publish-internal-names**
  - `guards.publish-internal-names.enabled` (bool, default `true`): one
    toggle for both of the guard's events (`bash` and `external-text`).
  - `guards.publish-internal-names.mode` (string, default `hard`): `soft`
    downgrades every denial to a warn event on the events service and lets
    the call proceed. It is the rollout setting while the derived name set
    is being tuned against real PR text and branch names.
  - `guards.publish-internal-names.allow_repos` (list of string, default:
    empty): canonical `host/owner/repo` entries (or a trailing `/*` org
    wildcard) carved out of the guard even though `public_repos` (or the
    legacy host rule) marks them public-bound. They are matched against the
    push remote, the gh `-R` target, the repo a `gh api` endpoint or `gh repo
    create` argument names, the cwd origin, and the MCP call's owner/repo.
    Needed only for a private repo inside a listed org, or under the legacy
    rule; a repo that is simply not on `public_repos` needs nothing. Per
    check on purpose (docs/adr/0013 enumerates exemptions per check), so a
    carve-out that should hold for both guards is listed under both. The
    name set itself comes from `internal_names`, shared with the write
    guard.
- **custom guard entries**
  - `guards.<name>.enabled` (bool, default `true`): a secondary toggle for a
    custom guard, used only when that guard's own
    `custom_guards.<name>.enabled` is left unset. The other toggle fields
    (`exclude_paths`, `extra_patterns`, `allow_repos`) have no effect on
    custom guards and are not
    rendered under `guards:` by `belt config` (they print under
    `custom_guards:` instead, with their full definition).

### hints

A map keyed by built-in hint id or a `custom_hints` name. All eight
built-in hints default to enabled when the file or their entry is missing. The repo-aware hints (`commit-policy`,
`lint-policy`, `prefer-csl`, `kof-assertions`, `kof-consult`) also read
`exclude_repos`. The rest take only `enabled`. Any other key on a known id
is a validation error (guards exempt repos with `allow_repos`, hints opt
them out with `exclude_repos`, so each key on the wrong kind is rejected).
Exclusion matches the canonical `host/owner/repo` identity resolved from
the repo's origin remote everywhere a working tree is reachable. Only
`kof-assertions`, which sees index-side results with no path, matches the
pattern tail (host dropped, case-insensitive).

- `hints.agent-memory.enabled` (bool, default `true`): SessionStart, injects
  the agent memory index files.
- `hints.prefer-csl.enabled` (bool, default `true`): PostToolUse (bash),
  suggests `csl_search` in place of a filesystem sweep inside an indexed
  repo. `exclude_repos` silences it for the listed repos, matched against
  the swept repo's origin remote — not its checkout path, so a worktree
  under a different parent directory is still covered.
- **commit-policy**
  - `hints.commit-policy.enabled` (bool, default `true`): PostToolUse
    (bash), states the branch + PR commit policy after a `git commit` lands
    on main or master.
  - `hints.commit-policy.exclude_repos` (list of string, default: empty):
    repos opted out of this hint only. A repo whose main is committed to
    directly by design belongs on the top-level `direct_main_repos` list
    instead, which silences this hint and exempts git-push-main in one
    entry.
  - Repo-local overlay: a `.belt.yaml` at the commit repo's root can carry
    `hints.commit-policy` with `exclude` (bool, overriding the machine
    lists in either direction). It can also carry `protected_branches`
    (replacing the default `main`/`master` set; exact names or trailing-`*`
    prefixes). Note a replacement set that matches nothing also silences
    the hint, so `exclude: false` is an opt-in only for the branches the
    set names. `message` is one line appended to the advice. The overlay is
    hints-only and can never deny — a broken file degrades to one line of
    advisory text, unknown ids and keys are ignored (docs/adr/0012). It is
    not part of this machine config file.
- **lint-policy**
  - `hints.lint-policy.enabled` (bool, default `true`): PostToolUse (bash),
    relays the commit repo's declared lint/format policy after a `git
    commit`, once per session per repo. A trigger command that itself names
    the fmt/lint toolchain outside quotes is skipped without spending the
    session's nudge.
  - `hints.lint-policy.exclude_repos` (list of string, default: empty):
    repos opted out of this hint.
  - Repo-local overlay: the hint speaks only for repos whose root
    `.belt.yaml` carries `hints.lint-policy.message`. That message is the
    whole advice (the repo names its own fmt/lint commands; belt ships no
    language table and executes nothing). `exclude` opts the repo out or
    back in over the machine list (docs/adr/0012).
- `hints.kof-assertions.enabled` (bool, default `true`): PostToolUse
  (search), surfaces kof assertions about code a search just hit.
  `exclude_repos` silences it for the listed repos (pattern tail matched —
  the search results carry no path to resolve a host from).
- `hints.kof-consult.enabled` (bool, default `true`): SessionStart, surfaces
  the cwd repo's kof assertions before any searching happens.
  `exclude_repos` silences it for the listed repos, matched against the cwd
  repo's origin remote.
- `hints.kof-deposit.enabled` (bool, default `true`): UserPromptSubmit,
  nudges a session that did substantial work but never deposited a kof
  assertion.
- `hints.humanizer-check.enabled` (bool, default `true`): fires after an MCP
  call publishes text off the machine, pointing at `humanizer_detect` while
  the wording is still editable.
- `hints.em-dash.enabled` (bool, default `true`): one toggle for both of
  the hint's events (`external-text` and `bash`). Flags em dashes in text a
  publishing MCP call or gh command just sent.
- **custom hint entries**
  - `hints.<name>.enabled` (bool, default `true`): a secondary toggle for a
    custom hint, used only when that hint's own
    `custom_hints.<name>.enabled` is left unset.
  - `hints.<name>.exclude_repos` (list of string, default: empty): opt-outs
    added to the entry's own `exclude_repos`. The other toggle fields have
    no effect on custom hints. `belt config` renders custom hints under
    `custom_hints:` with their full definition, not under `hints:`.

## Environment variables

None of these is a `config.yaml` key; all are read directly from the process
environment at hook invocation time.

- `BELT_CONFIG` (default: unset): path to the belt config file, replacing
  `~/.config/belt/config.yaml`. `--config` wins over it. A relocated file
  has no legacy TOML fallback.
- `XDG_CONFIG_HOME` (default: unset, meaning `~/.config`): when it holds an
  absolute path, belt's config directory is `$XDG_CONFIG_HOME/belt` — config
  file and overrides both. A relative value is ignored, per the XDG spec.
- `EVENTS_BASE_URL` (default `http://127.0.0.1:7430`): base URL of the local
  events service that guard denials, hint fires, soft-mode/fail-open
  warnings and ledger entries POST to, and that `belt watch` polls. A POST
  failure (service down) is silently dropped.
- `KOF_PORT` (default `7431`; the legacy `KEEP_PORT` is honored as a
  fallback when `KOF_PORT` is unset): port of the local kof serve instance
  the `kof-assertions` and `kof-consult` hints, and the `belt doctor`
  reachability probe, query.

## Example

```yaml
# ~/.config/belt/config.yaml — every key optional; guards and hints default
# to enabled when the file or their entry is missing.

direct_main_repos:
  - github.com/you/dotfiles

public_repos:
  - github.com/you/your-open-source-tool
  - github.com/you/your-public-site

internal_names:
  include:
    - ~/.config/internal-names/common.yaml
  workspace_dirs:
    - ~/workspace
  blocked_words:
    - internal-brand
  allowlist:
    - some-safe-name

claude_settings:
  enabled: true

git_identity:
  - repos:
      - github.com/you/*
    email: you@personal.example
    mode: hard

commit_guards:
  - repos:
      - github.com/you/*
    always_allow:
      - github.com/you/essential-tooling
    block_hours: "09:00-17:00"
    block_days: [mon, tue, wed, thu, fri]
    mode: soft
    override: vacation

custom_guards:
  check-branch-naming:
    enabled: true
    event: bash
    command: [branch-lint, check]
    match: git commit
    mode: hard

custom_hints:
  daily-journal:
    enabled: true
    event: session-start
    command: [journal, open-actions, --since, yesterday]
    timeout_ms: 400
    exclude_repos:
      - github.com/you/scratch
  worker-brief:
    event: subagent-start
    command: [factory, brief]
    agent_types: [factory-worker-low*, factory-worker-medium*, factory-worker-high*]

ledger:
  enabled: true
  source: belt-ledger
  agent_types:
    factory-worker-low*: {model: sonnet, effort: low}
    factory-worker-medium*: {model: sonnet, effort: medium}
    factory-worker-high*: {model: opus, effort: high}

watch:
  enabled: true
  poll_interval_ms: 2000
  max_wait_s: 540
  source: belt-inbox

guards:
  git-push-main:
    enabled: true
    # direct_main_repos above already exempts the repos whose workflow is
    # direct-to-main; allow_repos is the escape hatch for a one-off that is
    # not a workflow fact.
    # allow_repos:
    #   - github.com/you/one-off-exception

  script-deny-list:
    enabled: true
    mode: hard
    extra_patterns:
      - rm -rf
      - "re:git\\s+push\\s.*--force"
    exclude_paths:
      - ~/trusted/scripts

  write-internal-names:
    enabled: true
    allow_repos:
      - github.com/you/private-companion
    exclude_paths:
      - ~/notes

  publish-internal-names:
    enabled: true
    allow_repos:
      - github.com/you/private-companion

hints:
  agent-memory:
    enabled: true
  commit-policy:
    enabled: true
  lint-policy:
    enabled: true
  prefer-csl:
    enabled: true
  kof-assertions:
    enabled: true
  kof-consult:
    enabled: true
  kof-deposit:
    enabled: true
  humanizer-check:
    enabled: true
  em-dash:
    enabled: true
```

Activating the `vacation` override above so the `commit_guards` rule stops
applying. An override is timed: 10 minutes by default, sized with `--for`,
extendable, and self-expiring. The switch is one file under
`~/.config/belt/overrides/` carrying its RFC 3339 expiry. An empty file
from an older belt counts as untimed and stays active until cleared.
Re-set it with `--for` to make it expire.

```bash
belt override set vacation --for 2h --reason "half-day off"  # active for two hours
belt override extend vacation --reason "still off"           # push the expiry 10 more minutes
belt override                         # list every override with its state
belt override clear vacation          # end it early
```
