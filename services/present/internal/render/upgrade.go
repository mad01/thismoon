package render

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// UpgradeLegacyHTML rewrites a stored content.html fragment from the pre-webkit
// markup (bespoke CSS classes like .section/.panel/.chip) to the webkit custom
// elements (<wk-section>, <wk-panel>, <wk-badge>, …) emitted by the current Doc
// renderer. The migration that introduced the new markup was a 1:1 structural
// rename, so this transform is deterministic.
//
// It parses the fragment into a node tree, rewrites nodes by their class, and
// re-serializes. It is idempotent: a fragment already in wk-* form (or one with
// none of the legacy classes) passes through unchanged. Blocks present-specific
// chrome keeps untouched: .brief-title/.brief-meta/.brief-summary/.chip-row,
// the Cytoscape .cy-* containers, and the .refs-list/.refs-item/.refs-url
// references list.
func UpgradeLegacyHTML(in string) string {
	nodes, err := html.ParseFragment(strings.NewReader(in), &html.Node{
		Type:     html.ElementNode,
		Data:     "body",
		DataAtom: atom.Body,
	})
	if err != nil {
		// Parsing a fragment that the renderer produced should not fail; if it
		// somehow does, leave the input untouched rather than corrupting it.
		return in
	}

	root := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
	for _, n := range nodes {
		root.AppendChild(n)
	}

	upgradeNode(root)
	wrapKVRows(root)

	var b strings.Builder
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		if err := html.Render(&b, c); err != nil {
			return in
		}
	}
	return b.String()
}

// upgradeNode walks the tree depth-first and rewrites each element in place by
// its class. Children are processed first so wrapper rewrites see final markup.
func upgradeNode(n *html.Node) {
	// Capture each child's next sibling before recursing: rewriting a child can
	// move it into a new wrapper (e.g. data-table → wk-table), which rewrites
	// its NextSibling and would otherwise make the walk skip its real siblings.
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		upgradeNode(c)
		c = next
	}
	if n.Type != html.ElementNode {
		return
	}
	classes := classSet(n)

	switch {
	case classes["refs-section"]:
		// <div class="section refs-section"> → <wk-section class="refs-section">.
		// Keep the refs-section class; drop only "section".
		renameElement(n, "wk-section")
		setClass(n, removeClasses(n, "section"))

	case classes["section"]:
		renameElement(n, "wk-section")
		dropClassAttr(n)

	case classes["section-heading"]:
		renameElement(n, "wk-section-heading")
		dropClassAttr(n)

	case classes["section-id"]:
		renameElement(n, "wk-section-id")
		dropClassAttr(n)

	case classes["section-subheading"]:
		renameElement(n, "wk-section-subheading")
		dropClassAttr(n)

	case classes["section-text"]:
		// Drop the class, keep the tag (<p>) and its data-bionic attribute.
		setClass(n, removeClasses(n, "section-text"))

	case classes["toc"]:
		renameElement(n, "wk-toc")
		dropClassAttr(n)

	case classes["toc-title"]:
		renameElement(n, "wk-toc-title")
		dropClassAttr(n)

	case classes["toc-list"]:
		// <ul class="toc-list"> → plain <ul>.
		setClass(n, removeClasses(n, "toc-list"))

	case classes["kv-row"]:
		renameElement(n, "wk-kv-row")
		dropClassAttr(n)

	case classes["kv-label"], classes["kv-key"]:
		// kv-label is the spec name; kv-key is an older pre-spec variant.
		renameElement(n, "wk-kv-label")
		dropClassAttr(n)

	case classes["kv-value"], classes["kv-val"]:
		// kv-value is the spec name; kv-val is an older pre-spec variant.
		renameElement(n, "wk-kv-value")
		dropClassAttr(n)

	case classes["kv"]:
		// Older pre-spec variant wrapped each row's label/value spans in an inner
		// <div class="kv">. The modern markup has no such wrapper, so unwrap it:
		// splice its children into its place.
		unwrapElement(n)

	case classes["progress-inline"]:
		renameElement(n, "wk-progress")
		dropClassAttr(n)

	case classes["progress-bar"]:
		renameElement(n, "wk-progress-bar")
		dropClassAttr(n)

	case classes["progress-fill"]:
		renameElement(n, "wk-progress-fill")
		dropClassAttr(n)

	case classes["progress-label"]:
		renameElement(n, "wk-progress-label")
		dropClassAttr(n)

	case classes["callout"]:
		renameElement(n, "wk-callout")
		variant := calloutVariant(classes)
		dropClassAttr(n)
		if variant != "" {
			prependAttr(n, "variant", variant)
		}

	case classes["chip"]:
		renameElement(n, "wk-badge")
		variant := chipVariant(classes)
		dropClassAttr(n)
		if variant != "" {
			prependAttr(n, "variant", variant)
		}

	case classes["data-table"]:
		// <table class="data-table"> → <wk-table><table>…</table></wk-table>.
		dropClassAttr(n)
		wrapInElement(n, "wk-table")

	case classes["panel"]:
		renameElement(n, "wk-panel")
		dropClassAttr(n)

	case classes["panel-title"]:
		renameElement(n, "wk-panel-title")
		dropClassAttr(n)

	case classes["panel-subtitle"]:
		renameElement(n, "wk-panel-subtitle")
		dropClassAttr(n)
	}
}

