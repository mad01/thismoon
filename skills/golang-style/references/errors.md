# Error handling

Backed by [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments), the [Google Go Style Guide](https://google.github.io/styleguide/go/best-practices), the [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md), and the `errors` package docs.

## Wrap with `%w` (the one place the codebase drifts)

When you build an error from an underlying one, wrap it with `%w` so the chain stays inspectable by `errors.Is`/`errors.As`. `present` does this almost everywhere:

```go
// present/internal/store/store.go
if err := os.MkdirAll(s.pagesDir(), 0o755); err != nil {
	return nil, fmt.Errorf("create pages dir: %w", err)
}
```

**The deviation to fix:** `worklog/internal/store/store.go` wraps underlying errors with `%v`, which flattens the chain:

```go
// worklog/internal/store/store.go — current
return fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
```

New and touched code uses `%w` for the underlying error:

```go
return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
```

Use `%v` only when you *deliberately* want to obscure the chain at a boundary (e.g. not leaking an internal error type to an API client). Put the `%w` verb at the end of the message so it reads in chain order, and add only non-redundant context when wrapping.

## Sentinel errors, package-prefixed

Define a sentinel `var` only where a consumer branches on it. Prefix the message with the package name — it reads better when printed at the CLI top level. From `present`:

```go
// ErrNotFound is returned by Get and Update when no page exists for the id.
var ErrNotFound = errors.New("present: page not found")
```

**Standardize on the prefix.** `present` prefixes (`present: page not found`); so does `events` (`event: source is required`). Use the package-prefixed form for new code.

## Map errors at the boundary with `errors.Is`/`errors.As`

Branch on a sentinel with `errors.Is`, never by string-matching. The HTTP layer maps the store's sentinel to a status code in one helper:

```go
// keeper-of-facts/internal/server/server.go
func writeStoreErr(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeErr(w, http.StatusBadRequest, err)
}
```

Use `errors.As` to pull a custom type back out — `humanizer` retrieves its `ValeNotInstalledError` that way to print install instructions. Standard-library sentinels work the same: `os.ErrNotExist`, `http.ErrServerClosed` (see `http.md`).

## Custom error type when there's structured data

Reach for a custom error type when callers need fields off the error, not just its identity:

```go
// humanizer/internal/rules/vale.go
type ValeNotInstalledError struct{ Binary string }

func (e *ValeNotInstalledError) Error() string { ... }
```

Otherwise a sentinel `var` or a plain `fmt.Errorf` is enough. Pick by two axes: is the message fixed or built at runtime, and does a caller need to match it.

## Handle an error once

Log it **or** return it, never both. In this codebase, library functions return errors and the CLI handles them once at the top: the cobra root sets `SilenceErrors: true` and `main` prints `tool: err` (see `cli.md`). Logging happens only in genuine fire-and-forget paths — `deps` logs a failed notification and returns nil so the flags stay pending for the next cycle, because no caller could do better:

```go
// deps/internal/scanner/scanner.go
if err := n.Notify(title, body); err != nil {
	log.Printf("deps: notify failed, will retry: %v", err)
	return 0, nil
}
```

## CLI vs library layering

- **Library packages** (`store`, `render`, `config`) return descriptive errors and never call `os.Exit` or `log.Fatal`.
- **The HTTP client** turns a transport error into a user-facing one with `%w` so the cause is still there: `events`' `client.do` wraps a dead-server dial with "events serve not reachable … (t-man status events): %w".
- **`os.Exit`/`log.Fatal` only in `main`** (here, only in the `cmd/<tool>/main.go` shim).

## Always check errors — but don't litter `_ =` to silence the linter

Check every error that can carry real information. The `errcheck` linter enforces this. There are two clean ways to satisfy it, and a wrong one:

- **Wrong:** scattering `_, _ = fmt.Fprintf(...)`, `defer func() { _ = f.Close() }()`, `_ = os.Remove(tmp)` across the codebase purely to make errcheck pass. This buries the signal — a reader can no longer tell a deliberate ignore from linter-appeasement noise.
- **For whole classes of genuinely-uninteresting returns** — writing to `os.Stderr`/`os.Stdout`, `(*os.File).Close` in a `defer`, removing a temp file — exclude the function **once** in the lint config (`errcheck.exclude-functions`, see `formatting.md`). Then write the call bare: `fmt.Fprintln(os.Stderr, msg)`, not `_, _ = fmt.Fprintln(...)`. The canonical `main` shim does exactly this — bare `fmt.Fprintln(os.Stderr, ...)` (see `cli.md`).
- **For a genuine one-off** where ignoring is safe and obvious but the function isn't worth a global exclude — discard explicitly with `_ =` and a short comment: `_ = json.NewEncoder(w).Encode(v) // header already written`.

Rule of thumb: if you're ignoring the same function's error in more than two or three places, it belongs in `exclude-functions`, not in repeated `_ =` discards.
