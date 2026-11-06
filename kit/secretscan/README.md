# kit/secretscan

The secret scanner behind suspenders: the built-in detection rules, entropy
gating, per-repo suppression, and redaction. It lives in kit so any component
that needs to catch a checked-in secret uses the same rules as the pre-commit
hook.

- `DefaultRules` (content rules) and `DefaultFileRules` (filename rules) are
  the built-in sets. `New(rules)` builds a `Scanner`; `NewWatchRule` compiles
  a user pattern into a `Rule`.
- `ScanDir` scans git-tracked files, `ScanStaged` reads content from the
  index, and `ScanFile`, `ScanLine` and `CheckFileName` scan one input.
  `ScanContent` scans bytes that have not reached disk, under the path they
  are bound for.
- A `Finding` keeps the unredacted match in `RawMatch`. Print `Match` or
  `Context`, or pass the text through `Redact`.
- Skipped files (too large, unreadable) go to `Scanner.OnSkip`, or stderr when
  it is nil. A line over the size cap fails the scan.
- `LoadIgnoreConfig` reads a repo's `.suspenders.yaml`, and the inline
  `suspenders:ignore` marker suppresses findings on its line.

Adding or tuning a rule: `tools/suspenders/docs/working-on-it.md`. Run the
rule tests with `go test ./kit/secretscan/`.
