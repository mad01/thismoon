// Package mermaid converts Mermaid flowchart source into the graph input
// present renders with Cytoscape, so a fenced mermaid block in imported
// markdown becomes the page's graph instead of a code block.
//
// Only what present can draw survives: node ids and labels, links, link
// labels, dotted links (drawn dashed), and subgraph membership (drawn as
// module border colours). Every link becomes one directed edge with an
// arrowhead, including open links (---) and bidirectional ones (<-->),
// because present draws a head on every edge. Node shapes, class and style
// statements, click handlers, accessibility text, YAML front matter, edge
// ids, and every key but label in a Mermaid v11 @{ } block are parsed past
// and dropped.
package mermaid

import (
	"errors"
	"fmt"
	"html"
	"regexp"
	"slices"
	"strings"

	"github.com/mad01/thismoon/services/present/internal/render"
)

// ErrNotFlowchart is returned, wrapped, when src does not open with a
// graph or flowchart header (a sequence diagram, a class diagram, prose).
var ErrNotFlowchart = errors.New("mermaid: not a flowchart")

var errNoNodes = errors.New("mermaid: flowchart has no nodes")

// maxNodes and maxEdges bound the graph Convert will build. A fenced block
// past either limit is not a page graph present can lay out, so Convert
// fails and the importer keeps it as a code block; the edge cap also stops
// `a&b&... --> c&d&...` fan-outs from multiplying without bound.
const (
	maxNodes = 2000
	maxEdges = 5000
)

var (
	errTooManyNodes = fmt.Errorf("mermaid: flowchart has more than %d nodes", maxNodes)
	errTooManyEdges = fmt.Errorf("mermaid: flowchart has more than %d edges", maxEdges)
)

// moduleColors is how many module border colours app.js's graph style defines
// (modBorder in render/graph.go); subgraph indexes wrap around it.
const moduleColors = 4

// snippetLen caps the statement text quoted in error messages.
const snippetLen = 60

// Convert parses src as a Mermaid flowchart and returns the graph.
func Convert(src string) (render.GraphInput, error) {
	stmts := split(src)
	if len(stmts) == 0 {
		return render.GraphInput{}, fmt.Errorf("%w: empty input", ErrNotFlowchart)
	}
	dir, err := parseHeader(stmts[0].text)
	if err != nil {
		return render.GraphInput{}, err
	}
	p := &parser{index: map[string]int{}, edgeIDs: map[string]bool{}}
	for _, st := range stmts[1:] {
		if err := p.statement(st); err != nil {
			return render.GraphInput{}, err
		}
	}
	if len(p.nodes) == 0 {
		return render.GraphInput{}, errNoNodes
	}
	return render.GraphInput{Nodes: p.nodes, Edges: p.edges, Direction: dir}, nil
}

// statement is one newline- or semicolon-delimited unit of the source with
// the 1-based line it came from, for error messages.
type statement struct {
	line int
	text string
}

// wrap turns a scanner reason into the error Convert reports for st. Size
// limit errors already say everything and pass through unchanged.
func (st statement) wrap(err error) error {
	if errors.Is(err, errTooManyNodes) || errors.Is(err, errTooManyEdges) {
		return err
	}
	return fmt.Errorf("mermaid: line %d: cannot parse %q: %w", st.line, snippet(st.text), err)
}

// split breaks src into statements. YAML front matter, blank lines,
// whole-line %% comments, and accTitle/accDescr lines are dropped; the rest
// is cut at newlines and at semicolons outside double quotes. A quoted
// string may span lines, so its newlines stay inside the statement. Line
// numbers count from the original source so errors point at the block the
// author sees.
func split(src string) []statement {
	lines := strings.Split(src, "\n")
	sp := &splitter{}
	for i := skipFrontMatter(lines); i < len(lines); i++ {
		if !sp.quoted {
			if strings.HasPrefix(strings.TrimSpace(lines[i]), "%%") {
				continue
			}
			if end := accessibilityEnd(lines, i); end >= 0 {
				i = end
				continue
			}
		}
		sp.feed(i+1, lines[i])
	}
	sp.flush()
	return sp.out
}

