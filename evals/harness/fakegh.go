package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// fakeGHRepo is the repository the fake gh says it opened pull requests
// and issues on. Nothing is ever sent there.
const fakeGHRepo = "https://github.com/bramblewick-freight/team-vault"

// fakeGHScript stands in for gh in a brain workspace, so a brain command
// that opens a pull request or an issue never reaches GitHub. Each call is
// one line of $HARNESS_EVAL_GH_LOG: every argument followed by a unit
// separator (\037), with a newline inside an argument written as a record
// separator (\036). pr create and issue create print a URL numbered by the
// calls before them, pr view prints an open pull request, and anything else
// fails.
const fakeGHScript = `#!/bin/sh
log=${` + ghLogEnv + `:-/dev/null}
printf '%s\037' "$@" | tr '\n' '\036' >>"$log"
printf '\n' >>"$log"
us=$(printf '\037')
calls() { grep -c "^$1$us$2$us" "$log"; }
repo=` + fakeGHRepo + `
case "$1 $2" in
"pr create") echo "$repo/pull/$((100 + $(calls pr create)))" ;;
"pr view") echo '{"state":"OPEN","mergedAt":null}' ;;
"issue create") echo "$repo/issues/$((200 + $(calls issue create)))" ;;
"auth status") echo "github.com: logged in as harness-eval (fake gh)" >&2 ;;
"--version "* | "version "*) echo "gh version 0.0.0-harness-eval (fake)" ;;
*)
	echo "fake gh: gh $* is not faked" >&2
	exit 1
	;;
esac
`

// writeFakeGH puts the fake gh in dir, which a brain workspace puts first
// on PATH.
func writeFakeGH(dir string) error {
	return os.WriteFile(filepath.Join(dir, "gh"), []byte(fakeGHScript), 0o755)
}

// readGHLog reads the fake gh's log at path, one call per entry with its
// arguments joined by spaces, or nil when gh never ran.
func readGHLog(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var calls []string
	for line := range strings.SplitSeq(string(data), "\n") {
		if line == "" {
			continue
		}
		args := strings.Split(strings.TrimSuffix(line, "\x1f"), "\x1f")
		calls = append(calls, strings.ReplaceAll(strings.Join(args, " "), "\x1e", "\n"))
	}
	return calls, nil
}
