---
id: ai-changelog-entries
label: likely_ai
bucket: ai
source: generated
license: generated for this eval
generator: claude-sonnet-5-5 via claude -p, 2026-10-08
words: 138
notes: Keep-a-changelog style entries; AI counterpart to hard-human-terse-changelog. Prompt was Write five CHANGELOG entries for version 2.4.0 of a Go HTTP router library, in markdown. Output only the changelog text, nothing else.
---
## [2.4.0] - 2026-10-08

### Added
- `Router.Group` now accepts variadic middleware, so you can attach shared middleware when you create a route group instead of calling `Use` afterward.
- Route patterns support regular-expression constraints on path parameters, for example `/users/{id:[0-9]+}`. Requests that don't match fall through to the next route or the 404 handler.

### Changed
- Path matching uses a compressed radix tree. Lookups on routers with more than 500 routes are about 18% faster, and per-request allocations are lower.

### Fixed
- `MethodNotAllowed` responses now include a correct `Allow` header listing every method registered for the matched path. Previously the header was omitted when the path was matched through a wildcard.
- Fixed a data race when routes were registered concurrently with in-flight requests. `Handle` now takes a write lock while it updates the tree.