// skipFrontMatter returns the index of the first line after a leading YAML
// front-matter block (a --- line through the next --- line), or 0 when the
// source has none.
func skipFrontMatter(lines []string) int {
	first := 0
	for first < len(lines) && strings.TrimSpace(lines[first]) == "" {
		first++
	}
	if first == len(lines) || strings.TrimSpace(lines[first]) != "---" {
		return 0
	}
	for i := first + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return i + 1
		}
	}
	return len(lines)
}

// accessibilityEnd returns the index of the last line of an accTitle or
// accDescr statement that starts at lines[i], or -1 when the line is not
// one. The `accDescr { ... }` form runs to the line holding its }.
func accessibilityEnd(lines []string, i int) int {
	text := strings.TrimSpace(lines[i])
	var rest string
	switch {
	case strings.HasPrefix(text, "accTitle"):
		rest = text[len("accTitle"):]
	case strings.HasPrefix(text, "accDescr"):
		rest = text[len("accDescr"):]
	default:
		return -1
	}
	rest = strings.TrimLeft(rest, " \t")
	switch {
	case rest == "" || rest[0] == ':':
		return i
	case rest[0] != '{':
		return -1
	case strings.IndexByte(rest, '}') >= 0:
		return i
	}
	for j := i + 1; j < len(lines); j++ {
		if strings.IndexByte(lines[j], '}') >= 0 {
			return j
		}
	}
	return len(lines) - 1
}

// splitter accumulates statement text across lines, carrying double-quote
// state so a quoted string may span them.
type splitter struct {
	out    []statement
	cur    strings.Builder
	line   int // 1-based line the current statement started on
	quoted bool
}

// feed scans one source line, emitting a statement at every semicolon
// outside quotes (except one closing an entity code such as #quot;) and at
// the end of the line unless a quoted string is still open.
func (sp *splitter) feed(lineNo int, line string) {
	if sp.cur.Len() == 0 {
		sp.line = lineNo
	}
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case c == '"':
			sp.quoted = !sp.quoted
		case c == ';' && !sp.quoted && !endsEntity(sp.cur.String()):
			sp.flush()
			sp.line = lineNo
			continue
		}
		sp.cur.WriteByte(c)
	}
	if sp.quoted {
		sp.cur.WriteByte('\n')
		return
	}
	sp.flush()
}

// flush emits the accumulated text as a statement unless it is blank.
func (sp *splitter) flush() {
	text := strings.TrimSpace(sp.cur.String())
	sp.cur.Reset()
	if text != "" {
		sp.out = append(sp.out, statement{line: sp.line, text: text})
	}
}

// endsEntity reports whether text ends with the #name of an entity code, so
// the semicolon after it belongs to the label.
func endsEntity(text string) bool {
	i := len(text)
	for i > 0 && isIdent(text[i-1]) && text[i-1] != '_' {
		i--
	}
	return i < len(text) && i > 0 && text[i-1] == '#'
}

// parseHeader validates the graph/flowchart header statement and returns
// present's direction for it: TB, LR, or "" when the header names none so
// present picks one from the node count.
func parseHeader(text string) (string, error) {
	fields := strings.Fields(text)
	if len(fields) == 0 || len(fields) > 2 {
		return "", fmt.Errorf("%w: %q", ErrNotFlowchart, snippet(text))
	}
	switch strings.ToLower(fields[0]) {
	case "graph", "flowchart":
	default:
		return "", fmt.Errorf("%w: %q", ErrNotFlowchart, snippet(text))
	}
	if len(fields) == 1 {
		return "", nil
	}
	switch strings.ToUpper(fields[1]) {
	case "TB", "TD", "BT":
		return "TB", nil
	case "LR", "RL":
		return "LR", nil
	}
	return "", fmt.Errorf("%w: %q", ErrNotFlowchart, snippet(text))
}

// parser accumulates nodes and edges across statements and tracks the
// subgraph stack that colours nodes.
type parser struct {
	nodes     []render.GraphNode
	index     map[string]int // node id -> position in nodes
	edges     []render.GraphEdge
	subgraphs []int // open subgraph indexes, innermost last
	nextSub   int   // index the next subgraph statement receives
	edgeIDs   map[string]bool
}

// skipped lists statement keywords that carry nothing present can draw; a
// statement opening with one of them is dropped unparsed.
var skipped = []string{"classDef", "class", "style", "linkStyle", "click", "direction"}

