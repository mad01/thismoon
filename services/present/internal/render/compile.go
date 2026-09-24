package render

import (
	"encoding/json"
	"fmt"
)

// Compiled is a Doc made ready for the store: the HTML fragment content.html
// holds, the canonical JSON doc.json holds, and the Doc both came from. The
// JSON is re-marshaled from the parsed struct, so unknown fields and
// formatting differences in the input do not reach disk.
type Compiled struct {
	Doc  Doc
	HTML string
	JSON []byte
}

// Compile parses Doc JSON and compiles it under title. It is the one step
// every write path runs, whether the Doc arrives from an MCP tool, from a
// stored doc.json being re-rendered, or from a converter.
func Compile(data []byte, title string) (Compiled, error) {
	var doc Doc
	if err := json.Unmarshal(data, &doc); err != nil {
		return Compiled{}, fmt.Errorf("parse doc: %w", err)
	}
	return CompileDoc(doc, title)
}

// CompileDoc renders d under title and returns the artifacts a page stores.
func CompileDoc(d Doc, title string) (Compiled, error) {
	out, err := RenderDoc(d, title)
	if err != nil {
		return Compiled{}, err
	}
	canonical, err := json.Marshal(d)
	if err != nil {
		return Compiled{}, fmt.Errorf("marshal doc: %w", err)
	}
	return Compiled{Doc: d, HTML: out, JSON: canonical}, nil
}
