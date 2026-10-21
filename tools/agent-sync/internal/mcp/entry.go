package mcp

import (
	"fmt"
	"strings"

	"github.com/mad01/thismoon/tools/agent-sync/internal/gate"
	"github.com/mad01/thismoon/tools/agent-sync/internal/pyjson"
	"github.com/mad01/thismoon/tools/agent-sync/internal/pypath"
)

// The keys of a servers.json entry this package reads. Any other key (a
// _comment, say) is kept for the hash and otherwise ignored.
const (
	fieldTransport     = "transport"
	fieldCommand       = "command"
	fieldArgs          = "args"
	fieldEnv           = "env"
	fieldURL           = "url"
	fieldHeaders       = "headers"
	fieldHeadersHelper = "headersHelper"
	fieldHosts         = "hosts"
	fieldProfiles      = "profiles"
)

// entry is one raw servers.json entry and where it came from.
//
// Entries are checked up front: a field of the wrong type is an error that
// names the file, the server and the field. sync_mcp.py crashed on most of
// those at the point of use, and coerced a few (a string args became a list
// of characters).
type entry struct {
	where string // "<file>: server '<name>'", the prefix of every error
	obj   *pyjson.Object
}

// gated reports whether the entry's hosts and profiles gates let it apply
// to m. Both must pass, and profiles is not read when the host gate fails.
func (e entry) gated(m gate.Machine) (bool, error) {
	hosts, err := e.strList(fieldHosts)
	if err != nil || !m.HostAllowed(hosts) {
		return false, err
	}
	profiles, err := e.strList(fieldProfiles)
	if err != nil {
		return false, err
	}
	return m.ProfileAllowed(profiles), nil
}

// parse returns the entry as the hash sees it (hosts and profiles dropped,
// ~ expanded in command, headersHelper and args) and as a typed server.
func (e entry) parse() (*pyjson.Object, server, error) {
	spec := pyjson.NewObject()
	for _, k := range e.obj.Keys() {
		if k != fieldHosts && k != fieldProfiles {
			v, _ := e.obj.Get(k)
			spec.Set(k, v)
		}
	}
	var s server
	var err error
	if s.transport, err = e.str(fieldTransport); err != nil {
		return nil, server{}, err
	}
	if s.transport == "" {
		s.transport = transportStdio
	}
	if s.command, err = e.expandedString(spec, fieldCommand); err != nil {
		return nil, server{}, err
	}
	if s.headersHelper, err = e.expandedString(spec, fieldHeadersHelper); err != nil {
		return nil, server{}, err
	}
	if s.args, err = e.expandedArgs(spec); err != nil {
		return nil, server{}, err
	}
	if s.url, err = e.str(fieldURL); err != nil {
		return nil, server{}, err
	}
	if s.env, err = e.scalarMap(fieldEnv); err != nil {
		return nil, server{}, err
	}
	if s.headers, err = e.scalarMap(fieldHeaders); err != nil {
		return nil, server{}, err
	}
	return spec, s, nil
}

// expandedString reads a string field, expands a leading ~ and stores the
// result back in spec.
func (e entry) expandedString(spec *pyjson.Object, key string) (string, error) {
	s, err := e.str(key)
	if err != nil || !strings.HasPrefix(s, "~") {
		return s, err
	}
	if s, err = pypath.ExpandUser(s); err != nil {
		return "", fmt.Errorf("%s: %s: %w", e.where, key, err)
	}
	spec.Set(key, s)
	return s, nil
}

// expandedArgs reads args, expands a leading ~ in each and stores the
// result back in spec.
func (e entry) expandedArgs(spec *pyjson.Object) ([]string, error) {
	args, err := e.strList(fieldArgs)
	if err != nil || args == nil {
		return args, err
	}
	out := make([]any, len(args))
	for i, a := range args {
		if strings.HasPrefix(a, "~") {
			if a, err = pypath.ExpandUser(a); err != nil {
				return nil, fmt.Errorf("%s: %s[%d]: %w", e.where, fieldArgs, i, err)
			}
			args[i] = a
		}
		out[i] = a
	}
	spec.Set(fieldArgs, out)
	return args, nil
}

// str returns a string field, "" when it is absent or null.
func (e entry) str(key string) (string, error) {
	v, _ := e.obj.Get(key)
	switch v := v.(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	}
	return "", fmt.Errorf("%s: %s: want a string, got %s", e.where, key, pyjson.Describe(v))
}

// strList returns a list-of-strings field, nil when it is absent or null.
func (e entry) strList(key string) ([]string, error) {
	v, _ := e.obj.Get(key)
	if v == nil {
		return nil, nil
	}
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf(
			"%s: %s: want a list of strings, got %s",
			e.where,
			key,
			pyjson.Describe(v),
		)
	}
	out := make([]string, len(list))
	for i, item := range list {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s: %s[%d]: want a string, got %s",
				e.where, key, i, pyjson.Describe(item))
		}
		out[i] = s
	}
	return out, nil
}

// scalarMap returns an object field whose values are JSON scalars, nil when
// it is absent or null.
func (e entry) scalarMap(key string) (*pyjson.Object, error) {
	v, _ := e.obj.Get(key)
	if v == nil {
		return nil, nil
	}
	obj, ok := v.(*pyjson.Object)
	if !ok {
		return nil, fmt.Errorf("%s: %s: want an object, got %s", e.where, key, pyjson.Describe(v))
	}
	for _, k := range obj.Keys() {
		if val, _ := obj.Get(k); !isScalar(val) {
			return nil, fmt.Errorf("%s: %s.%s: want a string, number, bool or null, got %s",
				e.where, key, k, pyjson.Describe(val))
		}
	}
	return obj, nil
}

// isScalar reports whether v is a JSON string, number, bool or null.
func isScalar(v any) bool {
	switch v.(type) {
	case []any, *pyjson.Object:
		return false
	}
	return true
}