// statement dispatches one statement: subgraph bookkeeping, a skipped
// keyword, or a node chain.
func (p *parser) statement(st statement) error {
	switch {
	case st.text == "end":
		if n := len(p.subgraphs); n > 0 {
			p.subgraphs = p.subgraphs[:n-1]
		}
		return nil
	case hasKeyword(st.text, "subgraph"):
		p.subgraphs = append(p.subgraphs, p.nextSub)
		p.nextSub++
		return nil
	}
	for _, kw := range skipped {
		if hasKeyword(st.text, kw) {
			return nil
		}
	}
	if id := blockSubject(st.text); p.edgeIDs[id] {
		return nil
	}
	return p.chain(st)
}

// blockSubject returns the id that a whole-statement `id@{ ... }` block
// configures, or "". For a known edge id the statement is dropped; for any
// other id the same syntax declares a node.
func blockSubject(text string) string {
	s := &scanner{src: text}
	id := s.ident()
	if id == "" || !strings.HasPrefix(s.rest(), "@{") {
		return ""
	}
	return id
}

// hasKeyword reports whether text opens with kw as a whole word.
func hasKeyword(text, kw string) bool {
	if !strings.HasPrefix(text, kw) {
		return false
	}
	return len(text) == len(kw) || text[len(kw)] == ' ' || text[len(kw)] == '\t'
}

// chain parses `nodeGroup (link nodeGroup)*` and records its nodes and
// edges; groups on both sides of a link fan out to every pair.
func (p *parser) chain(st statement) error {
	s := &scanner{src: st.text}
	from, err := p.nodeGroup(s)
	if err != nil {
		return st.wrap(err)
	}
	for !s.eof() {
		lk, err := s.link()
		if err != nil {
			return st.wrap(err)
		}
		if lk.id != "" {
			p.edgeIDs[lk.id] = true
		}
		to, err := p.nodeGroup(s)
		if err != nil {
			return st.wrap(err)
		}
		if !lk.invisible {
			if err := p.connect(from, to, lk); err != nil {
				return st.wrap(err)
			}
		}
		from = to
	}
	return nil
}

// connect records one edge for every (from, to) pair in source order,
// refusing the whole statement when it would pass maxEdges.
func (p *parser) connect(from, to []string, lk link) error {
	if len(p.edges)+len(from)*len(to) > maxEdges {
		return errTooManyEdges
	}
	for _, a := range from {
		for _, b := range to {
			p.edges = append(
				p.edges,
				render.GraphEdge{From: a, To: b, Type: lk.kind, Label: lk.label},
			)
		}
	}
	return nil
}

// nodeGroup parses `node ("&" node)*` and returns the distinct ids in
// order, so `a&a --> b` fans out to one edge.
func (p *parser) nodeGroup(s *scanner) ([]string, error) {
	var ids []string
	for {
		s.skipSpaces()
		id, err := p.node(s)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
		s.skipSpaces()
		if s.peek() != '&' {
			return ids, nil
		}
		s.pos++
	}
}

// node parses one node reference (id, optional shape label, optional :::class
// and @{ } suffixes), records it, and returns its id.
func (p *parser) node(s *scanner) (string, error) {
	id := s.ident()
	if id == "" {
		return "", s.unexpected()
	}
	label, err := s.shapeLabel()
	if err != nil {
		return "", err
	}
	blockLabel, err := s.skipSuffixes()
	if err != nil {
		return "", err
	}
	if blockLabel != "" {
		label = blockLabel
	}
	if err := p.mention(id, label); err != nil {
		return "", err
	}
	return id, nil
}

// mention records a node at first sight and updates it on later mentions:
// a labelled definition replaces an earlier bare mention's label (a bare
// mention never clears one), and the first subgraph whose body mentions the
// node claims it as a module coloured by that subgraph, as Mermaid does,
// even when the node first appeared outside any subgraph.
func (p *parser) mention(id, label string) error {
	i, ok := p.index[id]
	if !ok {
		if len(p.nodes) >= maxNodes {
			return errTooManyNodes
		}
		i = len(p.nodes)
		p.index[id] = i
		p.nodes = append(p.nodes, render.GraphNode{ID: id, Label: id})
	}
	if label != "" {
		p.nodes[i].Label = label
	}
	if k := len(p.subgraphs); k > 0 && p.nodes[i].Type == "" {
		p.nodes[i].Type = "module"
		p.nodes[i].Color = p.subgraphs[k-1] % moduleColors
	}
	return nil
}

