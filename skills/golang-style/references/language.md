# Language-level idioms

Panic, `init()`, enums, embedding, type assertions, and time values — the language-level rules the structural references assume. Backed by [Effective Go](https://go.dev/doc/effective_go), [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments), the [Google Go Style Guide](https://google.github.io/styleguide/go/decisions), and the [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md).

## Panic only on broken invariants, never for expected errors

Expected failures (missing file, bad input, network error) return an `error`. `panic` is reserved for conditions that mean the program itself is broken and cannot continue safely:

- A must-succeed primitive failing: the ID generators panic when `crypto/rand` fails (`present/internal/store/id.go`: `panic("present: crypto/rand failed: " + err.Error())`) — same pattern in `keeper-of-facts` and `events`.
- A build-time asset missing at startup: `d-man`'s blockpage panics when its embedded `index.html` isn't in the binary (`blockpage.go`).

Panic messages are package-prefixed like sentinel errors. Never panic on user input or I/O results, and never use panic/recover as control flow. The codebase has no `recover` — if you think you need one (a server that must survive a misbehaving handler), it goes at one top-level boundary only, and `net/http` already provides that for handlers.

## `init()` for cobra wiring only; no mutable package globals

`func init()` appears in exactly one place: `internal/cli` files registering subcommands and flags on the root command (`t-man/internal/cli/add.go` is the pattern; `cli.md`'s builder-function style is the alternative — either is fine, don't mix both in one tool). Rules:

- No I/O, environment reads, or computation in `init()` — it runs before `main`, has no error return, and makes startup order magic.
- Package-level `var`s are limited to cobra command/flag definitions, `Version`, and immutable lookup tables (`csl` writes its `extLabels` map once, then only reads it). No package-level mutable state that business logic writes to — state lives in a struct created by `New(...)` (see `functions.md`).
- Compiled regexes may live at package level with `regexp.MustCompile` (a legitimate panic-on-broken-invariant: the pattern is a constant).

## Enums: `iota` for internal states, strings for stored values

- Small internal state sets use a typed constant block with `iota`, where the zero value is a real, safe default: `suspenders`' `hookAbsent hookState = iota` — a zero `hookState` means "no hook file", which is exactly what an unset value should mean.
- Anything serialized (JSON stores, APIs, config) uses typed string constants instead, so files stay readable and reordering constants can't corrupt persisted data. `keeper-of-facts` stores assertion status as a string (`fresh`/`stale`/`retracted`) and `events` its level (`info`/`warn`/`error`), not as ints.
- If the zero value would be ambiguous, either make the first constant an explicit invalid/unknown state or start real values at `iota + 1` — don't let `0` silently mean something.

## Embedding: internal convenience, not API surface

- Embedding `sync.Mutex` in an unexported-usage struct is fine when the type is only ever passed by pointer (`proxy.Handler`) — but see `safety.md`: never copy such a struct.
- Don't embed types in exported structs to inherit their method sets — it leaks the embedded type's full API into yours and couples your contract to theirs. Prefer a named field and explicit forwarding methods for the one or two methods you actually want.
- Embedding an interface to partially implement it (test fakes aside) hides "method missing" until a runtime nil-method panic. Implement the methods you support explicitly.

## Type assertions: always comma-ok

A bare assertion `v.(string)` panics on mismatch. Use the comma-ok form and handle (or deliberately default) the miss — `belt`'s hook decoder reads untrusted JSON as `s, _ := p.ToolInput[key].(string)`, where an absent or mistyped key safely yields `""`. Same rule for map reads where absence matters (`v, ok := m[k]`) and for `errors.As` targets. Type switches (`switch v := x.(type)`) are the idiom when there are several concrete cases.

Interface-compliance asserts (`var _ http.Handler = (*Server)(nil)`) aren't house style — the codebase has none, because concrete types are wired directly and the compiler checks at the call site. Add one only when implementing a third-party interface whose use is far from the definition.

## Time: `time.Duration` and `time.Time`, never bare ints

- Durations are `time.Duration`, in signatures, struct fields, and constants (`5 * time.Second`) — a bare `int` timeout forces every reader to guess the unit.
- Instants are `time.Time`. Comparisons use `.Before`/`.After`/`.Equal`, not unix-second arithmetic.
- Code never calls `time.Now()` directly in logic that needs testing — it takes an injected `now func() time.Time` (see `store.md` and `testing.md`).
