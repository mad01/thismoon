# Security updates

thismoon keeps its dependencies up to date as far as security needs, and no further. A dependency moves when a known advisory reaches code thismoon runs. Nothing bumps versions on a schedule. Merging to main deploys to every machine, so a person merges each security pull request by hand.

## What watches the dependencies

- govulncheck (`.github/workflows/govulncheck.yml`) checks the Go standard library and every Go module against the Go vulnerability database, and reports only vulnerable code that thismoon calls. The workflow runs on pull requests that change `go.mod` or `go.sum`, every Monday, and on demand from the Actions tab. Because it isn't part of `ci-ok`, a new advisory turns the check red without blocking other merges.
- Dependabot security updates open a pull request when an advisory hits a Go module, an npm package in the `webkit` or `services/brain/web` lockfile, or a GitHub Action. The two composite actions under `.github/actions` count too. `.github/dependabot.yml` sets the commit prefixes and keeps weekly version updates off.
- The deps service (`services/deps`) checks the pinned dependencies of every local repo against the Open Source Vulnerabilities (OSV) database. It reads `go.sum`, so it also flags modules the build never imports. For Go, govulncheck decides.

The base images of the build-only containers (`webkit/Dockerfile.build`, `services/brain/web/Dockerfile.build`, `tools/suspenders/Dockerfile`) have no watcher. They pull the current image for their tag on each build. The `present` image pins its base images by digest, and the Go bump below moves its build image.

## When govulncheck fails

Look at the module the finding names. For a Go module, run `go get <module>@<fixed version>` and `go mod tidy`, then `make test`, and open a pull request. For `stdlib`, the standard library, only a new Go release fixes it, so follow the Go bump below.

To reproduce a finding, run `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...` at the repo root with the Go version from `go.mod`. Don't use a govulncheck binary built with an older Go: it fails to load the packages, while `go run` builds it with the current one.

## Bump the Go version

1. Run `go mod edit -go=<version>` at the repo root. CI reads its Go version from `go.mod`.
2. Move the golang base images. `services/present/Dockerfile` pins a digest: take the new one from `docker buildx imagetools inspect golang:<version>-bookworm`. `tools/suspenders/Dockerfile` uses a tag.
3. Update the docs that name the old version, found with `git grep -n '<old version>'`.
4. Add a `docs/UPGRADING.md` entry. A change to the root `go.mod` alone rebuilds no component, so each machine needs `ralph up --reset-builds`.
5. Move the consuming repo's Go pin to the same version, so code outside this repo builds on it too. thismoon builds switch to the version in `go.mod` through `GOTOOLCHAIN=auto` either way, so the two changes can merge in either order.

After `ralph up --reset-builds`, `go version -m ~/code/bin/csl | head -1` prints the new version.
