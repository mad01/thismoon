# belt configuration

## Where config lives

belt's own settings live in `~/.config/belt/config.yaml` (YAML). A legacy
`~/.config/belt/config.toml` beside it is read only when the YAML file does
not exist at all; if the YAML file exists but fails to parse, belt does not
fall back to the TOML file. A present-but-broken config of either format
yields the built-in defaults: every guard and hint enabled with no rules
configured. This is deliberate fail-closed behavior, not a bug: a hook must
never run on a config it half-understood.

Two settings fall back to the tool that originated them when the belt config
does not set them:

- `profiles`: the belt config's `profiles` list when it is non-empty, else
  the `profiles` list from `~/.config/ralph/config.local.toml` (TOML). If
  both are empty, profile-gated guards fail closed: `git-push-main` denies
  every push to main or master, because no machine carries a "personal"
  profile to exempt it.
- `internal_names`: the belt config's `internal_names` section when it is
  present at all (even with empty inner lists, presence is what decides it),
  else the `guard:` section of `~/.config/suspenders/config.yaml` (YAML). The
  two sources are never merged field by field. Whichever file supplies the
  section supplies all of it.

One more surface is always read live, never cached and never copied into the
belt config: the Bash deny patterns behind `script-deny-list` come from the
`permissions.deny` entries of `~/.claude/settings.json` and
`~/.claude/settings.local.json` on every invocation, so the guard and the
Claude Code permission system can never drift apart. `belt config` shows
these as `claude_deny` in its output, and `belt config --help` prints the
full annotated key reference reproduced in the Example section below.

Run `belt config` to see which files loaded (or failed to) and every setting
belt is actually running with, after every default and fallback is applied.
Run `belt doctor` to see the state those settings produce: enabled guards and
hints, active overrides, and the resolved blocked-name list.

## Keys

- `profiles` (list of string, default: falls back to ralph, then empty):
  machine profiles for profile-gated rules. `git-push-main` checks for
  `personal` directly; `git_identity` and `commit_guards` rules can gate on
  an arbitrary profile name via their own `profile` field.

### internal_names

The name-derivation source for the `write-internal-names` guard. Belt-owned
when this section is present at all; otherwise it falls back whole to the
suspenders `guard:` section (see Where config lives).

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

### git_identity

A list of rules; the first rule whose `profile` and `repos` cover a commit's
repo wins. An empty list means the `git-identity` guard is a no-op.

- `git_identity[].repos` (list of string, default: empty = covers every
  repo): canonical `host/owner/repo` entries, exact or with a trailing `/*`
  org wildcard.
- `git_identity[].profile` (string, default: empty = applies on every
  machine): restrict the rule to machines carrying this ralph profile.
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
- `commit_guards[].profile` (string, default: empty = applies on every
  machine): restrict the rule to machines carrying this ralph profile.
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
  which hook event the guard runs on.
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
guards, but which fields have an effect depends on the guard. Note that
`allow_repos` and `allow_repos_by_profile` entries here match by exact
`host/owner/repo` string only: unlike `git_identity[].repos` and
`commit_guards[].repos` above, a trailing `/*` is not treated as an org
wildcard.

- **git-push-main**
  - `guards.git-push-main.enabled` (bool, default `true`)
  - `guards.git-push-main.allow_repos` (list of string, default: empty):
    canonical `host/owner/repo` entries exempt from the deny, matched
    against the push working directory's origin remote. Unlike
    `write-internal-names` below, this guard does not read
    `allow_repos_by_profile`. An unresolved repo, an unknown profile, and an
    off-allowlist repo all fail closed.
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
    empty): canonical `host/owner/repo` entries exempt from the guard,
    matched against the write target's origin remote.
  - `guards.write-internal-names.allow_repos_by_profile` (map of profile
    name to list of string, default: empty): like `allow_repos`, but each
    entry applies only on machines carrying the named ralph profile, so one
    fleet-shared config can allow a repo on personal machines while work
    machines stay fail-closed.
  - `guards.write-internal-names.exclude_paths` (list of string, default:
    empty): target paths where internal references are deliberate. Same
    prefix-or-substring matching as `script-deny-list.exclude_paths`.
- **custom guard entries**
  - `guards.<name>.enabled` (bool, default `true`): a secondary toggle for a
    custom guard, used only when that guard's own
    `custom_guards.<name>.enabled` is left unset. The other toggle fields
    (`exclude_paths`, `extra_patterns`, `allow_repos`,
    `allow_repos_by_profile`) have no effect on custom guards and are not
    rendered under `guards:` by `belt config` (they print under
    `custom_guards:` instead, with their full definition).

### hints

A map keyed by hint id. All six hints default to enabled when the file or
their entry is missing, and `enabled` is the only field with effect: hints
have no `exclude_paths`, `extra_patterns`, or `allow_repos` behavior even
though the config shape technically permits them.

- `hints.agent-memory.enabled` (bool, default `true`): SessionStart, injects
  the agent memory index files.
- `hints.prefer-csl.enabled` (bool, default `true`): PostToolUse (bash),
  suggests `csl_search` in place of a filesystem sweep inside an indexed
  repo.
- `hints.kof-assertions.enabled` (bool, default `true`): PostToolUse
  (search), surfaces kof assertions about code a search just hit.
- `hints.kof-consult.enabled` (bool, default `true`): SessionStart, surfaces
  the cwd repo's kof assertions before any searching happens.
- `hints.kof-deposit.enabled` (bool, default `true`): UserPromptSubmit,
  nudges a session that did substantial work but never deposited a kof
  assertion.
- `hints.humanizer-check.enabled` (bool, default `true`): fires after an MCP
  call publishes text off the machine, pointing at `humanizer_detect` while
  the wording is still editable.

## Environment variables

Neither variable is a `config.yaml` key; both are read directly from the
process environment at hook invocation time.

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

profiles:
  - personal

internal_names:
  workspace_dirs:
    - ~/workspace
  blocked_words:
    - internal-brand
  allowlist:
    - some-safe-name

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
    profile: work
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
    allow_repos_by_profile:
      personal:
        - github.com/you/personal-store
    exclude_paths:
      - ~/notes

hints:
  agent-memory:
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
