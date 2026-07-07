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

Release PRs pile up harmlessly — merge them when you want the release, not
before.

## Artifacts

Each released component gets, on its GitHub Release:

- `<name>_vX.Y.Z_darwin_arm64.tar.gz`: the binary, built with
  `CGO_ENABLED=0 -trimpath`, version embedded via ldflags
  (`internal/cli.Version` is set to `vX.Y.Z-<short-sha>`)
- `checksums.txt`: sha256 sums for every tarball
- `checksums.txt.bundle`: a cosign keyless signature over `checksums.txt`,
  signed with the workflow's GitHub OIDC identity

Artifacts target darwin/arm64 only; the platform is macOS-focused and the
linux targets carried no users (`docs/adr/0007`).

## Verifying a release

```sh
cosign verify-blob \
  --bundle checksums.txt.bundle \
  --certificate-identity-regexp '^https://github.com/mad01/thismoon/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt

sha256sum -c checksums.txt
```

The first command proves this repo's release workflow produced
`checksums.txt`; the second proves your tarball matches it.

## The csl exception

csl ships no prebuilt artifacts. It needs cgo (tree-sitter) plus ONNX runtime
libraries on the target machine, so a `CGO_ENABLED=0` cross-compile can't
build it and a tarball wouldn't run anyway. Its tag, changelog, and GitHub
Release still happen; the artifact steps are no-ops for its matrix entry.
Install csl from a checkout; the fleet does this via its ralph recipe.

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

- **`RELEASE_PLEASE_TOKEN`**: a fine-grained personal access token (PAT)
  scoped to this repo with Contents and Pull requests read-write. The
  default `GITHUB_TOKEN` isn't used because it may not create PRs without a
  repo-wide Actions toggle (too broad a grant for a public repo), and PRs it
  opens never trigger `ci.yml` checks. Symptom of an expired or under-scoped
  token: release-please fails with "Error adding to tree" or silently opens
  no PR. Fine-grained PATs expire after at most a year; renew and update
  the repo secret.
- **Concurrency group `release`**: two merges to main racing release-please
  can double-process the release PR, so runs queue instead of overlapping
  (`cancel-in-progress: false`).
