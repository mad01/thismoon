# How `history clean` works

`suspenders history clean` removes strings from a repository's entire git history: file contents and commit messages across every commit on every local branch and tag. This page explains what actually happens when you run it, so you can judge the risk before rewriting anything.

The one-line version: history is re-created by piping `git fast-export` through an in-process text transform into `git fast-import`. Git does all the object plumbing; suspenders only edits the byte stream between the two processes, so there is nothing extra to install.

```
git fast-export <local refs>
        |
        v
  suspenders transform      replace flagged strings, redact flagged files,
        |                   drop stale signatures
        v
git fast-import --force     writes new objects, moves refs
        |
        v
git reset --hard            syncs the working tree
```

## The steps, in order

### 1. Collect the replacement set

Explicit flags win; the scan only runs when none are given:

- `--replace <string>` flags, repeatable, plus one string per line from `--replace-file`.
- `--redact-file <glob>` flags, repeatable, plus globs from `history.redact_files` in the config: files whose entire content is the secret (a key file, a token file). A bare file name like `id_rsa` matches in any directory; a glob with a path (`keys/*.pem`) matches the full repo-relative path.
- With no `--replace` or `--redact-file` flags, suspenders runs the same walk as `history scan` over the public branch and collects every raw secret match and every guard blocked-name match. The raw values are used internally for replacement but never printed unredacted. Findings from file-name rules (say, a committed `id_rsa`) become whole-file redactions of that path.

### 2. Check preconditions

The command refuses to start on a bare repository, on a detached HEAD, or with a dirty working tree. Nothing has been touched at this point, so a failed precondition costs you nothing.

### 3. Write a backup bundle

Before any rewrite, suspenders runs `git bundle create .git/suspenders-backup-<timestamp>.bundle --all`. This is mandatory, not a flag. A bundle is a single file holding every ref and every object, so the original history is always one `git clone <bundle>` away.

### 4. Confirm the blast radius

Unless you pass `--yes`, the command prints exactly what it is about to do (the refs it will rewrite, the strings it will replace, redacted, and the files it will redact) and waits for a `y`. Pass `--dry-run` instead to run the export and transform with the output thrown away: you get the same counts a real run would produce, and nothing changes.

### 5. Export

Suspenders lists local refs explicitly (`git for-each-ref refs/heads refs/tags`) and hands them to `git fast-export --reencode=no --signed-tags=strip`. It deliberately avoids `--all`: that would also rewrite remote-tracking refs, leaving them claiming a state the actual remote doesn't have.

fast-export produces a text stream in which every file content and every commit/tag message appears as a `data <n>` block: a header announcing exactly n bytes of payload, followed by those bytes.

### 6. Transform the stream

This is the only part suspenders adds, and it is a pure reader-to-writer edit:

- **File contents and messages.** Each `data` payload is read in full, every flagged string is replaced with `***REDACTED***`, and the block is re-emitted with a recomputed byte count. Both blob data and commit/tag messages go through this, so a secret pasted into a commit message is removed too.
- **Redacted files.** A blob whose path matches a redact glob has its whole payload replaced with `***REDACTED***`, binary or not. The file stays present in every commit's tree — its history shows it existed, but no version of its content survives. Redaction takes precedence over the protected-path skip below. One caveat: git stores identical content once, so if a redacted file and an unredacted file had byte-identical content, both are redacted.
- **Protected files.** Lockfiles and dependency manifests (`go.sum`, `package-lock.json`, and friends) are never string-replaced: a flagged string can appear inside a base64 integrity hash, and blind substitution would corrupt it. Matches in these files are counted and reported instead. A redact glob overrides this.
- **Binary files.** A payload with a null byte in its first 512 bytes is passed through untouched — byte replacement inside a binary could corrupt it. If a binary contains a flagged string, you get a warning and a count instead of a modification. Redact globs still apply: a matching binary is redacted wholesale.
- **Signatures.** `gpgsig` blocks are dropped. A commit signature signs the exact bytes of the old commit, so it would be invalid after any rewrite anyway; dropping it makes that explicit. The count appears in the summary.
- **Everything else** passes through byte-identical. A unit test pins this: a stream with no matches comes out exactly as it went in.

### 7. Import and sync

The transformed stream feeds `git fast-import --force`, which writes the new objects and moves every local branch and tag onto the rewritten commits. Because content changed, every commit hash from the first affected commit onward is new. A final `git reset --hard` brings your working tree in line with the rewritten HEAD.

## After the rewrite

The rewrite is purely local, and the old objects aren't gone yet:

- **The remote still has the old history.** Push with `git push --force-with-lease origin <branch>` for every rewritten branch.
- **Collaborators still have the old history.** Anyone with an existing clone holds the removed strings; they need to re-clone (or hard-reset onto the rewritten branch). A rewrite doesn't un-leak a secret: if it was pushed anywhere, rotate it. The rewrite stops the value from spreading further, nothing more.
- **Your own `.git` still has the old objects**, reachable from the reflog and stored in the backup bundle. When you are sure the rewrite is right, drop them with `git reflog expire --expire=now --all && git gc --prune=now`, and delete the bundle.

## Undoing a rewrite

The backup bundle is a complete copy of the pre-rewrite repository:

```sh
git clone .git/suspenders-backup-<timestamp>.bundle restored-repo
```

Or fetch it back into the existing checkout with `git fetch <bundle> <branch>` and reset. `git bundle verify <bundle>` confirms the file is intact.

## Why not git-filter-repo?

git-filter-repo is the battle-tested standard for this job, but it is a Python tool the user has to install, and suspenders is meant to be a single offline binary. The trade-off and its mitigations are recorded in [ADR 0002](adr/0002-native-history-rewrite.md).
