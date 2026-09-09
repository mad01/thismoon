# suspenders configuration

## Where config lives

- Global config: `~/.config/suspenders/config.yaml`, or `$XDG_CONFIG_HOME/suspenders/config.yaml` when that variable holds an absolute path. YAML format. `--config <path>` and `$SUSPENDERS_CONFIG` relocate the file; the flag wins over the variable, and a leading `~` expands.
- The config file is optional and **suspenders never creates it**. With no file, the defaults documented under Keys are what suspenders runs with, held in memory. Run `suspenders config init` to write a file with those defaults when you want one to edit; it refuses to overwrite an existing file. Loading used to write this file on first run, which under a git pre-commit hook meant creating one wherever git had left the process — and persisting one machine's directory layout onto every other machine that ran the tool.
- A config file that exists but cannot be read or parsed is an error, never a silent fallback to the defaults: `hook run`, `scan`, `doctor`, and the `hook install/update/status` commands all fail on it, so a broken file blocks the commit instead of checking it against nothing. A names file listed under `guard.include` that is missing or does not parse is the same error. `suspenders config` is the exception: it reports the parse error in its header and prints the defaults anyway, so the reference stays readable while the file is broken.
- An unresolvable home directory (no `$HOME`, no `$XDG_CONFIG_HOME`) is also an error. The old behavior resolved `./.config/suspenders/config.yaml` relative to the working directory, which inside a pre-commit hook is the repository being committed to.
- Per-repo overrides: `.suspenders.yaml` (or `.suspenders.yml`) at a repository's root. Resolved from the scan root or the enclosing git top-level. Both `scan` and the pre-commit hook read it.
- Precedence: per-repo `allowlist`, `watch`, `guard.allowlist`, and `guard.blocked_words` entries are appended to the corresponding global lists. Per-repo `rules`, `paths`, and `patterns` are repo-only ignore mechanisms with no global equivalent. Nothing in the per-repo file replaces a global value outright.
- `suspenders config` prints the config file path and the settings suspenders resolves to after defaults are applied, not just what the file spells out.

## Keys

- `dirs` (string list, default `["~/code/src", "~/workspace"]`): directories `hook install --all` and other `--all` commands walk to discover git repositories. A leading `~` expands to the home directory. The default applies when the key is absent; an explicit `dirs: []` means "discover nothing", and the `--all` commands then fail with `config sets no dirs` rather than walking the defaults or reporting success over zero repos.
- `exclude` (string list, default empty): repo name globs (`org/repo`) skipped by `--all` discovery and exempt from the guard when committing inside them. A matching repo's name still contributes to the blocked name list for other repos, deliberately.
- `watch` (list of rule objects, default empty): custom secret-detection rules layered on top of the built-in rules.
  - `id` (string): rule identifier.
  - `description` (string): shown in finding output.
  - `pattern` (string): regular expression to match.
  - `severity` (string): `high`, `medium`, or `low`.
- `allowlist` (list of entries, default empty): exact-match secret values known to be safe, suppressed globally regardless of which rule matched them.
  - `match` (string): the exact value to allow.
  - `description` (string, optional): why it's safe.
  - `paths` (string list, optional): restrict the allowance to files matching these globs; omitted means every path.

### Scan

- `scan.enabled` (bool, default `true`): toggles the built-in secret scanner. The field left out of the file resolves to enabled, kept for backwards compatibility; the guard and external hooks still run when this is `false`.
- `scan.exclude_rules` (string list, default empty): rule IDs removed globally, in every repo. This is rule exclusion, distinct from a per-repo rule ignore in `.suspenders.yaml`.

### Guard

The internal-reference guard blocks commits that mention internal org or repo names, or any additional blocked word, in a public repo.

- `guard.enabled` (bool, default `false`): turns the guard on. Off by default because it needs `workspace_dirs` pointed at real checkouts to be useful.
- `guard.workspace_dirs` (string list, default empty): directories walked to discover repos. Every discovered repo's org name, repo name, and checkout directory basename become blocked names. Repos inside these directories are themselves guard-exempt.
- `guard.blocked_words` (string list, default empty): terms added to the blocked name list regardless of workspace discovery, such as a brand name or an internal domain. Literal, case-insensitive match; `*` matches a run of non-space characters (`*.example.net`).
- `guard.allowlist` (string list, default empty): safe references, names that must never be treated as blocked even though discovery or `blocked_words` would otherwise collect them (for example `grpc/grpc-go`). This is a separate list from the top-level `allowlist` above, which holds secret values, not names.
- `guard.allow_phrases` (string list, default empty): exact phrases, matched case-insensitively, blanked out of the checked content before name matching. A sanctioned compound that contains a blocked name (a private companion repo called `dotfiles-<name>`, say) passes, while the bare name anywhere else still blocks. Honored by every check: staged diff, working-tree scan, history scan and clean.
- `guard.include` (string list, default empty): paths of shared names files, `~` allowed. A names file holds exactly `blocked_words`, `allowlist`, and `allow_phrases`; each listed file's lists are appended to the three lists above at load time, in include order, and per-repo overrides layer on top of the merged result. belt lists the same files under its `internal_names.include`, so both guards read one source (docs/adr/0016). A listed file that is missing, carries any other key, or does not parse fails the load like a broken config.
- `guard.file_patterns` (string list, default empty, meaning every file): globs restricting which staged files the guard inspects for blocked names.

