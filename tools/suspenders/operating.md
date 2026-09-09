# operating suspenders

suspenders is an offline pre-commit gate for git. It scans staged content for
secrets (content rules with entropy gating, plus filename rules for key and
token files) and, when the guard is enabled, blocks staged changes that
reference internal org or repo names from a public repo. It is CLI-only: no
service, no web page, and no MCP surface. The only network call is a
best-effort event to the local events service when a commit is blocked.

## how it runs

`{{.Bin}} hook install` writes the repo's pre-commit hook (resolved via `git
rev-parse --git-path hooks`, so core.hooksPath and worktrees are honored), a
thin script that calls `{{.Bin}} hook run pre-commit`. A pre-existing foreign
hook is backed up as `pre-commit.backup` and chained first. The pipeline per
commit: external hooks from config, then the guard, then the scan; any failing
step blocks the commit. Staged content is read from the git index (`git show
:<path>`), so the scan sees exactly what would be committed, and a staged file
it cannot read fails the scan instead of being skipped.

`{{.Bin}} scan --staged` runs the same checks outside git; `{{.Bin}} scan
[path]` checks every tracked file in a working tree. A blocked commit exits 1;
any other failure exits 2.

The guard's blocked-name list is derived fresh on every run from the repos
under `guard.workspace_dirs` plus `blocked_words`, including the names files
under `guard.include`; it is never persisted. Matching is case-insensitive
and by name only, after `allow_phrases` are blanked out. Repos inside a
workspace dir, or matching a top-level `exclude` glob, are guard-exempt.

## where config lives

Global config: `~/.config/suspenders/config.yaml` (respects XDG_CONFIG_HOME;
`--config` and `$SUSPENDERS_CONFIG` relocate it). It is optional and nothing
creates it implicitly — with no file, the built-in defaults apply in memory,
and `{{.Bin}} config init` writes one when you want a file to edit.
`{{.Bin}} config` prints the path and the settings in effect. Per-repo
overrides live in `.suspenders.yaml` at the repo root: ignore rules, paths,
patterns, allowlist entries, and a `guard` section whose entries append to
the global guard config.

## failure modes

Commit blocked: read the finding. A scan finding names the rule and the
file:line with the match redacted; a guard block lists the blocked names.
Usually a true positive: remove the secret or the reference and commit
again — that is the fix, not an override.

Wrong block: take the lowest rung that solves it. One commit —
`git commit --no-verify` skips the hook entirely; it is the explicit,
auditable override, preferred over silently weakening the scan for
everyone. One line — a `suspenders:ignore` marker on the flagged line. One
value — the exact string in `allowlist` (scan finding) or `guard.allowlist`
(guard hit), global or per-repo. One repo — `.suspenders.yaml` at the repo
root suppresses a rule id, path glob, or match substring; per-repo
`allowlist` and `guard` entries append to the global config rather than
replacing it. One check — `scan.enabled: false` or `guard.enabled: false`
turns it off entirely. There is no environment-variable override, and the
guard has no path-based exclusion.

Hook not firing: hooks are per-clone, so run `{{.Bin}} hook install` after
every clone. `{{.Bin}} hook status` reports installed, outdated,
not-installed, foreign, or error; `{{.Bin}} hook update` refreshes an
outdated script.

Config broken or missing: a config file that fails to parse, or a names file
listed under `guard.include` that is missing or broken, fails every command
that reads it, including the hook run: a commit is blocked rather than
checked against nothing. No config file at all is fine; the defaults apply.
`--all` reporting `config sets no dirs` means the file lists `dirs: []`, so
there is nothing to walk.

Machines disagree: two causes. The guard derives blocked names from what is
checked out locally, so a machine missing a checkout will not block that
repo's name. And version skew between the installed binaries.

## version skew

`{{.Bin}} version` prints a bare version token; `{{.Bin}} version -o json`
prints the four-key build metadata object (version, commit, tag, build_time).
There is no long-running service, so the binary on PATH is the only build in
play; `{{.Bin}} doctor` opens with the build answering.

## first moves

1. `{{.Bin}} scan --staged` in the blocked repo, to reproduce the finding outside git
2. `{{.Bin}} doctor`, for the build, config in effect, exemption status, and the derived blocked names
3. Fix the flagged content, or add the narrowest override above
4. `{{.Bin}} hook status`, then `{{.Bin}} hook install` if the hook is missing
5. `{{.Bin}} version -o json` on each machine when they disagree
