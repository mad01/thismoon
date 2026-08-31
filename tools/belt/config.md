# belt configuration

## Where config lives

belt's own settings live in `~/.config/belt/config.yaml` (YAML), or
`$XDG_CONFIG_HOME/belt/config.yaml` when that variable holds an absolute
path. `--config <path>` and `$BELT_CONFIG` relocate the file; the flag wins
over the variable, and a leading `~` is expanded. A relocated file is read as
YAML with no legacy fallback.

A legacy `~/.config/belt/config.toml` beside the default YAML file is read
only when the YAML file does not exist at all; if the YAML file exists but
fails to parse, belt does not fall back to the TOML file.

**Absent and invalid are different answers.** No config file means the
built-in defaults: every guard and hint enabled, no rules configured. A
config file that is present but fails to parse or validate is an error, and
every `belt hook` invocation then denies with the reason
`belt[config]: belt cannot read its config …` naming the file. Belt is a
guard: it cannot tell "no rules configured" from "the rules did not load",
and treating the second as the first is how one typo silently disarms
`write-internal-names` and every commit rule at once. The way out is to fix
the file — or move it aside, which is the explicit way to ask for the
defaults. `belt doctor` and `belt config` keep working in that state and
print the built-in defaults, with a line saying those defaults are not what
is being enforced.

Validation rejects the values that parse as YAML and then quietly do
nothing:

- a `custom_guards.<name>.event` that is not `bash` or `write` (including a
  missing one) — the guard would register on an event that never fires;
- a `mode:` anywhere other than `hard` or `soft`;
- `mode: soft` under `guards:` or `hints:` on any id except
  `script-deny-list`, the only one that reads a toggle mode. Use
  `enabled: false` to switch a different guard off;
- any list key on a built-in id that does not read it — each id declares
  the toggle fields it reads (the `GuardFields`/`HintFields` tables in the
  config package), and the error names the fields that do work there.
  Guards exempt repos with `allow_repos`, hints opt them out with
  `exclude_repos`, so each key on the other kind is always rejected. Ids
  the binary does not know (custom guards, or a config targeting a newer
  belt) stay lax on purpose: rejecting unknown keys would turn ordinary
  config/binary rollout skew into a machine-wide deny.

Overrides stay in `~/.config/belt/overrides/` (or the `XDG_CONFIG_HOME`
equivalent) whatever `--config` points at: an override is machine state set
from the CLI, not part of a rendered config. Files beginning with a dot in
that directory are ignored rather than read as malformed overrides.

The belt config is standalone: belt never reads another tool's config file,
and there is no machine-profile concept in it (docs/adr/0010 records the
decision). The provisioning layer renders one config per machine class, so
a guard or rule that should exist only on some machines is simply absent
(or disabled) in the other classes' files — `git-push-main`, for example,
ships enabled and denies every push to main until a machine's rendered
config disables it or allowlists a repo.

Exactly one non-belt surface is read, visible in `belt doctor`: the Claude
Code settings. The Bash deny patterns behind `script-deny-list` come from
the `permissions.deny` entries of `~/.claude/settings.json` and
`~/.claude/settings.local.json` on every invocation — read live, never
cached and never copied into the belt config, so the guard and the Claude
Code permission system can never drift apart. This read has its own switch,
`claude_settings.enabled` (see Keys); `belt config` shows the patterns as
`claude_deny` in its output, and `belt config --help` prints the full
annotated key reference reproduced in the Example section below.

`internal_names` has no fallback: an absent or empty section means an empty
name set, and `write-internal-names` then has nothing to match (`belt
doctor` calls this out). The section is deliberately shape-compatible with
the `guard:` section of the suspenders config so a provisioning layer can
render one authored block into both files — that is where the two tools'
name configs stay in step, not at read time.

Run `belt config` to see which files loaded (or failed to) and every setting
belt is actually running with, after every default and fallback is applied.
Run `belt doctor` to see the state those settings produce: enabled guards and
hints, active overrides, and the resolved blocked-name list.

## Keys

### direct_main_repos (top-level)

