package mcp

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"

	"github.com/mad01/thismoon/tools/agent-sync/internal/apply"
	"github.com/mad01/thismoon/tools/agent-sync/internal/claudecode"
	"github.com/mad01/thismoon/tools/agent-sync/internal/pyjson"
)

// scope is the claude mcp scope every server is added to and removed from.
const scope = "user"

// claudeCLI runs the claude CLI found on PATH.
type claudeCLI struct{ path string }

// findClaude looks claude up on PATH; a missing one is
// claudecode.ErrNotFound.
func findClaude() (claudeCLI, error) {
	path, err := claudecode.LookPath()
	if err != nil {
		return claudeCLI{}, err
	}
	return claudeCLI{path: path}, nil
}

// output is one finished claude run, decoded as Python's text=True decodes.
type output struct {
	stdout, stderr string
	ok             bool // exit status 0
}

// run runs claude with args. A claude that exits non-zero is an output
// with ok false; one that cannot start is an error.
func (c claudeCLI) run(args ...string) (output, error) {
	cmd := exec.Command(c.path, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return output{}, fmt.Errorf("run %s: %w", c.path, err)
	}
	return output{
		stdout: pyjson.Text(stdout.Bytes()),
		stderr: pyjson.Text(stderr.Bytes()),
		ok:     err == nil,
	}, nil
}

// list returns the names `claude mcp list` prints, from every scope. A
// failed list is apply.CodeExternal.
func (c claudeCLI) list() (map[string]bool, error) {
	out, err := c.run("mcp", "list")
	if err != nil {
		return nil, err
	}
	if !out.ok {
		return nil, apply.Fail(apply.CodeExternal,
			fmt.Errorf("`claude mcp list` failed: %s", out.stderr))
	}
	return parseList(out.stdout), nil
}

// get returns the server `claude mcp get name` prints. ok is false when
// get fails or prints neither a command nor a url.
func (c claudeCLI) get(name string) (registered, bool, error) {
	out, err := c.run("mcp", "get", name)
	if err != nil || !out.ok {
		return registered{}, false, err
	}
	r, ok := parseGet(out.stdout)
	return r, ok, nil
}

// parseList returns the server names in `claude mcp list` output: the text
// before the first colon on each line, skipping the health-check banner
// and the no-servers line.
func parseList(stdout string) map[string]bool {
	names := map[string]bool{}
	for _, line := range pyjson.SplitLines(stdout) {
		line = pyjson.Strip(line)
		if !strings.Contains(line, ":") ||
			strings.HasPrefix(line, "Checking") || strings.HasPrefix(line, "No MCP") {
			continue
		}
		name, _, _ := strings.Cut(line, ":")
		if name = pyjson.Strip(name); name != "" {
			names[name] = true
		}
	}
	return names
}

// registered is a server as `claude mcp get` describes it.
type registered struct {
	transport, command, url string
	args                    []string
	env                     map[string]string
}

// parseGet reads the Type, URL, Command, Args and Environment lines of
// `claude mcp get` output. Args split at whitespace and Environment at
// commas, on the line itself: an argument holding a space, or an
// Environment block printed one variable per line, reads differently from
// servers.json and so counts as changed, as it did in sync_mcp.py.
func parseGet(stdout string) (registered, bool) {
	r := registered{transport: transportStdio, env: map[string]string{}}
	for _, line := range pyjson.SplitLines(stdout) {
		line = pyjson.Strip(line)
		label, rest, _ := strings.Cut(line, ":")
		rest = pyjson.Strip(rest)
		switch label {
		case "Type":
			r.transport = rest
		case "URL":
			r.url = rest
		case "Command":
			r.command = rest
		case "Args":
			r.args = pyjson.Fields(rest)
		case "Environment":
			for _, kv := range strings.Split(rest, ",") {
				if k, v, ok := strings.Cut(kv, "="); ok {
					r.env[pyjson.Strip(k)] = pyjson.Strip(v)
				}
			}
		}
	}
	if r.command == "" && r.url == "" {
		return registered{}, false
	}
	return r, true
}

