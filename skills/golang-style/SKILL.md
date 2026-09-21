---
name: golang-style
description: Write and review well-structured, safe, idiomatic Go — naming, formatting, function and package design, error handling, code structure, and the HTTP/CLI/store patterns used across this codebase. Use when writing new Go, reviewing a diff for structure and idioms, or deciding how to lay out a package, name things, handle errors, or shape a function. Complements golang-pro (which owns concurrency, channels, generics depth, pprof, and gRPC). Invoke for Go code structure, idiomatic Go, gofmt/golangci-lint, error wrapping, package layout, function design, or "make this more idiomatic".
license: MIT
metadata:
  version: "1.1.0"
  domain: language
  triggers: Go, Golang, idiomatic Go, Go style, gofmt, gofumpt, golangci-lint, error wrapping, package layout, function design, Go naming, code structure, Go review
  role: specialist
  scope: implementation
  output-format: code
  related-skills: golang-pro
---

# Golang Style

Write Go the way this codebase already writes it: stdlib-first, compact, explicit, anti-magic. This skill carries the house template (the `cmd/` + `internal/` layout, cobra CLIs, `net/http` servers, file-backed stores, `fmt.Errorf("%w")` errors, hermetic table tests) plus the idioms that keep it consistent. It reinforces what the code does well and tightens the few places it drifts.

**Split with `golang-pro`:** `golang-pro` owns concurrent systems (goroutine patterns, channel pipelines, deep generics, pprof, gRPC, microservices). This skill owns *how code is structured and written*: naming, formatting, function and package design, error handling, safety, and the repo's HTTP/CLI/store/MCP patterns. When a task is "build a concurrent worker pool", use `golang-pro`. When it is "is this package laid out right, are the errors wrapped, is this function shaped well", use this skill.

**Where the two disagree, this skill wins.** `golang-pro` is vendored beside this skill (`skills/golang-pro`, from jeffallan/claude-skills) and its generic advice conflicts with house style in three places: it teaches functional options (banned here — config structs), `var _ Iface` satisfaction asserts (not house style, see `language.md`), and a `pkg/` public-library layout (house layout is `cmd/` + `internal/` only). Take its concurrency, generics, and profiling content; ignore those three.

## House style in one screen

- **Layout:** one module per tool. `cmd/<tool>/main.go` is thin and calls `internal/cli.Execute()`. Everything else lives under `internal/`, in packages named for their role (`store`, `server`, `ticker`, `proxy`, `notify`). No `util`/`common`/`helpers`.
- **Constructors:** `New(...)` returns a concrete `*T`, or `(*T, error)` when construction can fail. Never return an interface.
- **Config in, not options:** pass an `Input`/`Config`/`Patch` struct, not a long parameter list and not functional `WithX` options. Use pointer fields (`*string`) when "omitted" must differ from "cleared".
- **Interfaces at the consumer:** define small interfaces (1–3 methods) in the package that consumes them, for a test seam. Accept interfaces, return concrete types.
- **Errors:** wrap with `%w` whenever there is an underlying error. Package-prefix sentinel messages (`present: page not found`). Map sentinels to status/exit at the boundary with `errors.Is`/`errors.As`. Handle an error once — log it or return it, never both.
- **Side effects at the edges:** keep pure model and helpers in one file, the I/O shell in another. Push `exec`, file writes, notifications, and time to the edge behind a small interface or an injected `now func() time.Time`.
- **Boundary parsing:** parse and validate CLI, HTTP, config, and persisted input at the boundary. Pass typed, trusted values into the core and keep each invariant in one place.
- **Lifetimes:** pair acquired resources with cleanup, pass `context.Context` at real boundaries, and make every goroutine's exit condition visible.
- **Format + lint:** `goimports` + `golines -m 100` + `gofumpt` base; `golangci-lint run ./...` is the house linter.
- **Tests:** stdlib `testing` only (no testify). Derive expected behavior from the contract, not the implementation. Use table tests, `t.Helper()`, `t.TempDir()`, and an injected clock for hermeticity. Add `-race` for concurrent changes and fuzz parsers when their input risk warrants it.
- **Docs:** every package has a doc comment that states its contract; every exported name has a doc comment starting with its name.

## Reference guide

Load the file that matches the task.