The one shared repo list, named for the workflow fact it states: repos
whose workflow is direct-to-main (single-writer store clones, config repos
that never take PRs). Patterns are canonical `host/owner/repo` or a
trailing `/*` org wildcard. Exactly two checks read it — the two whose
subject is that workflow: `git-push-main` (the push is allowed there) and
the `commit-policy` hint (the branch + PR advice is silenced there). No
other check consults it, so editing this list can never disarm an
unrelated guard; a repo that needs an exemption from anything else uses
that check's own `allow_repos`/`exclude_repos` (docs/adr/0013 records the
reversibility criterion behind this).

### internal_names

The name-derivation source for the `write-internal-names` guard. Belt-owned
and standalone: absent or empty means an empty name set (see Where config
lives).

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
  matching. Use it when a sanctioned compound contains a blocked name — a
  private companion repo's own name, say — so writing the compound passes
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
  override in the deny reason (the safe place to copy it from), a
  suppressed block leaves a warn event on the events timeline, and the
  override expires on its own — `belt override extend <name>` pushes it
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

### guards

A map keyed by built-in guard id (`git-push-main`, `git-identity`,
`commit-guard`, `script-deny-list`, `write-internal-names`) or a
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
    regex — the escape hatch for flag reordering and argument wildcards the
    literal form cannot express; anchor it yourself when word boundaries
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
- **custom guard entries**
  - `guards.<name>.enabled` (bool, default `true`): a secondary toggle for a
    custom guard, used only when that guard's own
    `custom_guards.<name>.enabled` is left unset. The other toggle fields
    (`exclude_paths`, `extra_patterns`, `allow_repos`) have no effect on
    custom guards and are not
    rendered under `guards:` by `belt config` (they print under
    `custom_guards:` instead, with their full definition).

### hints

A map keyed by hint id. All eight hints default to enabled when the file or
their entry is missing. The repo-aware hints (`commit-policy`,
`lint-policy`, `prefer-csl`, `kof-assertions`, `kof-consult`) also read
`exclude_repos`;
the rest take only `enabled`, and any other key on a known id is a
validation error (guards exempt repos with `allow_repos`, hints opt them
out with `exclude_repos` — each key on the wrong kind is rejected).
Exclusion matches the canonical `host/owner/repo` identity resolved from
the repo's origin remote everywhere a working tree is reachable; only
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
    lists in either direction), `protected_branches` (replacing the default
    `main`/`master` set; exact names or trailing-`*` prefixes — note a
    replacement set that matches nothing also silences the hint, so
    `exclude: false` is an opt-in only for the branches the set names), and
    `message` (one line appended to the advice). The overlay is hints-only
    and can never deny — a broken file degrades to one line of advisory
    text, unknown ids and keys are ignored (docs/adr/0012). It is not part
    of this machine config file.
- **lint-policy**
  - `hints.lint-policy.enabled` (bool, default `true`): PostToolUse (bash),
    relays the commit repo's declared lint/format policy after a `git
    commit`, once per session per repo. A trigger command that itself names
    the fmt/lint toolchain outside quotes is skipped without spending the
    session's nudge.
  - `hints.lint-policy.exclude_repos` (list of string, default: empty):
    repos opted out of this hint.
  - Repo-local overlay: the hint speaks only for repos whose root
    `.belt.yaml` carries `hints.lint-policy.message` — that message is the
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
  events service that guard denials and soft-mode/fail-open warnings POST
  to. A POST failure (service down) is silently dropped.
- `KOF_PORT` (default `7431`; the legacy `KEEP_PORT` is honored as a
  fallback when `KOF_PORT` is unset): port of the local kof serve instance
  the `kof-assertions` and `kof-consult` hints, and the `belt doctor`
  reachability probe, query.

## Example

```yaml
# ~/.config/belt/config.yaml — every key optional; guards and hints default
# to enabled when the file or their entry is missing.

internal_names:
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

guards:
  git-push-main:
    enabled: true
    allow_repos:
      - github.com/you/yourrepo

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
```

Activating the `vacation` override above so the `commit_guards` rule stops
applying. An override is timed: 10 minutes by default, sized with `--for`,
extendable, and self-expiring. The switch is one file under
`~/.config/belt/overrides/` carrying its RFC 3339 expiry (an empty file
from an older belt counts as untimed and stays active until cleared —
re-set it with `--for` to make it expire).

```bash
belt override set vacation --for 2h --reason "half-day off"  # active for two hours
belt override extend vacation --reason "still off"           # push the expiry 10 more minutes
belt override                         # list every override with its state
belt override clear vacation          # end it early
```