// link is one parsed link between node groups.
type link struct {
	id        string // Mermaid v11 edge id from `A e1@--> B`, or ""
	kind      string // "" solid, "publishes" dotted
	label     string
	invisible bool // ~~~ layout-only link: nodes are recorded, no edge
}

// scanner walks one statement byte by byte. The grammar is ASCII, so byte
// indexing is safe and multibyte label text passes through untouched.
type scanner struct {
	src string
	pos int
}

func (s *scanner) eof() bool { return s.pos >= len(s.src) }

func (s *scanner) rest() string { return s.src[s.pos:] }

// peek returns the current byte, or 0 at the end of the statement.
func (s *scanner) peek() byte { return s.peekAt(0) }

// peekAt returns the byte n past the current one, or 0 past the end.
func (s *scanner) peekAt(n int) byte {
	if s.pos+n >= len(s.src) {
		return 0
	}
	return s.src[s.pos+n]
}

func (s *scanner) skipSpaces() {
	for s.peek() == ' ' || s.peek() == '\t' {
		s.pos++
	}
}

// accept consumes prefix when the statement continues with it.
func (s *scanner) accept(prefix string) bool {
	if !strings.HasPrefix(s.rest(), prefix) {
		return false
	}
	s.pos += len(prefix)
	return true
}

// run consumes the longest prefix whose bytes satisfy ok.
func (s *scanner) run(ok func(byte) bool) string {
	start := s.pos
	for !s.eof() && ok(s.peek()) {
		s.pos++
	}
	return s.src[start:s.pos]
}

// unexpected describes what the scanner cannot consume at its position.
func (s *scanner) unexpected() error {
	if s.eof() {
		return errors.New("unexpected end of statement")
	}
	return fmt.Errorf("unexpected %q", snippet(s.rest()))
}

// isIdent reports whether c can appear anywhere in a node id.
func isIdent(c byte) bool {
	return c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// ident scans a node id: identifier bytes plus a - or . that joins two of
// them, so my-node-->B yields my-node and A-->B yields A.
func (s *scanner) ident() string {
	start := s.pos
	for !s.eof() {
		c := s.peek()
		joiner := (c == '-' || c == '.') && isIdent(s.peekAt(1))
		if !isIdent(c) && !joiner {
			break
		}
		s.pos++
	}
	return s.src[start:s.pos]
}

// opener is a shape opener with the byte that ends its label.
type opener struct {
	open  string
	close byte
}

// openers lists shape openers longest first so `([` wins over `(`.
var openers = []opener{
	{"(((", ')'},
	{"([", ')'},
	{"[[", ']'},
	{"[(", ']'},
	{"((", ')'},
	{"{{", '}'},
	{"[/", ']'},
	{`[\`, ']'},
	{">", ']'},
	{"[", ']'},
	{"(", ')'},
	{"{", '}'},
}

// closerBytes may trail a label before the statement resumes: the three
// closing brackets plus the / and \ of trapezoid shapes.
const closerBytes = `/\])}`

// shapeLabel parses the optional shape+label right after an id and returns
// the cleaned label, or "" when the node has none. The shape itself is
// dropped: present draws every node as a rounded rectangle.
func (s *scanner) shapeLabel() (string, error) {
	for _, o := range openers {
		if !s.accept(o.open) {
			continue
		}
		raw, err := s.labelBody(o.close)
		if err != nil {
			return "", err
		}
		return cleanLabel(raw), nil
	}
	return "", nil
}

// labelBody reads label content up to the family's closing byte. A
// double-quoted label may contain closers; an unquoted one ends at the first
// closer and loses the trailing bracket bytes of two-character shapes.
func (s *scanner) labelBody(close byte) (string, error) {
	if s.accept(`"`) {
		end := strings.IndexByte(s.rest(), '"')
		if end < 0 {
			return "", errors.New("unterminated quote in label")
		}
		raw := s.rest()[:end]
		s.pos += end + 1
		if !s.skipClosers(close) {
			return "", fmt.Errorf("missing %q after label", close)
		}
		return raw, nil
	}
	end := strings.IndexByte(s.rest(), close)
	if end < 0 {
		return "", fmt.Errorf("missing %q after label", close)
	}
	raw := strings.TrimRight(s.rest()[:end], `/\])`)
	s.pos += end + 1
	s.skipClosers(close)
	return raw, nil
}

