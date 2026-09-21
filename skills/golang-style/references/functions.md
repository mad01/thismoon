# Function & method design

Backed by [Effective Go](https://go.dev/doc/effective_go), [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments), the [Google Go Style Guide](https://google.github.io/styleguide/go/best-practices), and the [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md).

## Input/Config/Patch structs over long parameter lists

When a function needs more than ~3 parameters, or when callers will grow over time, pass a struct. This is the dominant house pattern — and the codebase **does not** use functional `WithX` options.

```go
// worklog/internal/store/item.go
// CheckpointInput carries one checkpoint's content. Empty fields are skipped.
type CheckpointInput struct {
	Ticket string // set on first creation
	Topic  string
	Where  string // replaces the "Where I am" snapshot
	Note   string // prepended as a new "Log" entry
	Repo   string // repo this checkpoint touched ("" = none / not in a repo)
	Cwd    string // recorded as last_cwd
}
```

Other examples: `store.CreateInput`, `present`'s `store.Patch`, `mcpserver.Config`. The struct documents each field inline and lets callers set only what they need.

> Backing: the Google guide recommends grouping many or shared parameters into an options struct; Uber warns against opaque "naked" parameters at call sites.

## Pointer fields for optional-vs-clear

When a patch must distinguish "field omitted, leave unchanged" from "field set to empty, clear it", use pointer fields. From `present/internal/store/store.go`:

```go
// Patch carries optional field updates for Update. Nil fields are left
// unchanged; a non-nil empty string clears the field.
type Patch struct {
	Title      *string
	Content    *string
	Graph      *string
	References *[]Reference
}
```

The JSON boundary mirrors this with `*string` fields (`present`'s `updateInput` in its MCP server). Document the nil-vs-empty meaning in a comment every time.

## Receivers: value vs pointer

- **Pointer receiver** when the method mutates, when the type is large, or when it holds a `sync.Mutex`/uncopyable field. Stateful types (`*Store`, `*Server`, `*Client`, `*Handler`) always use pointer receivers.
- **Value receiver** for small, immutable, behavior-only types. `deps`' `alert.Osascript` is an empty struct, so `func (Osascript) Notify(...)` takes a value receiver.
- Be consistent within a type: don't mix value and pointer receivers on the same type.

## Returns

- **Return `(value, error)`** rather than an in-band sentinel like `-1` or `""`. `error` is the last return value and is typed `error`, not a concrete type.
- **Avoid named returns** in general. The one idiomatic use here is the single-return-through-a-helper pattern, where a named return reads no better than the explicit form, so the repo writes it explicitly:

```go
// events/internal/client/client.go
func (c *Client) Sources() ([]SourceCount, error) {
	var out []SourceCount
	return out, c.do(http.MethodGet, "/api/sources", nil, &out)
}
```

- Use named returns only when they genuinely aid the reader (e.g. a deferred close that sets the error), and keep naked returns to short functions. `nakedret` in the lint config flags the rest.

## Accept interfaces, return structs

Return concrete types from constructors and functions (`*Store`, `*Server`). Accept an interface only where you need a seam, and define that interface **in the consumer**, as small as possible. From `deps/internal/scanner/scanner.go`:

```go
// Checker is the OSV surface the scanner needs (real *osv.Client or a fake).
type Checker interface {
	Check(ctx context.Context, queries []osv.Query) ([][]store.Advisory, error)
}
```

The scanner takes this one-method view, so a fake checker in a test implements one method. The `alert.Notifier` interface (one method) and `proxy.Prober` (a func type) are the same idea: a tiny consumer-side seam for the side effect. Don't define an interface next to its only implementation "just in case" — add it when a second implementation or a test seam actually appears. See `safety.md` for keeping the implementation behind the seam pure.

## Let semantics set function boundaries

Do not split functions by line count. Keep a function at one level of abstraction with one reason to change. Split when it mixes policy with mechanics, owns unrelated state, or forces callers to understand details that belong behind a boundary. A long flat table or switch can be clearer than several tiny forwarding functions. Use complexity and nesting as signals to inspect the design, not as proof that a particular line count is wrong.
