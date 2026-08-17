# Stores & persistence

The file-backed store pattern shared by `worklog`, `reminder`, and `present`. Backed by the repo stores.

## A `Store` struct with an injectable clock

A store is a concrete type rooted at a directory, with `now` injectable for tests. `New` returns `*Store` (or `(*Store, error)` when it must create directories). From `worklog/internal/store/store.go`:

```go
// Store is a worklog tree rooted at Root. Now is injectable for tests.
type Store struct {
	Root string
	Now  func() time.Time
}

// New returns a Store rooted at root (DefaultRoot if empty).
func New(root string) *Store {
	if root == "" {
		root = DefaultRoot()
	}
	return &Store{Root: root, Now: time.Now}
}
```

`present`'s store creates its directory and so returns an error:

```go
func New(dir string) (*Store, error) {
	s := &Store{dir: dir, now: time.Now}
	if err := os.MkdirAll(s.pagesDir(), 0o755); err != nil {
		return nil, fmt.Errorf("create pages dir: %w", err)
	}
	return s, nil
}
```

## Pure model and helpers split from the I/O shell

Keep the data model and pure functions in their own file, the file-touching methods in another (the functional-core/imperative-shell split — see `structure.md` and `safety.md`). `reminder/internal/store` separates `reminder.go` (the `Reminder` model + pure `NextDue`/`Overdue`) from `store.go` (the mutex-guarded JSON store) and `id.go` (id minting). Unexported path helpers stay on the store:

```go
func (s *Store) itemDir(key string) string    { return filepath.Join(s.Root, key) }
func (s *Store) contextPath(key string) string { return filepath.Join(s.itemDir(key), "CONTEXT.md") }
```

## Sentinels for "not found", not bare errors

Expose a package-prefixed `ErrNotFound` so the HTTP/CLI boundary can map it (see `errors.md` and `http.md`):

```go
var ErrNotFound = errors.New("present: page not found")
```

Treat a missing file as "none": `worklog`'s `List` returns `nil, nil` when the root doesn't exist (`os.IsNotExist`), rather than an error.

## Config roots and `~` expansion

Resolve the root from an env override, then a default under `$HOME`. Never hardcode `/Users/<name>`:

```go
// DefaultRoot returns $WORKLOG_DIR, or ~/code/worklog.
func DefaultRoot() string {
	if d := os.Getenv("WORKLOG_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "code", "worklog")
}
```

When a path comes from config or env that the shell didn't expand, expand a leading `~` in Go at load time (`present`'s `expandTilde`). This is the repo-wide path convention.

## Serialization formats by source

- **TOML** (`BurntSushi/toml`) for human-edited config: `d-man`'s `routes.toml`, `status`.
- **JSON** for machine state: the `reminder` store, the `present` `meta.json`.
- **YAML frontmatter** (`gopkg.in/yaml.v3`) for markdown documents: `worklog`'s `CONTEXT.md` `Frontmatter`.

Define a struct with explicit tags and document the contract:

```go
// Frontmatter is the CONTEXT.md contract: the machine-readable metadata that
// makes an item findable by ticket, topic, status, or repo.
type Frontmatter struct {
	Key     string    `yaml:"key"`
	Ticket  string    `yaml:"ticket,omitempty"`
	Status  string    `yaml:"status"`
	Created time.Time `yaml:"created"`
	Repos   []string  `yaml:"repos,omitempty"`
}
```

## Single writer for shared state

When more than one process touches the same store, make one process the only writer and have the rest go through it (an HTTP client — see `http.md`). `reminder serve` owns its JSON store behind a mutex; the MCP and CLI are HTTP clients, so there are no file-lock races. Prefer this to file locking.

## Updates take a `Patch`

Mutations take a `Patch` with pointer fields so "omitted" differs from "cleared" (see `functions.md`). `Update(id, patch)` applies only the non-nil fields and bumps a version.
