package lint

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mad01/thismoon/services/present/internal/render"
)

// ParseArgs reads a present_create argument object into a Page: its title,
// and its content and deck, each either a Doc JSON string the way the tool
// takes it or the Doc object itself. A content string that is not a Doc is
// legacy HTML, which the lint cannot read, so that rendition stays empty.
// Nothing is compiled: the lint judges the Doc as written, and the renderer
// still refuses at create time what it refuses.
func ParseArgs(data []byte) (Page, error) {
	var raw struct {
		Title   string          `json:"title"`
		Content json.RawMessage `json:"content"`
		Deck    json.RawMessage `json:"deck"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Page{}, fmt.Errorf("lint: read arguments: %w", err)
	}
	p := Page{Title: raw.Title}
	var err error
	if p.Brief, err = docArg(raw.Content); err != nil {
		return Page{}, fmt.Errorf("lint: content: %w", err)
	}
	if p.Deck, err = docArg(raw.Deck); err != nil {
		return Page{}, fmt.Errorf("lint: deck: %w", err)
	}
	return p, nil
}

// docArg reads one Doc argument: absent or empty, a JSON string holding a
// Doc or legacy HTML, or a Doc object.
func docArg(raw json.RawMessage) (render.Doc, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return render.Doc{}, nil
	}
	switch raw[0] {
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return render.Doc{}, err
		}
		return docSource([]byte(s))
	case '{':
		return docSource(raw)
	}
	return render.Doc{}, errors.New("want a Doc JSON string or object")
}

// docSource reads Doc JSON. Anything that does not open as a JSON object
// is legacy HTML and reads as no Doc.
func docSource(data []byte) (render.Doc, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || data[0] != '{' {
		return render.Doc{}, nil
	}
	var d render.Doc
	if err := json.Unmarshal(data, &d); err != nil {
		return render.Doc{}, fmt.Errorf("parse doc: %w", err)
	}
	return d, nil
}

// FromSources builds a Page from a stored page's title and its Doc
// sources, the doc.json and deck.json bytes; nil or empty means the page
// has no Doc for that rendition.
func FromSources(title string, doc, deck []byte) (Page, error) {
	p := Page{Title: title}
	var err error
	if p.Brief, err = docSource(doc); err != nil {
		return Page{}, fmt.Errorf("lint: content: %w", err)
	}
	if p.Deck, err = docSource(deck); err != nil {
		return Page{}, fmt.Errorf("lint: deck: %w", err)
	}
	return p, nil
}
