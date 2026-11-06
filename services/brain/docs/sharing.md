# brain git vaults and sharing

`brain docs sharing` prints this page. `brain docs` prints the CLI guide and
`brain docs lifecycle` the note statuses. brain runs `git` for every vault
that is a git repo and `gh` (the GitHub CLI, logged in with `gh auth login`)
for the pull requests and issues of a shared vault.

## Git per vault

When a vault's root is the top level of a git repo, brain commits every
change it makes there at once. That covers write, capture, edit, promote,
supersede, retract, withdraw, flag, `inbox keep`, `inbox archive`,
`migrate --ids`, and the removals and returns `brain sync` makes.

- The message is `brain: <verb> <path>`, or `brain: <verb> <path> and N
  more` when one change touches several notes (a promote that rewrites
  backlinks, an archive).
- brain stages and commits only the paths it changed (`git add -- <paths>`,
  then `git commit -- <paths>`), never `-A`. Your own staged, unstaged and
  untracked edits stay as they were.
- brain never pushes on a commit; `brain sync` pushes.
- A failed commit leaves the change on disk and fails the command (exit 1)
  with a message saying so.
- `autocommit: false` on the vault's config entry turns the commits off. A
  vault that is not a git repo, or sits inside a larger repo, works as
  before, with no commits.

A private vault with no remote is a supported setup: brain commits there and
never pulls or pushes it.

## brain sync

`brain sync [--vault <name>]...` brings each vault in step with its remote:

1. Pull each vault that has a remote, fast-forward only, from its branch's
   upstream.
2. Settle each promoting note of the private vaults (see promote below)
   with `gh pr view <shared_pr> --json state,mergedAt`.
3. Push each private vault that is ahead of its upstream. A shared vault is
   never pushed.

One line per vault, each followed by a line per note it settled:

```
personal: local-only: no remote, so nothing is pulled or pushed
  personal:notes/rollout.md: merged: removed the private copy; links resolve to team:notes/rollout.md (https://github.com/example-org/brain-shared/pull/7)
team: pulled 1 commit(s) from origin/main
```

- A vault that is not a git repo: `not a git repo: nothing to pull or push`.
- sync refuses a vault with uncommitted changes to tracked files, with a
  branch diverged from its upstream, or with a branch that tracks no remote
  branch. The hint says what to do; brain never stashes, resets or merges.
- MERGED: once a pulled shared vault holds a note with the same id, brain
  deletes the private copy and commits `brain: remove <path>`. Links and
  `id:` refs then resolve to the shared copy. Until the shared copy is
  there, the note waits.
- CLOSED: the note returns to `validated` and `shared_pr` is dropped.
- OPEN: the note waits.

sync exits 1 when a vault is refused or fails, or a pull request cannot be
read, and prints the report either way.

## brain promote --to

`brain promote <note> --to <shared vault> [--force]` shares a private note
through a pull request on the shared vault's repo:

1. The note must be `validated` and have an id (else exit 2). A target that
   is not a shared vault, or a note already in one, exits 6. A note that is
   `promoting` with an open `shared_pr` exits 2 unless `--force`.
