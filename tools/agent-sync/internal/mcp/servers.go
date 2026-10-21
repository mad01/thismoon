package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mad01/thismoon/tools/agent-sync/internal/apply"
	"github.com/mad01/thismoon/tools/agent-sync/internal/gate"
	"github.com/mad01/thismoon/tools/agent-sync/internal/pyjson"
	"github.com/mad01/thismoon/tools/agent-sync/internal/pypath"
)

// Transports that name a remote server: a url instead of a command.
const (
	transportStdio = "stdio"
	transportHTTP  = "http"
	transportSSE   = "sse"
)

// server is one desired server, its servers.json fields checked and typed.
type server struct {
	// transport is the entry's transport, transportStdio when it sets none.
	transport string
	command   string
	args      []string
	// env and headers keep the file's order; their values are JSON
	// scalars, put on the command line as Python's str() renders them.
	env     *pyjson.Object
	headers *pyjson.Object
	url     string
	// headersHelper is a command Claude Code runs to mint headers; a
	// remote server that has one is registered with add-json.
	headersHelper string
}

// remote reports whether s is registered by url.
func (s server) remote() bool {
	return s.transport == transportHTTP || s.transport == transportSSE
}

// desiredSet is the servers this machine should have registered.
type desiredSet struct {
	servers map[string]server
	// specs maps each name to its entry as the hash sees it: hosts and
	// profiles dropped, ~ expanded in command, headersHelper and args.
	specs *pyjson.Object
}

// names returns the desired names in sorted order.
func (d desiredSet) names() []string {
	names := d.specs.Keys()
	sort.Strings(names)
	return names
}

// hash returns the stamp value for d on host: the SHA-256 of
// json.dumps(specs, sort_keys=True), a newline and the host, as
// sync_mcp.py computed it.
func (d desiredSet) hash(host string) string {
	sum := sha256.Sum256([]byte(pyjson.Compact(pyjson.SortKeys(d.specs)) + "\n" + host))
	return hex.EncodeToString(sum[:])
}

// loadDesired reads the base servers file and every *.json drop-in in name
// order, drops the entries whose name starts with "_" and the ones gated
// off m, and checks the rest. A drop-in that redefines a server is warned
// about on warn.
func loadDesired(cfg Config, m gate.Machine, warn io.Writer) (desiredSet, error) {
	if !pathExists(cfg.Servers) {
		return desiredSet{}, fmt.Errorf("%s not found", cfg.Servers)
	}
	merged, err := readServersFile(cfg.Servers)
	if err != nil {
		return desiredSet{}, err
	}
	source := map[string]string{}
	for _, name := range merged.Keys() {
		source[name] = cfg.Servers
	}
	for _, path := range pypath.Glob(cfg.Dropins, "*.json") {
		dropin, err := readServersFile(path)
		if err != nil {
			return desiredSet{}, err
		}
		for _, name := range dropin.Keys() {
			if merged.Has(name) {
				apply.Warn(warn, "%s overrides server %s", filepath.Base(path), pyjson.Repr(name))
			}
			v, _ := dropin.Get(name)
			merged.Set(name, v)
			source[name] = path
		}
	}
	return selectDesired(merged, source, m)
}

// selectDesired applies the name, host and profile gates to the merged
// entries and checks the ones that pass. A name `claude mcp add` would
// reject is an error even in an entry gated off this machine, so prune's
// not-managed note never names a declared server.
func selectDesired(
	merged *pyjson.Object,
	source map[string]string,
	m gate.Machine,
) (desiredSet, error) {
	d := desiredSet{servers: map[string]server{}, specs: pyjson.NewObject()}
	for _, name := range merged.Keys() {
		if strings.HasPrefix(name, "_") {
			continue
		}
		v, _ := merged.Get(name)
		e := entry{where: fmt.Sprintf("%s: server %s", source[name], pyjson.Repr(name))}
		if !addableName.MatchString(name) {
			return desiredSet{}, fmt.Errorf("%s: name not valid for claude mcp add, "+
				"which takes letters, numbers, hyphens, and underscores", e.where)
		}
		obj, ok := v.(*pyjson.Object)
		if !ok {
			return desiredSet{}, fmt.Errorf(
				"%s: want an object, got %s",
				e.where,
				pyjson.Describe(v),
			)
		}
		e.obj = obj
		ok, err := e.gated(m)
		if err != nil {
			return desiredSet{}, err
		}
		if !ok {
			continue
		}
		spec, srv, err := e.parse()
		if err != nil {
			return desiredSet{}, err
		}
		d.specs.Set(name, spec)
		d.servers[name] = srv
	}
	return d, nil
}

// readServersFile reads one servers file and returns its servers object.
func readServersFile(path string) (*pyjson.Object, error) {
	v, err := pyjson.ReadFile(path)
	var syntax *pyjson.SyntaxError
	switch {
	case errors.As(err, &syntax):
		return nil, fmt.Errorf("%s is not valid JSON: %w", path, err)
	case err != nil:
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	doc, ok := v.(*pyjson.Object)
	if !ok {
		return nil, fmt.Errorf(
			"%s: want a JSON object at the top level, got %s",
			path,
			pyjson.Describe(v),
		)
	}
	servers, _ := doc.Get("servers")
	obj, ok := servers.(*pyjson.Object)
	if !ok {
		return nil, fmt.Errorf("%s has no 'servers' object", path)
	}
	return obj, nil
}

// pathExists reports whether path names something, following symlinks; any
// stat error counts as absent, as Python 3.13's Path.exists treats it.
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
