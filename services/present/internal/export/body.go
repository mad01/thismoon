package export

import (
	"fmt"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// wellFormed returns body parsed and serialized again, so that it closes
// everything it opens. A served page mounts the body through innerHTML,
// which keeps whatever the body holds inside it. An exported file places it
// in the middle of a document, where an html block or a legacy page could
// otherwise reach past it: an unclosed comment or element swallows the
// page data and the scripts after it. The parse uses the context the served
// page mounts into, a div, and has scripting off, so a noscript block is
// read as the markup a reader without scripts would see.
func wellFormed(body string) (string, error) {
	nodes, err := html.ParseFragmentWithOptions(
		strings.NewReader(body),
		&html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div},
		html.ParseOptionEnableScripting(false),
	)
	if err != nil {
		return "", fmt.Errorf("export: parse body: %w", err)
	}
	var b strings.Builder
	for _, n := range nodes {
		contain(n)
		if err := html.Render(&b, n); err != nil {
			return "", fmt.Errorf("export: serialize body: %w", err)
		}
	}
	return b.String(), nil
}

// scriptCommentOpen is "<!--" with its "<" written as the six-character
// Unicode escape, which a JS string, a JS regex, and a JSON string all read
// as "<". Built in two parts so the source shows the escape as written.
var scriptCommentOpen = `\` + `u003C!--`

// contain makes every script in the body inert (see inert) and rewrites the
// three elements whose serialized text can still change how a browser reads
// what follows them. A plaintext element runs to the end of the file, so it
// becomes the pre it looks like. A "<!--" in a script lets a later "<script"
// keep the closing tag from ending it, so it becomes scriptCommentOpen. The
// text of a noscript block is written as it is, and a reader with scripts
// off parses it as markup, so it is escaped first; with scripts on the
// browser never shows it.
func contain(n *html.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		contain(c)
	}
	if n.Type != html.ElementNode {
		return
	}
	if n.Data == "script" { // an SVG script runs in a parsed document too
		inert(n)
	}
	if n.Namespace != "" {
		return
	}
	switch n.DataAtom {
	case atom.Plaintext:
		n.Data, n.DataAtom = "pre", atom.Pre
	case atom.Script:
		eachText(
			n,
			func(s string) string { return strings.ReplaceAll(s, "<!--", scriptCommentOpen) },
		)
	case atom.Noscript:
		eachText(n, html.EscapeString)
	}
}

// jsonType is the one script type the renderer writes into a body: data
// app.js reads, which a browser never runs.
const jsonType = "application/json"

// inert gives a script the type text/plain, which a browser neither runs
// nor fetches, unless the script is a JSON island. A served page mounts the
// body through innerHTML, which never runs a script, so a script an html
// block carries runs in neither. The browser reads the first type attribute
// of a tag, so that is the one checked, and every one is replaced.
func inert(n *html.Node) {
	i := attrIndex(n, "type")
	if i >= 0 && strings.EqualFold(strings.TrimSpace(n.Attr[i].Val), jsonType) {
		return
	}
	attrs := []html.Attribute{{Key: "type", Val: "text/plain"}}
	for _, a := range n.Attr {
		if a.Namespace != "" || a.Key != "type" {
			attrs = append(attrs, a)
		}
	}
	n.Attr = attrs
}

// attrIndex returns the index of n's first attribute named key, or -1.
func attrIndex(n *html.Node, key string) int {
	for i, a := range n.Attr {
		if a.Namespace == "" && a.Key == key {
			return i
		}
	}
	return -1
}

// eachText replaces the data of every text child of n with fn of it.
func eachText(n *html.Node, fn func(string) string) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			c.Data = fn(c.Data)
		}
	}
}