// matches reports whether the registered server is the desired one. A
// remote server matches on transport and url alone: get prints no
// headers, and re-adding an OAuth server would drop its token.
func (r registered) matches(s server) bool {
	if s.transport != r.transport {
		return false
	}
	if s.remote() {
		return s.url == r.url
	}
	return s.command == r.command && slices.Equal(s.args, r.args) && envEqual(s.env, r.env)
}

// envEqual compares desired env values with the strings get printed, by
// Python ==: a number never equals its text.
func envEqual(want *pyjson.Object, got map[string]string) bool {
	if want == nil {
		return len(got) == 0
	}
	if want.Len() != len(got) {
		return false
	}
	for _, k := range want.Keys() {
		g, ok := got[k]
		if v, _ := want.Get(k); !ok || !pyjson.Equal(v, g) {
			return false
		}
	}
	return true
}

// invocation is one state-changing claude run: its arguments after the
// claude path and the text plan prints after "  $ ".
type invocation struct {
	args  []string
	shown string
}

// addInvocation builds the claude call that registers s as name. A remote
// server with a headersHelper goes through add-json, the only form that
// carries one; every other server through add, with env flags after the
// name because -e is variadic and would swallow it. A remote server without
// a url, or another without a command, is an error.
func (c claudeCLI) addInvocation(name string, s server) (invocation, error) {
	if s.remote() && s.headersHelper != "" {
		if s.url == "" {
			return invocation{}, missingURL(name, s)
		}
		blob := pyjson.NewObject()
		blob.Set("type", s.transport)
		blob.Set("url", s.url)
		blob.Set("headersHelper", s.headersHelper)
		if s.headers != nil && s.headers.Len() > 0 {
			blob.Set("headers", s.headers)
		}
		args := []string{"mcp", "add-json", "--scope", scope, name}
		return invocation{
			args:  append(args, pyjson.Compact(blob)),
			shown: c.show(args) + " <json>",
		}, nil
	}
	args := []string{"mcp", "add", "--scope", scope}
	if s.transport != transportStdio {
		args = append(args, "--transport", s.transport)
	}
	if s.remote() {
		if s.url == "" {
			return invocation{}, missingURL(name, s)
		}
		args = appendPairs(args, "--header", s.headers, ": ")
		args = append(args, name, s.url)
		return invocation{args: args, shown: c.show(args)}, nil
	}
	args = appendPairs(append(args, name), "-e", s.env, "=")
	if s.command == "" {
		return invocation{}, fmt.Errorf("server %s has no 'command'", pyjson.Repr(name))
	}
	args = append(append(args, "--", s.command), s.args...)
	return invocation{args: args, shown: c.show(args)}, nil
}

// removeInvocation builds the claude call that removes name.
func (c claudeCLI) removeInvocation(name string) invocation {
	args := []string{"mcp", "remove", "--scope", scope, name}
	return invocation{args: args, shown: c.show(args)}
}

// show joins the claude path and args with spaces, as the script printed a
// command: no quoting.
func (c claudeCLI) show(args []string) string {
	return strings.Join(append([]string{c.path}, args...), " ")
}

// change runs inv; a failure is apply.CodeExternal naming verb and server.
func (c claudeCLI) change(verb, name string, inv invocation) error {
	out, err := c.run(inv.args...)
	if err != nil {
		return err
	}
	if !out.ok {
		return apply.Fail(apply.CodeExternal,
			fmt.Errorf("%s %s failed: %s", verb, pyjson.Repr(name), out.stderr))
	}
	return nil
}

func missingURL(name string, s server) error {
	return fmt.Errorf("server %s (transport %s) has no 'url'", pyjson.Repr(name), s.transport)
}

// appendPairs appends flag and "<key><sep><value>" for each entry of obj,
// the value as Python's str() renders it (entry.scalarMap admits only
// scalars).
func appendPairs(args []string, flag string, obj *pyjson.Object, sep string) []string {
	if obj == nil {
		return args
	}
	for _, k := range obj.Keys() {
		v, _ := obj.Get(k)
		args = append(args, flag, k+sep+pyjson.Str(v))
	}
	return args
}