// wrapKVRows groups runs of consecutive <wk-kv-row> siblings under a single
// <wk-kv> parent. The legacy markup emitted bare kv-rows with no wrapper; the
// new renderer wraps each kv block's rows in one <wk-kv>.
func wrapKVRows(n *html.Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		if c.Type == html.ElementNode && c.Data == "wk-kv-row" && !isKVChild(c) {
			c = collapseKVRun(n, c)
			next = c.NextSibling
		} else {
			wrapKVRows(c)
		}
		c = next
	}
}

// isKVChild reports whether n already sits inside a <wk-kv> wrapper.
func isKVChild(n *html.Node) bool {
	return n.Parent != nil && n.Parent.Type == html.ElementNode && n.Parent.Data == "wk-kv"
}

// collapseKVRun moves the run of consecutive wk-kv-row siblings starting at
// first into a new <wk-kv> element inserted at their former position. It returns
// the inserted wk-kv node so the caller can continue iteration after it.
func collapseKVRun(parent, first *html.Node) *html.Node {
	wrapper := &html.Node{Type: html.ElementNode, Data: "wk-kv"}
	parent.InsertBefore(wrapper, first)

	for c := first; c != nil; {
		next := c.NextSibling
		// Skip whitespace text between rows but keep collecting rows.
		if c.Type == html.ElementNode && c.Data == "wk-kv-row" {
			parent.RemoveChild(c)
			wrapper.AppendChild(c)
		} else if c.Type == html.TextNode && strings.TrimSpace(c.Data) == "" {
			parent.RemoveChild(c)
		} else {
			break
		}
		c = next
	}
	return wrapper
}

// ── element helpers ──

// classSet returns the element's class tokens as a set.
func classSet(n *html.Node) map[string]bool {
	set := map[string]bool{}
	for _, a := range n.Attr {
		if a.Key == "class" {
			for _, c := range strings.Fields(a.Val) {
				set[c] = true
			}
		}
	}
	return set
}

// renameElement changes the tag name, clearing the parsed atom so the custom
// element name serializes verbatim.
func renameElement(n *html.Node, name string) {
	n.Data = name
	n.DataAtom = 0
}

// dropClassAttr removes the class attribute entirely.
func dropClassAttr(n *html.Node) {
	out := n.Attr[:0]
	for _, a := range n.Attr {
		if a.Key == "class" {
			continue
		}
		out = append(out, a)
	}
	n.Attr = out
}

// setClass rewrites the class attribute in place to the given tokens, keeping
// the attribute's original position among the element's attributes. An empty
// token list removes the attribute.
func setClass(n *html.Node, tokens []string) {
	if len(tokens) == 0 {
		dropClassAttr(n)
		return
	}
	for i := range n.Attr {
		if n.Attr[i].Key == "class" {
			n.Attr[i].Val = strings.Join(tokens, " ")
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: "class", Val: strings.Join(tokens, " ")})
}

// removeClasses returns the element's class tokens minus the given ones,
// preserving their original left-to-right order.
func removeClasses(n *html.Node, drop ...string) []string {
	dropSet := map[string]bool{}
	for _, d := range drop {
		dropSet[d] = true
	}
	var kept []string
	for _, a := range n.Attr {
		if a.Key != "class" {
			continue
		}
		for _, c := range strings.Fields(a.Val) {
			if !dropSet[c] {
				kept = append(kept, c)
			}
		}
	}
	return kept
}

// prependAttr inserts an attribute at the front of the element's attribute list
// so serialized output reads `variant="…" …` to match the renderer's ordering.
func prependAttr(n *html.Node, key, val string) {
	n.Attr = append([]html.Attribute{{Key: key, Val: val}}, n.Attr...)
}

// unwrapElement removes n from the tree, splicing its children into its former
// position among its siblings (preserving order).
func unwrapElement(n *html.Node) {
	parent := n.Parent
	if parent == nil {
		return
	}
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		n.RemoveChild(c)
		parent.InsertBefore(c, n)
		c = next
	}
	parent.RemoveChild(n)
}

// wrapInElement wraps n in a new element with the given tag, taking n's place
// among its siblings.
func wrapInElement(n *html.Node, tag string) {
	parent := n.Parent
	if parent == nil {
		return
	}
	wrapper := &html.Node{Type: html.ElementNode, Data: tag}
	parent.InsertBefore(wrapper, n)
	parent.RemoveChild(n)
	wrapper.AppendChild(n)
}

// calloutVariant maps the legacy callout-* modifier class to a wk-callout
// variant. A plain .callout has no variant.
func calloutVariant(classes map[string]bool) string {
	switch {
	case classes["callout-info"]:
		return "info"
	case classes["callout-warn"]:
		return "warn"
	default:
		return ""
	}
}

// chipVariant maps the legacy chip-* modifier class to a wk-badge variant.
func chipVariant(classes map[string]bool) string {
	for _, v := range []string{"a", "b", "c", "outline", "stat"} {
		if classes["chip-"+v] {
			return v
		}
	}
	return ""
}
