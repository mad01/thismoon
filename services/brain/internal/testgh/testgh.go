// Package testgh puts a fake gh on PATH for a test. The fake records the
// arguments of every call and answers pr create, pr view and issue create
// with canned output, so no test reaches GitHub or runs the real gh. Tests
// import it; the binary never does.
package testgh

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// script is the fake gh. It saves its arguments, NUL-separated, to the next
// numbered file under calls/, then prints the canned answer for the
// subcommand. pr view answers from views/<last path element of the URL>.
const script = `#!/bin/sh
dir=$(dirname "$0")
mkdir -p "$dir/calls"
n=$(ls "$dir/calls" | wc -l | tr -d ' ')
printf '%s\0' "$@" > "$dir/calls/$(printf '%04d' "$n")"
case "$1 $2" in
"pr create") cat "$dir/pr-create.out" ;;
"issue create") cat "$dir/issue-create.out" ;;
"pr view")
	view="$dir/views/$(basename "$3")"
	if [ ! -f "$view" ]; then echo "fake gh: no view for $3" >&2; exit 1; fi
	cat "$view" ;;
*) echo "fake gh: unexpected call: $*" >&2; exit 1 ;;
esac
`

// Fake is a fake gh installed for one test.
type Fake struct {
	t   testing.TB
	dir string
}

// Install writes the fake gh into a temp dir and puts that dir first on
// PATH. pr create prints prURL and issue create prints issueURL until the
// test changes them. It uses t.Setenv, so the caller cannot run in
// parallel.
func Install(t testing.TB, prURL, issueURL string) *Fake {
	t.Helper()
	f := &Fake{t: t, dir: t.TempDir()}
	f.write("gh", script, 0o755)
	f.write("pr-create.out", prURL+"\n", 0o644)
	f.write("issue-create.out", issueURL+"\n", 0o644)
	t.Setenv("PATH", f.dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return f
}

// PRCreates makes the next pr create print url.
func (f *Fake) PRCreates(url string) { f.write("pr-create.out", url+"\n", 0o644) }

// PRState makes pr view of a URL ending in number answer state, with a
// mergedAt time when state is MERGED.
func (f *Fake) PRState(number, state string) {
	merged := "null"
	if state == "MERGED" {
		merged = `"2026-11-05T10:00:00Z"`
	}
	f.write(filepath.Join("views", number), `{"state":"`+state+`","mergedAt":`+merged+"}\n", 0o644)
}

// Calls returns the arguments of every call so far, oldest first.
func (f *Fake) Calls() [][]string {
	f.t.Helper()
	entries, err := os.ReadDir(filepath.Join(f.dir, "calls"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		f.t.Fatal(err)
	}
	var calls [][]string
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(f.dir, "calls", e.Name()))
		if err != nil {
			f.t.Fatal(err)
		}
		var args []string
		for _, a := range bytes.Split(bytes.TrimSuffix(data, []byte{0}), []byte{0}) {
			args = append(args, string(a))
		}
		calls = append(calls, args)
	}
	return calls
}

// Flag returns the value after flag in args, or "" when it is absent.
func Flag(args []string, flag string) string {
	i := slices.Index(args, flag)
	if i < 0 || i+1 >= len(args) {
		return ""
	}
	return args[i+1]
}

func (f *Fake) write(rel, content string, mode os.FileMode) {
	f.t.Helper()
	path := filepath.Join(f.dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		f.t.Fatal(err)
	}
}
