// Package mcptool reads Claude Code MCP tool names. A tool name has the
// shape mcp__<server>__<operation>; the server segment says which MCP
// server answers the call and the operation segment says what it does.
// The guard and hint packages both need the same reading of "does this
// call publish text", so the rule lives here rather than in either.
package mcptool

import "strings"

// prefix opens every MCP tool name.
const prefix = "mcp__"

// readOnlyVerbs name operations that fetch rather than publish. The consuming
// repo's hook matcher decides which MCP tools reach belt at all
// (docs/adr/0006); this list is what keeps a deliberately broad matcher, a
// whole server rather than named tools, from acting on plain reads.
var readOnlyVerbs = []string{"get", "list", "search", "read", "fetch"}

// Split returns the server and operation segments of an MCP tool name.
// ok is false for anything that is not an MCP tool name.
func Split(tool string) (server, operation string, ok bool) {
	rest, found := strings.CutPrefix(tool, prefix)
	if !found {
		return "", "", false
	}
	// mcp__<server>__<operation>: the server segment may itself contain
	// "__", so the operation is whatever follows the last separator.
	i := strings.LastIndex(rest, "__")
	if i < 0 {
		return "", "", false
	}
	return rest[:i], rest[i+2:], true
}

// Publishes reports whether a tool name looks like an MCP call that puts
// text somewhere other people read. Non-MCP tools never publish; an MCP
// operation whose name starts or ends with a read verb (get_file_contents,
// issue_read) is a fetch.
func Publishes(tool string) bool {
	_, op, ok := Split(tool)
	if !ok {
		return false
	}
	op = strings.ToLower(op)
	for _, verb := range readOnlyVerbs {
		// Servers put the verb at either end (get_file_contents, issue_read),
		// so both orders have to be recognised.
		if op == verb || strings.HasPrefix(op, verb+"_") || strings.HasSuffix(op, "_"+verb) {
			return false
		}
	}
	return true
}
