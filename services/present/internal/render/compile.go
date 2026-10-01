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

// Compile parses Doc JSON and compiles it as a brief under title. It is the
// one step every brief write path runs, whether the Doc arrives from an MCP
// tool, from a stored doc.json being re-rendered, or from a converter.
func Compile(data []byte, title string) (Compiled, error) {
	return compile(data, title, RenderDoc)
}

// CompileDeck parses Doc JSON and compiles it as a deck under title: the
// same Doc, rendered with the deck chrome the brief ignores. Every deck
// write path runs it, the MCP tools and the re-render alike.
func CompileDeck(data []byte, title string) (Compiled, error) {
	return compile(data, title, RenderDeck)
}

func compile(
	data []byte,
	title string,
	render func(Doc, string) (string, error),
) (Compiled, error) {
	var doc Doc
	if err := json.Unmarshal(data, &doc); err != nil {
		return Compiled{}, fmt.Errorf("parse doc: %w", err)
	}
	return compileDoc(doc, title, render)
}

// CompileDoc renders d as a brief under title and returns the artifacts a
// page stores.
func CompileDoc(d Doc, title string) (Compiled, error) {
	return compileDoc(d, title, RenderDoc)
}

func compileDoc(d Doc, title string, render func(Doc, string) (string, error)) (Compiled, error) {
	out, err := render(d, title)
	if err != nil {
		return Compiled{}, err
	}
	canonical, err := json.Marshal(d)
	if err != nil {
		return Compiled{}, fmt.Errorf("marshal doc: %w", err)
	}
	return Compiled{Doc: d, HTML: out, JSON: canonical}, nil
}

// CompileGraph renders g to the Cytoscape init script a page stores and
// returns it with the canonical GraphInput JSON, re-marshaled from the
// parsed struct like CompileDoc does for a Doc.
func CompileGraph(g GraphInput) (js string, canonical []byte, err error) {
	js, err = RenderGraph(g)
	if err != nil {
		return "", nil, err
	}
	canonical, err = json.Marshal(g)
	if err != nil {
		return "", nil, fmt.Errorf("marshal graph: %w", err)
	}
	return js, canonical, nil
}
