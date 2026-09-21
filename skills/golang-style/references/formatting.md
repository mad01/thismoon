# Format & lint toolchain

The house format and lint stack. Backed by [Effective Go](https://go.dev/doc/effective_go) and [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments) for the in-source rules, and the repo's own `Makefile`s for the toolchain.

## Format: goimports + golines + gofumpt

The dotfiles tools format with `golines -m 100 --base-formatter=gofumpt`. The `ralph` repo's `make format` is the canonical recipe:

```make
format:
	@goimports -w .
	@golines -m 100 --base-formatter=gofumpt -w .
```

- **`gofumpt`** — a stricter superset of `gofmt`. Tabs for indent, no manual alignment, opening brace never on its own line.
- **`golines -m 100`** — wraps long lines at a 100-column soft limit, breaking by syntax (one argument per line when a call is too long), not by hand.
- **`goimports`** — formats and maintains the three import groups (stdlib, third-party, internal).

Install them with `go install`:

```
go install golang.org/x/tools/cmd/goimports@latest
go install github.com/segmentio/golines@latest
go install mvdan.cc/gofumpt@latest
```

There is no hard line-length rule in Go itself — `golines -m 100` is this repo's chosen soft limit. Break long expressions by meaning, not by column count.

## Lint: golangci-lint is house standard

`golangci-lint` (installed at `~/code/bin/golangci-lint`) is the house linter. The `ralph` repo runs it across everything:

```make
lint:
	@golangci-lint run ./...
```

The `app-operator` repo carries a representative `.golangci.yml` — a good starting linter set for a new Go project here:

```yaml
linters:
  disable-all: true
  enable:
    - errcheck      # unchecked errors
    - govet
    - staticcheck
    - revive        # style/lint rules
    - unused
    - ineffassign
    - gocyclo       # cyclomatic complexity
    - goconst       # repeated string literals
    - nakedret      # naked returns in long funcs
    - unparam       # unused function parameters
    - prealloc      # slice preallocation
    - copyloopvar   # Go 1.22+ loop var
    - misspell
    - unconvert
    - gofmt
    - goimports
    - lll           # line length
```

`golangci-lint run --fix` auto-fixes what it can; `golangci-lint config verify` checks the config. `go vet ./...` is implied (govet runs inside golangci-lint) but is fine to run standalone.

### errcheck: exclude the genuinely-uninteresting functions

errcheck flags every unchecked error return. For functions whose error is genuinely not actionable — writing to `os.Stderr`/`os.Stdout`, closing a file in a `defer`, removing a temp file — exclude them **once** here rather than discarding with `_ =` at every call site (see `errors.md`):

```yaml
version: "2"
linters:
  settings:
    errcheck:
      exclude-functions:
        - fmt.Fprint
        - fmt.Fprintf
        - fmt.Fprintln
        - (*os.File).Close
        - os.Remove
        - os.RemoveAll
```

This is golangci-lint **v2** schema (`version: "2"` + `linters.settings.errcheck`). A v1 `linters-settings:` config fails under v2 with a JSON parse error. In this monorepo the dotfiles tools share one `.golangci.yml` at the repo root (golangci-lint searches upward from each tool's subdir); standalone repos each carry their own.

This keeps the bare call form readable and reserves `_ =` for genuine one-offs. Excluding `(*os.File).Close` means a failed flush-on-close on a *writable* file won't be caught — acceptable for these CLI tools; if a tool writes data it must not lose, check that specific `Close` explicitly instead.

## Gap to fix: no `-race` in the test targets

Every component Makefile has `fmt` and `lint` targets beside `test`, and the rule is to run `make fmt && make lint` in each touched component before pushing. What none of them has is `-race`: `make test` runs the plain suite. When touching a component with goroutines, run `go test -race ./...` in it before calling the change safe. Don't claim a tool is lint-clean unless you ran its lint target.

## In-source formatting rules

- **Imports in three blank-line-separated groups:** stdlib, third-party, internal (`goimports` enforces). Use `import _` only in `main`/tests; `import .` only to break a test cycle.
- **Group related declarations** — cluster related `const`/`var`/`type`; use `var (...)` and `const (...)` blocks.
- **Declare locals close to first use;** keep variable scope tight.
- **Reduce nesting with early returns.** Invert conditions, `return`/`continue` early, and omit `else` after a terminating branch. Keep the happy path at the left margin and indent the error path. The `group` switch in `status/internal/server/monitor.go` is the pattern: handle each case and return instead of nesting an else.
