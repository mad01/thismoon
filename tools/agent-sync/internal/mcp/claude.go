package mcp

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/mad01/thismoon/tools/agent-sync/internal/apply"
	"github.com/mad01/thismoon/tools/agent-sync/internal/claudecode"
	"github.com/mad01/thismoon/tools/agent-sync/internal/pyjson"
	"github.com/mad01/thismoon/tools/agent-sync/internal/pypath"
)

// scope is the claude mcp scope every server is added to and removed from.
const scope = "user"

// Name prefixes `claude mcp list` gives servers that no user-scope remove
// can reach: a claude.ai connector and a server a plugin provides
// (plugin:<plugin>:<server>).
const (
	connectorPrefix = "claude.ai "
	pluginPrefix    = "plugin:"
)

// listFooterHeading starts the diagnostics footer `claude mcp list` prints
// below its entries.
const listFooterHeading = "MCP config diagnostics"

// claudeCLI runs the claude CLI found on PATH, in dir when it is set.
type claudeCLI struct{ path, dir string }

// findClaude looks claude up on PATH; a missing one is
// claudecode.ErrNotFound.
func findClaude() (claudeCLI, error) {
	path, err := claudecode.LookPath()
	if err != nil {
		return claudeCLI{}, err
	}
	return claudeCLI{path: path}, nil
}

// openClaude finds claude and gives it an empty working directory, which
// close removes. list and get merge local scope (the working directory's
// entry in ~/.claude.json) and project scope (its .mcp.json) over user
// scope, so run from the caller's directory they would show servers no
// user-scope add or remove reaches and hide the user-scope copy of a name
// defined there too. An empty directory has neither.
func openClaude() (claudeCLI, error) {
	c, err := findClaude()
	if err != nil {
		return claudeCLI{}, err
	}
	if c.dir, err = os.MkdirTemp("", "agent-sync-mcp-"); err != nil {
		return claudeCLI{}, fmt.Errorf("make a working directory for claude: %w", err)
	}
	return c, nil
}

