# Testing

Stdlib `testing`, table-driven, hermetic. Backed by [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments) and the [Google Go Style Guide](https://google.github.io/styleguide/go/best-practices). The codebase uses **no testify** — stdlib only.

## Hermetic by construction

A test must never touch the real `$HOME`, the wall clock, or the network. Two tools make that automatic:

- **`t.TempDir()`** for any filesystem work — it is created per-test and cleaned up.
- **An injected `now func() time.Time`** on stores and servers, so time is deterministic. Every store/server in the repo has a `now` field set to `time.Now` in production and overridden in tests.

## Helpers with `t.Helper()`

Extract setup into a helper that calls `t.Helper()` so a failure points at the caller, not the helper. From thismoon `services/d-man/internal/config/config_test.go`:

```go
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "routes.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
```

## Table-driven with `t.Run` subtests

Drive cases from a slice or map of structs, one subtest each. Use `t.Fatalf` for setup that must succeed, `t.Errorf` for assertions so one failing row doesn't stop the rest.

```go
func TestParseRepeat(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want time.Duration
	}{
		{"daily", "daily", 24 * time.Hour},
		{"duration", "90m", 90 * time.Minute},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseRepeat(tc.in)
			if err != nil {
				t.Fatalf("ParseRepeat(%q): %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("ParseRepeat(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
```

## Failure messages: `got`, then `want`

Format messages as `Func(input) = got, want want`, with the actual value before the expected. Repo style: `t.Errorf("suffix = %q, want %q", c.Suffix, DefaultSuffix)`. Quote strings with `%q` so empties and whitespace are visible.

## HTTP handlers with httptest

Test handlers with `net/http/httptest` (a recorder, or a server for an end-to-end round trip) rather than a live port. `d-man` and `speak` do this. See `http.md` for the handler shape under test.

## What to test

- Cover new code with unit tests; the larger tools (`present`, `humanizer`, `d-man`, `reminder`) carry real suites.
- Test behavior through the public surface, not private internals, so the tests survive refactors.
- Tests live beside source as `*_test.go`.

## Optional uplifts (not current house default)

No benchmarks, fuzz tests, or `-race` in the test target today. Add them when the code warrants — a benchmark when optimizing, fuzzing for a parser, `-race` once a tool grows real concurrency. These belong to `golang-pro`'s testing reference for depth.
