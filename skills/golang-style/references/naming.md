# Naming

Backed by [Effective Go](https://go.dev/doc/effective_go), [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments), the [Google Go Style Guide](https://google.github.io/styleguide/go/decisions), and the [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md).

## Case

- **MixedCaps / mixedCaps**, never `snake_case` or `ALL_CAPS` — including constants. Exported = leading capital, unexported = leading lowercase.
- **Initialisms keep one case as a unit:** `URL`, `ID`, `HTTP`, `JSON`. So `baseURL` (unexported), `NewID`, `apiURL`, `webURL`, `cacheJSON` — not `baseUrl` or `newId`. Repo examples: `present` uses `URL` in `Reference`; `events/internal/event/event.go` has `NewID`; `proxy.go` has `cacheJSON`.

## Packages

- Short, lowercase, and a single word. Base it on the directory: `store`, `proxy`, `ticker`. An underscore does not belong in a package name.
- **Reduce stutter** between package and exported name. It is `store.New`, not `store.NewStore`; `client.Event`, not `client.ClientEvent`. The caller already writes the package name.
- No `util`/`common`/`helpers` (see `structure.md`).

## Constructors and getters

- Constructors are `New`. From `events/internal/client/client.go`: `func New(baseURL string) *Client`. When construction can fail it is `(*T, error)` — `present`'s `func New(dir string) (*Store, error)`.
- **No `Get` prefix on getters.** Expose the field, or name the method for the noun: `store.DefaultRoot()`, `(*Store).ItemDir()`, `cfg.Hosts()`, `cfg.RouteMap()`. A setter is `SetX`; use `Fetch`/`Compute` for expensive or remote work.

## Errors

- Sentinel error variables are `ErrX`: `var ErrNotFound = ...`. Unexported ones are `errX`.
- Custom error types end in `Error`: `humanizer`'s `ValeNotInstalledError`.
- Error strings are lowercase with no trailing punctuation (they get concatenated): `errors.New("present: page not found")`. Package-prefix the message — see `errors.md`.

## Receivers

- One or two letters, an abbreviation of the type, **consistent across every method** on that type. The repo uses `s *Store`, `s *Server`, `c *Client`, `h *Handler`, `rh *reloadableHandler`, `it *Item`.
- Never `this`, `self`, or `me`.
- Value vs pointer receiver is a design choice covered in `functions.md`; the *name* stays consistent regardless.

## Variables and constants

- Name length scales with scope: `i`, `r`, `w` in a tight loop; descriptive names at package level.
- Don't restate the type in the name (`events`, not `eventList`).
- Constants are named by role, not value; no `K` prefix, no `ALL_CAPS`. The repo uses typed-less `const` string blocks for statuses (`StatusPending`, `StatusFired`).
- Build metadata lives in a shared `buildinfo` package, not a per-tool var: `Version`, `Commit`, `Tag`, `BuildTime`, all set via `-ldflags`. Read it with `buildinfo.Get()`; see `cli.md`.

## Functions

- A function reads as a noun if it returns something, a verb if it acts. Omit info already carried by the package or receiver.
- Printf-style functions end in `f` (`Logf`, `Errorf`).
- Don't shadow predeclared identifiers (`error`, `string`, `len`, `min`).
