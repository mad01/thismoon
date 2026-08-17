# Package & project layout

How tools in this repo are laid out, and how to organize a file. Backed by [Effective Go](https://go.dev/doc/effective_go), [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments), [Organizing a Go module](https://go.dev/doc/modules/layout), and the [Google Go Style Guide](https://google.github.io/styleguide/go/best-practices).

## Module per tool, `cmd` + `internal`

Each tool is its own module: `module github.com/mad01/dotfiles/<tool>`. The entrypoint is a thin `cmd/<tool>/main.go` that delegates to `internal/cli`. Everything else lives under `internal/` so there is no exported library surface to keep stable.

```
worklog/
  cmd/worklog/main.go   — entry → cli.Execute()
  internal/
    cli/                — cobra command tree
    store/              — the on-disk tree: items, per-repo notes, git
    mcpserver/          — MCP tool wiring
  go.mod                — module github.com/mad01/thismoon (monorepo root; worklog is tools/worklog)
  Makefile
```

`main.go` is a fixed 15-line idiom: call `cli.Execute()`, print `tool: err` to stderr, `os.Exit(1)`. No logic in `main`.

## Packages named for their role

Split packages by what they *do*, not by type. Real package names from the repo: `store`, `server`, `client`, `ticker`, `notify`, `proxy`, `config`, `hosts`, `render`, `scan`, `mcpserver`. Each owns one job.

**Never create `util`, `common`, `helpers`, or `base`.** A cross-cutting helper stays unexported in the package that uses it (`store.contains`, `proxy.normalizeHost`). If two types must reference each other, they belong in the same package, not a shared "base" one.

> Backing: Effective Go and the Google guide both say package names should describe what the package provides; the Uber guide and Code Review Comments call out generic catch-all packages as an anti-pattern.

## One module = one root package; split only when it earns it

A tool starts as a few `internal/` packages. Split a package out when it has a distinct job and a clean boundary (a `store` that owns files, a `notify` that owns a side effect). Don't pre-split into one-type-per-file packages.

## Functional core, imperative shell

Keep pure model + helpers in one file and the I/O shell in another. `reminder/internal/store/reminder.go` holds the `Reminder` model and pure functions (`NextDue`, `Overdue`); `store.go` and `id.go` hold the mutable, file-touching store. thismoon `services/d-man/internal/hosts` is pure `Validate`/`Render`/`Splice` plus a thin `Sync` shell. Side effects (`exec`, file writes, notifications, time) live at the edge — see `safety.md` and `functions.md`.

## File organization

- **Group by cohesion, not one type per file.** One logical unit per file. `reminder/internal/store/` splits the model (`reminder.go`) from the store (`store.go`) from id generation (`id.go`).
- **Declaration order:** package doc comment → imports (three groups) → exported types → `New` constructor → methods → unexported helpers last.
- **Imports in three groups**, separated by blank lines: stdlib, third-party, internal. `goimports` maintains this. Example from `worklog/internal/cli/cli.go`:

```go
import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/worklog/internal/store"
)
```

- **Moderate file sizes.** Largest non-test files run ~450 lines; most are 200–350. No god-files.
- **Tests sit beside source** as `*_test.go` in the same package (white-box) or `_test` package (black-box) — see `testing.md`.

## Doc comments state the contract

Every package has a doc comment, and the good ones explain *why* and the invariant, not just *what*. From `worklog/internal/store/store.go`:

```go
// Package store manages the on-disk worklog tree: one directory per work item
// (keyed by ticket id or topic slug), a CONTEXT.md contract per item, lazy
// per-repo note files, and a local git history. There is no remote — the store
// is machine-local by design so internal references never leave the machine.
package store
```

Every exported name gets a doc comment starting with its name (`// Store is...`, `// New returns...`). See `naming.md` and the Code Review Comments doc-comment rules.
