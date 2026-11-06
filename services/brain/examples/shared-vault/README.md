# Example shared brain vault

A starting point for a vault a team shares through a git repo, here called
`example-org/brain-shared`. brain treats a shared vault as read-only: notes
change only through pull requests, which brain opens for you, and this
repo's CI checks every one against the vault's policy.

## What is here

- `inbox/`, `notes/`, `outputs/` and `MOCs/` (Maps of Content): the four
  tiers, one sample note each. Notes live under `notes/<domain>/`, so a
  domain can have its own owners.
- `.brain/policy.yaml`: the classifications the vault accepts (`public` and
  `internal`) and the rules that are errors here rather than warnings.
  `brain promote --to` checks a note against this file before it pushes.
- `.github/CODEOWNERS`: maintainers own the policy and the workflow, and
  each team owns its domain under `notes/`.
- `.github/workflows/brain-check.yml`: on every pull request, runs
  `brain check -o github` on the changed notes and `brain similar -o github`
  on the added ones. similar only warns; check fails the job on an error.

## Set up the repo

1. Create `example-org/brain-shared` and copy this directory's contents to
   its root, `.brain/` and `.github/` included.
2. Replace the teams in `.github/CODEOWNERS` with yours, and add a line for
   each domain directory you create under `notes/`.
3. Pin brain in the workflow. Pick a brain release of `mad01/thismoon` (tag
   `brain/vX.Y.Z`), set `BRAIN_VERSION` to `vX.Y.Z`, and set `BRAIN_SHA256`
   to the sha256 of `brain_vX.Y.Z_darwin_arm64.tar.gz` from the release's
   `checksums.txt`. Check that file's signature first:

   ```sh
   gh release download brain/vX.Y.Z --repo mad01/thismoon \
     --pattern 'checksums.txt*'
   cosign verify-blob --bundle checksums.txt.bundle \
     --certificate-identity-regexp '^https://github.com/mad01/thismoon/' \
     --certificate-oidc-issuer https://token.actions.githubusercontent.com \
     checksums.txt
   grep darwin_arm64 checksums.txt
   ```

   The workflow refuses a tarball that does not match, so it fails until
   both values are set.
4. Protect the default branch: require a pull request, a review from Code
   Owners and the `check` job. Delete the sample notes once real ones exist.

## Use it from brain

Each member clones the repo and adds it to their brain config next to
their private vault:

```yaml
vaults:
  - name: personal
    path: ~/notes
    default: true
  - name: team
    path: ~/src/github.com/example-org/brain-shared
    shared: true
```

brain uses `git` and `gh` for the repo, so run `gh auth login` once. Then:

- `brain sync` pulls both vaults, pushes the private one when it has a
  remote, and settles the notes waiting on a pull request.
- `brain promote personal:notes/x.md --to team` copies a validated private
  note to a branch, pushes it and opens a pull request. The private copy
  waits with status `promoting` until `brain sync` sees the pull request
  merged (brain removes the copy) or closed (the note is validated again).
- `brain edit team:notes/platform/x.md --pr --append "..."` opens a pull
  request with the edit.
- `brain flag team:notes/platform/x.md --reason "..."` opens an issue on
  the repo.

brain never edits a MOC in a shared vault. A promoted note's `mocs:` field
names the MOCs it belongs in, and the pull request says so; add the links
in the same pull request or a later one.