// close removes the working directory openClaude made; a failure is a
// warning.
func (c claudeCLI) close(warn io.Writer) {
	if err := os.RemoveAll(c.dir); err != nil {
		apply.Warn(warn, "could not remove %s: %s", c.dir, pypath.ErrorText(err))
	}
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
	cmd.Dir = c.dir
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

// listing is what `claude mcp list` printed: names, the servers agent-sync
// manages, and invalid, the sorted names add would reject, which it leaves
// alone.
type listing struct {
	names   map[string]bool
	invalid []string
}

// list returns the servers `claude mcp list` prints, from every scope it
// shows. A failed list is apply.CodeExternal.
func (c claudeCLI) list() (listing, error) {
	out, err := c.run("mcp", "list")
	if err != nil {
		return listing{}, err
	}
	if !out.ok {
		return listing{}, apply.Fail(apply.CodeExternal,
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

// userScoped reports whether `claude mcp remove --scope user` can remove
// name. A claude.ai connector, a plugin's server and a server get places in
// another scope cannot be removed there. A name get cannot describe counts
// as user scope, so a remove that then fails still fails the run.
func (c claudeCLI) userScoped(name string) (bool, error) {
	if connectorOrPlugin(name) {
		return false, nil
	}
	r, _, err := c.get(name)
	if err != nil {
		return false, err
	}
	return !r.otherScope(), nil
}

// addableName matches the names `claude mcp add` accepts: in claude's
// words, letters, numbers, hyphens, and underscores.
var addableName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// parseList reads the server names in `claude mcp list` output. It skips
// lines without a colon, which the health-check banner and the no-servers
// line are, and stops at the first blank line after an entry or at the
// diagnostics heading, so the diagnostics claude prints below the entries
// (a scope conflict, say) are not read. A name is the text before the
// first ": " on its line, so a plugin's plugin:<plugin>:<server> keeps its
// colons; a line without one is cut at its first colon. A name add would
// reject goes in invalid, unless it is a claude.ai connector's or a
// plugin's.
func parseList(stdout string) listing {
	l := listing{names: map[string]bool{}}
	inEntries := false
	for _, line := range pyjson.SplitLines(stdout) {
		line = pyjson.Strip(line)
		if (line == "" && inEntries) || strings.HasPrefix(line, listFooterHeading) {
			break
		}
		if !strings.Contains(line, ":") {
			continue
		}
		inEntries = true
		name, _, found := strings.Cut(line, ": ")
		if !found {
			name, _, _ = strings.Cut(line, ":")
		}
		name = pyjson.Strip(name)
		if addableName.MatchString(name) || connectorOrPlugin(name) {
			l.names[name] = true
		} else {
			l.invalid = append(l.invalid, name)
		}
	}
	sort.Strings(l.invalid)
	return l
}

// connectorOrPlugin reports whether list named a claude.ai connector or a
// plugin's server.
func connectorOrPlugin(name string) bool {
	return strings.HasPrefix(name, connectorPrefix) || strings.HasPrefix(name, pluginPrefix)
}

// registered is a server as `claude mcp get` describes it.
type registered struct {
	// scope is the Scope line's first word, lowercased ("user", "local",
	// "project"), or "" when get printed none.
	scope                   string
	transport, command, url string
	// args is the Args line as printed: the arguments joined by spaces,
	// which cannot be split back apart where an argument holds a space.
	args string
	env  map[string]string
}

// parseGet reads the Scope, Type, URL, Command, Args and Environment lines
// of `claude mcp get` output. A bare "Label:" line opens a block of the
// lines indented below it, which are never read as labels: an Environment
// block holds one K=v per line, and any other block (Headers, say) is
// skipped, so a header named URL cannot pass for the URL line. Environment
// may also come as "K=v, K=v" on its own line, the other form claude has
// printed. ok is false when get printed neither a command nor a url.
func parseGet(stdout string) (r registered, ok bool) {
	r = registered{transport: transportStdio, env: map[string]string{}}
	block, blockIndent := "", -1 // the bare label and its indent while in its block
	for _, raw := range pyjson.SplitLines(stdout) {
		line := pyjson.Strip(raw)
		if blockIndent >= 0 {
			if line != "" && indent(raw) > blockIndent {
				if block == "Environment" {
					addEnv(r.env, line)
				}
				continue
			}
			blockIndent = -1
		}
		label, rest, found := strings.Cut(line, ":")
		rest = pyjson.Strip(rest)
		// The server's name line is the one bare label at indent 0.
		if found && rest == "" && indent(raw) > 0 {
			block, blockIndent = label, indent(raw)
			continue
		}
		switch label {
		case "Scope":
			if words := pyjson.Fields(rest); len(words) > 0 {
				r.scope = strings.ToLower(words[0])
			}
		case "Type":
			r.transport = rest
		case "URL":
			r.url = rest
		case "Command":
			r.command = rest
		case "Args":
			r.args = rest
		case "Environment":
			for _, kv := range strings.Split(rest, ",") {
				addEnv(r.env, kv)
			}
		}
	}
	return r, r.command != "" || r.url != ""
}

// addEnv records one "K=v" of get's Environment output in env; text
// without an "=" is skipped.
func addEnv(env map[string]string, kv string) {
	if k, v, ok := strings.Cut(kv, "="); ok {
		env[pyjson.Strip(k)] = pyjson.Strip(v)
	}
}

// indent returns the length of line's leading whitespace.
func indent(line string) int {
	return len(line) - len(strings.TrimLeftFunc(line, unicode.IsSpace))
}

// otherScope reports whether get placed the server in a scope other than
// user, which hides any user-scope copy from get. From the empty directory
// openClaude makes, that leaves an enterprise or managed server.
func (r registered) otherScope() bool { return r.scope != "" && r.scope != scope }

// matches reports whether the registered server is the desired one. A
// remote server matches on transport and url alone, though get prints its
// headers too: re-adding an OAuth server would drop its token. Args and
// env compare in the form get prints them.
func (r registered) matches(s server) bool {
	if s.transport != r.transport {
		return false
	}
	if s.remote() {
		return s.url == r.url
	}
	return s.command == r.command &&
		pyjson.Strip(strings.Join(s.args, " ")) == r.args &&
		envEqual(s.env, r.env)
}

// envEqual compares desired env values with the strings get printed, each
// value rendered as add passes it to -e (Python's str(), so 3 matches "3").
func envEqual(want *pyjson.Object, got map[string]string) bool {
	if want == nil {
		return len(got) == 0
	}
	if want.Len() != len(got) {
		return false
	}
	for _, k := range want.Keys() {
		g, ok := got[k]
		if v, _ := want.Get(k); !ok || pyjson.Str(v) != g {
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
// carries one; every other server through add. -e and --header are
// variadic and would swallow the name, so env flags go after the name and
// header flags after the url. A remote server without a url, or another
// without a command, is an error.
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
		args = appendPairs(append(args, name, s.url), "--header", s.headers, ": ")
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