// skipClosers consumes a run of closer bytes and reports whether close was
// among them.
func (s *scanner) skipClosers(close byte) bool {
	seen := false
	for !s.eof() && strings.IndexByte(closerBytes, s.peek()) >= 0 {
		seen = seen || s.peek() == close
		s.pos++
	}
	return seen
}

// skipSuffixes drops a :::className suffix and reads past a Mermaid v11
// @{ ... } block, in either order. The block's label value is returned so
// `A@{ shape: rect, label: "x" }` labels A; its other keys are dropped.
func (s *scanner) skipSuffixes() (string, error) {
	var label string
	for {
		switch {
		case s.accept(":::"):
			if s.ident() == "" {
				return "", errors.New("missing class name after the ::: marker")
			}
		case s.accept("@{"):
			body, err := s.braceBody()
			if err != nil {
				return "", err
			}
			if l := cleanLabel(blockLabel(body)); l != "" {
				label = l
			}
		default:
			return label, nil
		}
	}
}

// braceBody consumes up to the } matching an already-consumed {, honouring
// nesting and double quotes, and returns the text between them.
func (s *scanner) braceBody() (string, error) {
	start := s.pos
	depth, quoted := 1, false
	for !s.eof() {
		c := s.peek()
		s.pos++
		switch {
		case c == '"':
			quoted = !quoted
		case quoted:
		case c == '{':
			depth++
		case c == '}':
			if depth--; depth == 0 {
				return s.src[start : s.pos-1], nil
			}
		}
	}
	return "", errors.New("unterminated @{ block")
}

// blockLabel returns the value of the label key in a @{ } block body such
// as `shape: rect, label: "Process"`, or "" when the body has none or does
// not read as key: value pairs.
func blockLabel(body string) string {
	s := &scanner{src: body}
	for !s.eof() {
		s.skipSpaces()
		key := s.run(isIdent)
		s.skipSpaces()
		if key == "" || !s.accept(":") {
			return ""
		}
		s.skipSpaces()
		val, ok := s.blockValue()
		if !ok {
			return ""
		}
		if key == "label" {
			return val
		}
		s.skipSpaces()
		if !s.eof() && !s.accept(",") {
			return ""
		}
	}
	return ""
}

// blockValue reads one @{ } value: a double-quoted string or bare text up
// to the next comma.
func (s *scanner) blockValue() (string, bool) {
	if s.accept(`"`) {
		end := strings.IndexByte(s.rest(), '"')
		if end < 0 {
			return "", false
		}
		val := s.rest()[:end]
		s.pos += end + 1
		return val, true
	}
	end := strings.IndexByte(s.rest(), ',')
	if end < 0 {
		end = len(s.rest())
	}
	val := strings.TrimSpace(s.rest()[:end])
	s.pos += end
	return val, true
}

var (
	brTag = regexp.MustCompile(`(?i)<br\s*/?>`)
	// entityCode is Mermaid's #name; or #123; spelling of an HTML entity.
	entityCode = regexp.MustCompile(`#([A-Za-z][A-Za-z0-9]*|[0-9]+);`)
)