2. brain fetches the shared clone and adds a temporary git worktree, under
   the state directory's `worktrees/`, of the remote branch the clone tracks
   (else the remote's default branch). The clone's own checkout is never
   touched.
3. The copy keeps the tier, the vault-relative path and the id, and drops the
   private-only fields (`status_reason`, `shared_pr`, `captured_by`,
   `distinct_from`, `expires_at`). `steward` stays.
4. The copy must pass `brain check` at error level against the shared
   vault's `.brain/policy.yaml` (exit 1 with the findings), and hold no
   secret (exit 8) or NUL byte (exit 2). A classification the policy does
   not accept exits 1, and the message names the ones it does:
   `classification internal is not accepted by team (accepts: public)`.
   With the `classification` rule at warning it only warns. A path the
   shared vault already has exits 4.
5. brain commits it on branch `brain/promote/<stem>-<short id>`, or with a
   `-2`, `-3` suffix when the remote has that name, pushes the branch and
   runs `gh pr create`. The body names the path, the id, the sources, the
   MOCs and the checks that passed. Then the worktree goes.
6. The private note becomes `promoting` with `shared_pr: <url>`, committed
   as `brain: share <path>`.

brain never edits a shared MOC (Map of Content). The note's `mocs:` field
lists the MOCs it belongs in; the pull request body and the command's output
say so.

`--dry-run` checks a promote first. It runs every gate the real one runs
(`target`, `status`, `repo`, `duplicate`, `secrets`, `classification`,
`check`) and changes nothing: no fetch, worktree, commit, push or `gh`, and
no write to either vault. It reads the shared clone as it is on disk,
without fetching, so a change the remote has since the last pull is not
seen. Each gate prints `pass`, `refused` or `skipped` with its detail, then
the target path and the branch. The real run may add a `-2` suffix to that
branch when the remote has the name. A refused gate does not stop the dry
run, so every gate shows. It exits 0 when the real promote would proceed,
else with the code the real one would give. `--dry-run` without `--to`
exits 2.

## brain vaults

`brain vaults` lists every configured vault in config order, the order a
bare note path resolves in. Each line names the vault, its path, and
whether it is shared or private and the default write target. Then come
its git state, how many notes its tiers hold, and the classifications its
`.brain/policy.yaml` accepts (`any` with no policy or an empty list). A
vault with a policy gets a second line with the rule levels it sets. The
git state is `remote` (with origin's URL, its credentials masked),
`local-only` (a repo with no `origin`) or `none` (the directory is not the
top of a git repo). It reads only the vaults and their git settings: it
uses no network and writes nothing. A missing or unreadable vault is listed
with its error and does not fail the command. Run it before `promote --to`
to see what a shared vault accepts.

## brain edit --pr

`brain edit <shared note> --pr` takes every edit flag (`--body`, `--append`,
`--set`, `--if-match` and the rest). The edit runs on the remote's tip in a
temporary worktree with the checks of a direct edit, on branch
`brain/edit/<stem>-<short id>`. brain pushes it, opens a pull request and
prints its URL. An edit that changes nothing exits 2. Without `--pr`, an edit
of a shared note exits 6 and the message names `--pr`.

## brain flag on a shared note

A shared note is not flagged in place. `brain flag <shared note> --reason
<text>` opens an issue with `gh issue create` on the repo of the shared
clone's remote: title `Disputed: <title>`, body with the note ref, its id and
the reason. `--dry-run` prints the issue and opens nothing. A clone with no
remote exits 6.

## brain similar

`brain similar <note>` lists notes in every vault that look like the given
one. A title or alias matches when it equals the note's title or an alias
once case and punctuation are folded. When a semantic index exists, notes at
least 0.85 alike in content match too (the write check refuses at 0.92). The
note itself, a copy with the same id, and revoked notes are left out. similar is a warning:
it always exits 0. `-o github` prints one warning annotation per similar
note, for a shared vault's CI.

## A shared vault's repo

`services/brain/examples/shared-vault/` in the thismoon repo is a starting
point. It has the four tiers with a sample note each, a `.brain/policy.yaml`
accepting `public` and `internal`, and a CODEOWNERS file with an owner per
`notes/<domain>/`. Its workflow runs `brain check -o github` on the changed
notes and `brain similar -o github` on the added ones. Its README says how to
set it up.

## JSON shapes

| Command | Top-level keys |
|---|---|
| `sync` | `ok`, `vaults[] {vault, shared, state, message, pulled, pushed, promotions[] {path, id, pr, state, action, detail}}`; `state` is `synced`, `local_only`, `not_git`, `refused` or `failed`, `action` is `removed`, `validated`, `waiting` or `failed` |
| `promote --to` | `vault`, `path`, `id`, `shared_vault`, `branch`, `pr`, `checks[]`, `mocs[]`, `warnings[]` |
| `promote --to --dry-run` | `dry_run`, `verdict` (`proceed` or `refused`), `vault`, `path`, `id`, `shared_vault`, `target`, `branch`, `base`, `gates[] {name, result, detail}`, `mocs[]`, `warnings[]`; `result` is `pass`, `refused` or `skipped` |
| `vaults` | `vaults[] {name, path, shared, default, git, remote, accepts_any, accepts[], policy {file, classifications, rules, error}, notes, error}`; `git` is `remote`, `local-only` or `none` |
| `edit --pr` | `vault`, `path`, `branch`, `pr`, `sha` |
| `flag` (shared) | `vault`, `path`, `id`, `repo`, `title`, `body`, `url`, `dry_run` |
| `similar` | `vault`, `path`, `similar[] {vault, path, title, match, score}`, `warnings[]`; `match` is `title`, `alias` or `semantic` |