### History

Settings for `history clean`.

- `history.replace_table` (map of string to string, default empty): old-to-new string mappings used when rewriting history; entries are applied longest-key-first to avoid partial matches.
- `history.redact_files` (string list, default empty): path globs whose blob content is replaced wholesale with a redaction placeholder in every commit, while the path itself stays in the tree. Bare names match at any directory depth.

### Hooks

External hook scripts run as part of a hook run, before the built-in guard and scan.

- `hooks.pre_commit` (list of hook entries, default empty): run in config order during the pre-commit hook run; the first failure stops the chain.
- `hooks.post_merge` (list of hook entries, default empty): run in config order during the post-merge hook run.

Each hook entry:

- `name` (string, required): identifier shown in logs and error messages.
- `command` (string, required): shell command executed via `sh -c`.
- `file_patterns` (string list, default empty, meaning every file): only run the hook if staged or changed files match one of these globs.
- `repos` (string list, default empty, meaning every repo): only run the hook in repos whose `org/repo` name matches one of these globs.
- `enabled` (bool, default `true`): omit to leave the hook enabled; set `false` to disable it without removing the entry.

### Per-repo overrides (`.suspenders.yaml`)

Layered over the global config at a repository's root. Ignore rules, paths, and patterns are repo-only; allowlist, watch, and guard entries are appended to the corresponding global lists.

- `rules` (string list, default empty): rule IDs to ignore in this repo; the rule still runs everywhere else.
- `paths` (string list, default empty): file path globs whose findings are ignored in this repo.
- `patterns` (string list, default empty): literal substrings that, when present in a finding's match or context, are ignored.
- `allowlist` (list of entries, default empty): same shape as the global `allowlist`, appended to it for this repo.
- `watch` (list of rule objects, default empty): same shape as the global `watch`, appended to it for this repo.
- `guard.allowlist` (string list, default empty): safe references appended to the global guard allowlist for this repo.
- `guard.blocked_words` (string list, default empty): additional blocked names for this repo, appended to the global list.

## Environment variables

- `SUSPENDERS_CONFIG`: path to the config file, replacing the default location. `--config` wins over it.
- `XDG_CONFIG_HOME`: base directory for the config file. When unset or relative, suspenders falls back to `~/.config`, so the config path is `$XDG_CONFIG_HOME/suspenders/config.yaml` or `~/.config/suspenders/config.yaml`.
- `EVENTS_BASE_URL`: base URL for the local events service that a blocked commit posts a best-effort event to (default `http://127.0.0.1:7430`). Never read for anything else; the guard and scan have no environment-variable overrides.

## Example

```yaml
dirs:
  - ~/code/src
  - ~/workspace

exclude:
  - "*/vendor/*"

scan:
  enabled: true
  exclude_rules:
    - generic-high-entropy

guard:
  enabled: true
  workspace_dirs:
    - ~/workspace
  blocked_words:
    - examplecorp
    - "*.examplecorp.net"
  allowlist:
    - grpc/grpc-go
  allow_phrases:
    - dotfiles-examplecorp
  include:
    - ~/.config/internal-names/common.yaml
  file_patterns:
    - "*.go"
    - "*.md"
    - "*.yaml"

watch:
  - id: internal-api-token
    description: Internal API token
    pattern: 'INTERNAL_TOKEN\s*=\s*(.+)'
    severity: high

allowlist:
  - match: AKIA<the-rest-of-a-known-fixture-key>
    description: AWS example key from documentation (spell out the real
      literal here; a placeholder is shown so this doc passes the scanner)
  - match: sk_test_1234567890
    paths: ["**/Makefile"]

history:
  replace_table:
    old.internal.net: new.example.com
  redact_files:
    - id_rsa
    - "certs/*.p12"

hooks:
  pre_commit:
    - name: go-vet
      command: go vet ./...
      file_patterns: ["*.go"]
      repos: ["mad01/ralph"]
  post_merge:
    - name: csl-reindex
      command: |
        mkdir -p "${HOME}/.config/csl" &&
        printf '%s\n' "$(git rev-parse --show-toplevel)" >> "${HOME}/.config/csl/reindex.queue"
```

Per-repo `.suspenders.yaml`:

```yaml
rules:
  - jwt-token

paths:
  - "**/*_test.go"

patterns:
  - EXAMPLE

allowlist:
  - match: test-dummy-key

guard:
  allowlist:
    - monitoring
  blocked_words:
    - project-x
```