// cleanLabel normalises a raw node label: <br> variants become newlines
// (Cytoscape wraps on them), a Mermaid markdown string loses its backticks,
// entity codes are decoded, and every line is trimmed.
func cleanLabel(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 && raw[0] == '`' && raw[len(raw)-1] == '`' {
		raw = raw[1 : len(raw)-1]
	}
	lines := strings.Split(brTag.ReplaceAllString(raw, "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	return decodeEntities(strings.TrimSpace(strings.Join(lines, "\n")))
}

// cleanEdgeLabel trims an edge label, strips the quotes of a quoted one,
// unwraps a Mermaid markdown string's backticks, and decodes entity codes.
func cleanEdgeLabel(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		raw = strings.TrimSpace(raw[1 : len(raw)-1])
	}
	if len(raw) >= 2 && raw[0] == '`' && raw[len(raw)-1] == '`' {
		raw = strings.TrimSpace(raw[1 : len(raw)-1])
	}
	return decodeEntities(raw)
}

// decodeEntities turns Mermaid entity codes (#quot; #9829;) into the
// characters they name. A # that opens no code (C#, #1 priority) and a code
// naming no entity (#zzz;) are kept as written.
func decodeEntities(text string) string {
	if !strings.Contains(text, "#") {
		return text
	}
	return entityCode.ReplaceAllStringFunc(text, func(code string) string {
		entity := "&" + code[1:]
		if c := code[1]; c >= '0' && c <= '9' {
			entity = "&#" + code[1:]
		}
		if decoded := html.UnescapeString(entity); decoded != entity {
			return decoded
		}
		return code
	})
}

// isLinkByte reports whether c can form a link body.
func isLinkByte(c byte) bool { return c == '-' || c == '.' || c == '=' || c == '~' }

// isHeadBoundary reports whether c may follow an x or o arrow head, which
// keeps `A --- oops` parsing oops as a node.
func isHeadBoundary(c byte) bool {
	return c == 0 || c == ' ' || c == '\t' || c == '|' || c == '&'
}

// Closers of the inline-text link forms `-- text -->`, `-. text .->`, and
// `== text ==>`; the head after the run is consumed separately.
var (
	closeDash = regexp.MustCompile(`-{2,}`)
	closeDot  = regexp.MustCompile(`\.+-+`)
	closeEq   = regexp.MustCompile(`={2,}`)
)

// link parses one link with the spaces around it: an optional `id@` edge
// name, an optional leading <, x, or o (bidirectional; collapsed to one
// From->To edge), a body of two or more link bytes, an optional head, and an
// optional label in either the inline `-- text -->` form or the `|text|`
// form. Every head becomes a plain arrow.
func (s *scanner) link() (link, error) {
	s.skipSpaces()
	lk := link{id: s.edgeID()}
	if c := s.peek(); c == '<' || ((c == 'x' || c == 'o') && isLinkByte(s.peekAt(1))) {
		s.pos++
	}
	body := s.run(isLinkByte)
	if len(body) < 2 {
		return link{}, s.unexpected()
	}
	lk.invisible = strings.Contains(body, "~")
	if strings.Contains(body, ".") {
		lk.kind = "publishes"
	}
	if !s.head() && (body == "--" || body == "-." || body == "==") {
		text, err := s.inlineText(body)
		if err != nil {
			return link{}, err
		}
		lk.label = text
	}
	s.skipSpaces()
	if s.accept("|") {
		end := strings.IndexByte(s.rest(), '|')
		if end < 0 {
			return link{}, errors.New("unterminated |label|")
		}
		lk.label = cleanEdgeLabel(s.rest()[:end])
		s.pos += end + 1
	}
	s.skipSpaces()
	return lk, nil
}

// edgeID consumes a Mermaid v11 `id@` edge name in front of a link body and
// returns the id, or "" (consuming nothing) when there is none.
func (s *scanner) edgeID() string {
	start := s.pos
	id := s.ident()
	if id != "" && s.accept("@") {
		return id
	}
	s.pos = start
	return ""
}

// head consumes an arrow head and reports whether it found one: > always,
// x or o only at a head boundary.
func (s *scanner) head() bool {
	switch c := s.peek(); {
	case c == '>':
		s.pos++
		return true
	case (c == 'x' || c == 'o') && isHeadBoundary(s.peekAt(1)):
		s.pos++
		return true
	}
	return false
}

// inlineText reads the text of an inline-form link up to its closing run,
// then consumes that run and any head after it.
func (s *scanner) inlineText(body string) (string, error) {
	closer := closeDash
	switch body {
	case "-.":
		closer = closeDot
	case "==":
		closer = closeEq
	}
	loc := closer.FindStringIndex(s.rest())
	if loc == nil {
		return "", fmt.Errorf("unterminated link text after %q", body)
	}
	text := s.rest()[:loc[0]]
	s.pos += loc[1]
	s.head()
	return cleanEdgeLabel(text), nil
}

// snippet shortens text for an error message.
func snippet(text string) string {
	r := []rune(text)
	if len(r) <= snippetLen {
		return text
	}
	return string(r[:snippetLen]) + "..."
}
