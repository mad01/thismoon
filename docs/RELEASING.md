# Releasing

Components release independently, on merge to main, with no manual tagging.
[release-please](https://github.com/googleapis/release-please) in manifest
mode watches each path registered in `release-please-config.json`, maintains
a release PR per component, and cuts a `name/vX.Y.Z` tag plus a GitHub
Release when that PR merges. A CI matrix job then builds and signs the
artifacts.

## Version bumps

Versions come from conventional commits, scoped by path. A commit touching
`services/present/` counts toward present's next release and nobody else's:

- `fix:` bumps the patch version
- `feat:` bumps the minor version
- `feat!:` (or a `BREAKING CHANGE:` footer) bumps the major version
- `chore:`, `docs:`, `ci:` and friends don't trigger a release

Current versions live in `.release-please-manifest.json`; don't edit it by
hand, release-please owns it.

## The release PR

1. A releasable commit merges to main.
2. release-please opens (or updates) a release PR for the affected component:
   version bump in the manifest, generated changelog.
3. Merging that PR creates the component tag (`present/v1.2.3`) and the
   GitHub Release.
4. The `artifacts` job fans out over every path released in that run and
   uploads the tarballs.

Release PRs pile up harmlessly; merge them when you want the release, not
before.

## Artifacts

Each released component gets, on its GitHub Release:

- `<name>_vX.Y.Z_darwin_arm64.tar.gz`: the binary, built natively on a
  macOS arm64 runner with `-trimpath` (`CGO_ENABLED=0` for everything
  except csl, which needs cgo for its tree-sitter grammars), build metadata
  embedded via ldflags
- `checksums.txt`: sha256 sums for every tarball
- `checksums.txt.bundle`: a cosign keyless signature over `checksums.txt`,
  signed with the workflow's GitHub OIDC identity

Artifacts target darwin/arm64 only; the platform is macOS-focused and the
linux targets carried no users (`docs/adr/0007`).

## Build metadata

The ldflags set four variables in the shared
`github.com/mad01/thismoon/buildinfo` package, the same four every local
`make build` injects through `buildinfo.mk`:

| Variable | Release build |
|---|---|
| `Version` | `vX.Y.Z-<short-sha>` (a local build sets the short sha alone) |
| `Commit` | the full sha the tag points at |
| `Tag` | the component tag, `<name>/vX.Y.Z` |
| `BuildTime` | UTC, RFC 3339 |

A released binary reports all four with `<name> version -o json`, and services
serve the same object from `GET /version`, so you can ask a downloaded or
deployed binary which release it came from. Plain `<name> version` stays the
bare version token that status and ralph parse.

Swift components take no ldflags: release-please writes the version into their
source, and the workflow builds them with `swift build`.

## Verifying a release

```sh
cosign verify-blob \
  --bundle checksums.txt.bundle \
  --certificate-identity-regexp '^https://github.com/mad01/thismoon/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt

shasum -a 256 -c checksums.txt
```

The first command proves this repo's release workflow produced
`checksums.txt`; the second proves your tarball matches it.

## Adding a new component

Register the component in the `packages` map of
`release-please-config.json`:

```json
"services/newthing": { "component": "newthing" }
```

CI fails the PR if a Makefile-bearing component is missing from the map, so
you can't forget. The first release uses `initial-version` (0.1.0).

## Maintainer setup

Two pieces of repo configuration the workflow depends on:

- **GitHub App token**: release-please authenticates with a token minted
  per run by a GitHub App (`actions/create-github-app-token`), not the
  default `GITHUB_TOKEN`. The default may not create PRs without a repo-wide
  Actions toggle (too broad a grant for a public repo), and PRs it opens
  never trigger `ci.yml` checks. The App token expires within the hour, so
  there is no long-lived secret to rotate. Symptom of a missing or misscoped
  App: release-please fails with "Error adding to tree" or silently opens no
  PR.

  Setup (reproduce if the App is ever lost):

  1. Create a GitHub App under the account that owns the repo
     (`https://github.com/settings/apps/new`). Uncheck Webhook → Active;
     set "Only on this account".
  2. Repository permissions: Contents read-write, Pull requests read-write,
     Issues read-write. Issues is required for release-please's labels; the
     other two for the branch and the release PR.
  3. Create the App, note its App ID, then Generate a private key
     (downloads a `.pem`).
  4. Install the App on this repo only (App → Install App).
  5. Add two repo secrets under
     `Settings → Secrets and variables → Actions`: `RELEASE_APP_ID` (the App
     ID) and `RELEASE_APP_PRIVATE_KEY` (the full `.pem`, including the
     `BEGIN`/`END` lines).

  The `release.yml` `create-github-app-token` step exchanges these for a
  short-lived installation token each run. The private key itself doesn't
  expire, but it can be regenerated from the App's settings without touching
  the workflow.
- **Concurrency group `release`**: two merges to main racing release-please
  can double-process the release PR, so runs queue instead of overlapping
  (`cancel-in-progress: false`).
