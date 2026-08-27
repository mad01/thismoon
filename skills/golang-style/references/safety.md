# Safety

Zero values, nil, defer, copying, context, and concurrency discipline. Backed by [Effective Go](https://go.dev/doc/effective_go), [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments), the [Google Go Style Guide](https://google.github.io/styleguide/go/decisions), and the [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md).

## Make the zero value useful

A freshly declared value should be usable. `var b strings.Builder`, `var mu sync.Mutex`, and `var buf bytes.Buffer` all work with no init. Design your own types the same way where you can. `reminder`'s status field is stored, not derived, so a zero `Reminder` is never silently "pending" — be explicit when the zero value would be ambiguous, and start enums at a real value rather than letting `0` mean something.

## Nil slices and maps

- A `nil` slice is a valid empty slice. Prefer `var t []T` over `[]T{}`, and test emptiness with `len(s) == 0`, not `s == nil`.
- Reading a `nil` map is fine; writing to one panics. Allocate before writing. When returning a list, returning `nil` for "none" is idiomatic — `worklog`'s `List` returns `nil, nil` when the root doesn't exist.

## Defer for cleanup

Use `defer` to pair acquire/release next to each other: `defer res.Body.Close()`, `defer t.Stop()`, `defer cancel()`. Arguments are evaluated at the `defer` statement, and defers run LIFO. Avoid `defer` inside a hot loop where resources would pile up until return — close per iteration instead.

## Copy at the boundary; don't copy locks

- When you hand a caller a slice or map that aliases internal state, copy it first so they can't mutate your innards.
- Never copy a struct that holds a `sync.Mutex` or whose methods take pointer receivers. `proxy.Handler` embeds `sync.Mutex` as a non-pointer field and is only ever passed as `*Handler`.

## Context at the edges, not through stores

The house rule: thread `context.Context` where it crosses a real boundary (a process lifetime, an `exec`, or an HTTP request), and **not** through local-I/O store/render layers that do quick synchronous work.

- `ticker.Run(ctx, ...)` takes a context for cancellation and selects on `ctx.Done()`.
- `humanizer`'s vale `exec` calls thread `ctx`.
- `store` and `render` APIs take **no** context — they do fast local I/O. This is a deliberate decision, not drift; keep it consistent.

When you do take a context, it is the **first** parameter, named `ctx`, never stored in a struct field. Only call `context.Background()` at an entry point.

## Parse at the boundary

CLI flags, HTTP bodies, config files, environment variables, and persisted data are untrusted input. Decode and validate them at the boundary, then pass typed values to inner packages. Do not spread the same validation across handlers, stores, and business logic. A domain invariant should have one owner and one error shape.

## Concurrency only where it pays; single-writer over scattered locks

These are mostly sequential CLI/HTTP tools. Add goroutines only where they earn it, and prefer one owner of mutable state over locks sprinkled everywhere.

- **Single-writer:** `reminder serve` is the *only* writer of the JSON store; the MCP and CLI are HTTP clients to it, so there are no file-lock races. Design for one owner before reaching for mutexes.
- **Goroutines where justified:** `d-man` probes backends concurrently with a `sync.WaitGroup` and an indexed result slice (no shared-write race), behind a `sync.Mutex`-guarded TTL cache. Its `serve` runs a watch loop plus signal handling.
- **Cancellation:** loop on `select { case <-ctx.Done(): return; case <-t.C: ... }` (the `reminder` ticker), and shut an HTTP server down on `signal.NotifyContext` (the `d-man` daemon — see `http.md`).
- **Lifetimes:** when spawning a goroutine, make its owner and exit condition clear at the call site. Do not start background work that has no cancellation, join, or process-lifetime contract.
- A swappable handler guards its pointer with a `sync.RWMutex` (`d-man`'s `reloadableHandler`) so config reloads don't restart the listener.

Run tests with `-race` when you do add concurrency (the internal tools don't yet pass `-race` by default — adding it is a worthwhile uplift).

## Keep the side effect at the edge behind a seam

Push `exec`, file writes, notifications, and randomness to the edge behind a tiny interface so the core stays testable. `notify.Notifier` wraps the `osascript` call; the ticker fires through the interface and a test passes a fake. The same edge escapes untrusted input — `notify.appleScriptString` escapes quotes and backslashes so a reminder title can't break out of the AppleScript literal. Use `crypto/rand` for anything security-sensitive, never `math/rand`.
