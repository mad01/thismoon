# Per-component releases via release-please manifest mode

Components release independently on merge to main: release-please in manifest mode, component tags in the form `svc/vX.Y.Z` (path prefix, slash separator). Artifact builds are a plain CI matrix job over the release tags.

## Considered Options

goreleaser's monorepo support was rejected because it is a Pro (paid) feature. A hand-rolled matrix job over `go build` covers the three target platforms without the license cost.