| Topic | Reference | Load when |
|-------|-----------|-----------|
| Package & project layout, file organization | `references/structure.md` | Laying out a tool, splitting packages, ordering a file |
| Naming | `references/naming.md` | Naming packages, types, funcs, receivers, errors |
| Format & lint toolchain | `references/formatting.md` | gofumpt/golines/goimports, golangci-lint, imports, go vet |
| Function & method design | `references/functions.md` | Input structs, receivers, returns, consumer interfaces |
| Error handling | `references/errors.md` | Wrapping, sentinels, errors.Is/As, custom types, layering |
| Safety | `references/safety.md` | Nil/zero values, defer, copying, context, single-writer |
| Language-level idioms | `references/language.md` | Panic/recover, init(), iota enums, embedding, type assertions, time values |
| Testing | `references/testing.md` | Table tests, helpers, injected clocks, httptest |
| HTTP services | `references/http.md` | net/http ServeMux, JSON helpers, shutdown, middleware |
| CLI (cobra) | `references/cli.md` | Root command, RunE builders, flags, build metadata |
| Stores & persistence | `references/store.md` | File-backed stores, model/store split, ~ expansion |
| MCP servers | `references/mcp.md` | go-sdk tool handlers, structured input/output |

## Constraints

### MUST DO
- Put `main` in `cmd/<tool>/main.go` as a thin shim over `internal/cli.Execute()`; print the error once and `os.Exit(1)`.
- Wrap underlying errors with `%w` in `fmt.Errorf` (this is the one place the codebase drifts — `worklog/internal/store/store.go` uses `%v`; new and touched code uses `%w`).
- Package-prefix sentinel error messages.
- Return concrete types from constructors; define interfaces where they are consumed.
- Pass an `Input`/`Config`/`Patch` struct instead of 4+ positional parameters.
- Parse untrusted input at CLI, HTTP, config, and persistence boundaries before passing typed values inward.
- Make resource and goroutine lifetimes explicit; every acquired resource has cleanup and every goroutine has a visible exit condition.
- Inject `now func() time.Time` and use `t.TempDir()` so tests never touch the real `$HOME` or wall clock.
- Run `golangci-lint run ./...` and `goimports`/`golines`/`gofumpt` before calling code done.
- Give every package a doc comment and every exported name a name-leading doc comment.

### MUST NOT
- No `util`, `common`, `helpers`, or `base` packages. Keep helpers unexported in the package that uses them.
- No functional-options (`WithX`) pattern — this codebase uses config structs.
- No third-party logging (`zap`/`logrus`/`slog` adoption), assertion libraries (`testify`), or error libraries (`pkg/errors`) unless asked. Match the stdlib-first stack.
- No `log`-and-`return` of the same error.
- No interface returned from a constructor; no interface defined next to its single implementation just in case.
- No `context.Context` threaded through local-I/O store layers — keep it at process/exec/HTTP boundaries.
- No `panic` for expected errors (input, I/O, network) — panic only on broken invariants (`crypto/rand` failure, missing embedded asset), package-prefixed; no `recover` outside a single top-level boundary.
- No `init()` beyond cobra command/flag registration in `internal/cli`, and no mutable package-level state business logic writes to — state lives in a struct from `New(...)`.

## Workflow

**Writing:** scaffold the layout (`structure.md`) → name things (`naming.md`) → shape functions and constructors (`functions.md`) → wire the surface (`http.md` / `cli.md` / `store.md` / `mcp.md`) → handle errors (`errors.md`) → cover the safety checklist (`safety.md`) and language-level idioms (`language.md`) → add hermetic tests (`testing.md`) → format and lint (`formatting.md`).

**Reviewing:** read the diff against the checklist below; cite the specific house pattern the code should match, with a file path from this repo as the example.

## Review checklist — definition of done

- [ ] `cmd/<tool>/main.go` is thin; the root cobra command sets `SilenceUsage`/`SilenceErrors`; errors print once.
- [ ] Packages are role-named under `internal/`; no `util`/`common`.
- [ ] Constructors return concrete `*T` (or `(*T, error)`); interfaces are defined at the consumer.
- [ ] 4+ parameters are grouped into an `Input`/`Config` struct; optional-vs-clear uses pointer fields.
- [ ] Every `fmt.Errorf` with an underlying error uses `%w`; sentinels are package-prefixed; boundaries map them with `errors.Is`/`errors.As`.
- [ ] Side effects sit behind a small interface or injected dependency; the core is pure.
- [ ] Boundary input is parsed once into typed values; invariants and mutable state have a clear owner.
- [ ] Resource cleanup, context cancellation, and goroutine exit conditions are visible where applicable.
- [ ] Tests derive expectations from the contract, reproduce fixed bugs, and are hermetic (`t.TempDir`, injected clock); `-race` and fuzzing are used when the risk calls for them.
- [ ] No panic on expected errors; no `init()` logic; type assertions use comma-ok; durations are `time.Duration`; serialized enums are strings.
- [ ] Every package and exported name has a doc comment.
- [ ] `golangci-lint run ./...` is clean; code is `gofumpt`+`golines -m 100` formatted.
