package record

import (
	"encoding/json"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// toolBash is the shell tool. With the harness setting bashEditDiffEnabled
// on, its PostToolUse tool_response carries bashEditDiff: the files the
// command changed under the repository, up to five with their hunks in
// files, and every one by absolute path in changedFiles. The harness calls
// the list best effort and its shape public beta, so every field is read
// on its own and one of the wrong type reads as absent.
const toolBash = "Bash"

// bashTargets lists one target per file the command's diff names: the files
// with hunks first, in the harness's order, then every other changed path
// by path alone. A Bash call without a diff names none.
func (rec *recorder) bashTargets() []target {
	diff := rec.p.response.obj("bashEditDiff")
	if diff == nil {
		return nil
	}
	var out []target
	seen := map[string]bool{}
	for _, f := range objects(diff.array("files")) {
		name := f.str("filePath")
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, target{name: name, kind: bashKind(f), hunks: f.array("hunks")})
	}
	for _, name := range strs(diff.array("changedFiles")) {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, target{name: name, kind: store.EditUpdate, pathOnly: true})
	}
	return out
}

// bashKind is what the diff says the command did to one file.
func bashKind(f fields) store.EditKind {
	switch {
	case f.flag("created"):
		return store.EditCreate
	case f.flag("deleted"):
		return store.EditDelete
	default:
		return store.EditUpdate
	}
}

// objects decodes the elements of a JSON array that are objects, one level
// deep; nil for a value that is not an array.
func objects(raw json.RawMessage) []fields {
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return nil
	}
	var out []fields
	for _, item := range items {
		var f fields
		if json.Unmarshal(item, &f) == nil && f != nil {
			out = append(out, f)
		}
	}
	return out
}

// strs decodes the elements of a JSON array that are strings.
func strs(raw json.RawMessage) []string {
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return nil
	}
	var out []string
	for _, item := range items {
		var s string
		if json.Unmarshal(item, &s) == nil {
			out = append(out, s)
		}
	}
	return out
}
