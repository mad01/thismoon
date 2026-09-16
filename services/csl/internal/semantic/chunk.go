package semantic

import (
	"fmt"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/mad01/thismoon/services/csl/internal/grammar"
)

// Line-window fallback parameters (1-based, inclusive ranges).
const (
	windowSize    = 120
	windowOverlap = 20
)

// chunkerVersion identifies the chunking behavior baked into stored
// embeddings. Bump it on ANY change that alters what ChunkFile emits — new
// languages, node-kind changes, traversal changes, window/budget tuning —
// so existing vector stores rebuild on the next index run instead of serving
// stale chunks forever (chunks are produced at index time; unchanged files
// are otherwise skipped by content hash and would never re-embed).
//
// History: 1 = top-level go/typescript/python; 2 = per-member java (MAD-139)
// + hcl/bash/dockerfile/markdown/protobuf/sql/yaml (MAD-229); 3 = qwen3-sized
// budgets (maxChunkBodyChars 900 → 6000, windows 40/10 → 120/20).
const chunkerVersion = 3

// maxChunkBodyChars bounds a chunk's body so its breadcrumb-prefixed EmbedText
// stays under the num_ctx the embedder requests from Ollama (8192 tokens for
// qwen3-embedding, past which input silently truncates). 6000 chars of code is
// roughly 1.5-2k tokens, so whole declarations almost never split. Oversized
// chunks — very long functions, big generated blocks — are still split into
// line-aligned sub-chunks rather than truncated, so no source content is
// dropped.
const maxChunkBodyChars = 6000

// Chunk is one indexable unit of source: a top-level declaration (when the
// language is parseable) or a fixed line window (fallback). StartLine and
// EndLine are 1-based and inclusive.
type Chunk struct {
	Repo      string
	Path      string
	Lang      string
	Kind      string
	StartLine int
	EndLine   int
	Text      string
	EmbedText string
}

// ChunkFile splits src into semantic chunks: one chunk per relevant declaration
// via tree-sitter for languages with a chunkSpec entry, per section for
// markdown, per build stage for dockerfiles. Languages whose symbols nest
// inside type bodies (java, protobuf services) are split per member. For any
// other lang, or when tree-sitter yields nothing, it falls back to overlapping
// line windows.
func ChunkFile(repo, path, lang string, src []byte) ([]Chunk, error) {
	chunks, err := parseChunks(repo, path, lang, src)
	if err != nil {
		return nil, err
	}
	if len(chunks) > 0 {
		return splitOversized(chunks), nil
	}
	return splitOversized(chunkWindows(repo, path, lang, src)), nil
}

// parseChunks runs the tree-sitter chunker for lang, or nothing for languages
// without one.
func parseChunks(repo, path, lang string, src []byte) ([]Chunk, error) {
	if spec, ok := chunkSpec(lang); ok {
		return chunkWithTreeSitter(repo, path, lang, src, spec)
	}
	return nil, nil
}

// splitOversized replaces any chunk whose body exceeds maxChunkBodyChars with
// line-aligned sub-chunks so no EmbedText overflows the embedding model's fixed
// sequence length. Normal-size chunks pass through unchanged.
func splitOversized(chunks []Chunk) []Chunk {
	out := make([]Chunk, 0, len(chunks))
	for _, c := range chunks {
		if len(c.Text) <= maxChunkBodyChars {
			out = append(out, c)
			continue
		}
		out = append(out, splitChunkByBudget(c)...)
	}
	return out
}

// splitChunkByBudget breaks one oversized chunk into contiguous line-aligned
// sub-chunks, each with a body under maxChunkBodyChars. A single line longer
// than the budget is hard-truncated (the only case where content is dropped).
func splitChunkByBudget(c Chunk) []Chunk {
	lines := strings.Split(c.Text, "\n")
	var out []Chunk
	segStart, cur := 0, 0

	flush := func(end int) {
		if end <= segStart {
			return
		}
		seg := strings.Join(lines[segStart:end], "\n")
		out = append(out, newChunk(c.Repo, c.Path, c.Lang, c.Kind,
			c.StartLine+segStart, c.StartLine+end-1, seg))
		segStart, cur = end, 0
	}

	for i, ln := range lines {
		if len(ln) > maxChunkBodyChars {
			lines[i] = truncateRunes(ln, maxChunkBodyChars)
			ln = lines[i]
		}
		if cur > 0 && cur+len(ln)+1 > maxChunkBodyChars {
			flush(i)
		}
		cur += len(ln) + 1
	}
	flush(len(lines))
	return out
}

// truncateRunes shortens s to at most max runes without splitting a rune.
func truncateRunes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	r := []rune(s)
	if len(r) > max {
		r = r[:max]
	}
	return string(r)
}

// langSpec describes how one language is chunked: which node kinds become
// whole chunks, and — for languages whose symbols nest inside type bodies —
// which container kinds to descend into and which member kinds to emit from
// inside them. The grammar itself comes from the shared grammar registry.
type langSpec struct {
	kinds      map[string]bool // nodes emitted whole
	containers map[string]bool // nodes split into a header chunk + per-member chunks
	members    map[string]bool // nodes emitted whole from inside a container body
	stageKind  string          // dockerfile-style: chunk per group of root children starting at this kind
}

// chunkSpec returns the chunking spec for a language tag.
func chunkSpec(lang string) (langSpec, bool) {
	switch lang {
	case "go":
		return langSpec{
			kinds: map[string]bool{
				"function_declaration": true,
				"method_declaration":   true,
				"type_declaration":     true,
			},
		}, true
	case "typescript":
		return langSpec{
			kinds: map[string]bool{
				"function_declaration": true,
				"class_declaration":    true,
				"export_statement":     true,
			},
		}, true
	case "python":
		return langSpec{
			kinds: map[string]bool{
				"function_definition": true,
				"class_definition":    true,
			},
		}, true
	case "java":
		return langSpec{
			kinds: map[string]bool{
				"enum_declaration": true,
			},
			containers: map[string]bool{
				"class_declaration":     true,
				"interface_declaration": true,
				"record_declaration":    true,
			},
			members: map[string]bool{
				"method_declaration":              true,
				"constructor_declaration":         true,
				"compact_constructor_declaration": true,
			},
		}, true
	case "hcl":
		return langSpec{
			kinds: map[string]bool{
				"block": true,
			},
		}, true
	case "bash":
		return langSpec{
			kinds: map[string]bool{
				"function_definition": true,
			},
		}, true
	case "dockerfile":
		return langSpec{stageKind: "from_instruction"}, true
	case "markdown":
		// One section per chunk: a heading plus its body, stopping at the
		// first subsection (sections nest, so subsections become their own
		// chunks via the container traversal).
		return langSpec{containers: map[string]bool{"section": true}}, true
	case "protobuf":
		return langSpec{
			kinds: map[string]bool{
				"message": true,
				"enum":    true,
			},
			containers: map[string]bool{
				"service": true,
			},
			members: map[string]bool{
				"rpc": true,
			},
		}, true
	case "sql":
		return langSpec{
			kinds: map[string]bool{
				"statement": true,
			},
		}, true
	case "yaml":
		return langSpec{
			kinds: map[string]bool{
				"block_mapping_pair": true,
			},
		}, true
	default:
		return langSpec{}, false
	}
}

// chunkWithTreeSitter parses src and emits chunks for each relevant node.
func chunkWithTreeSitter(repo, path, lang string, src []byte, spec langSpec) ([]Chunk, error) {
	tree, err := grammar.Parse(lang, src)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	defer tree.Close()

	if spec.stageKind != "" {
		return chunkStages(repo, path, lang, src, tree.RootNode(), spec.stageKind), nil
	}
	return chunkDecls(repo, path, lang, src, tree.RootNode(), spec), nil
}

// chunkDecls walks the tree below n and emits a chunk for each outermost node
// matching the spec, descending through wrapper nodes that match nothing (hcl
// wraps blocks in a body node, yaml wraps mappings in stream/document/block
// nodes). Matched nodes are not descended into, so a declaration is never
// emitted twice.
func chunkDecls(repo, path, lang string, src []byte, n *sitter.Node, spec langSpec) []Chunk {
	var out []Chunk
	for i := 0; i < int(n.NamedChildCount()); i++ {
		child := n.NamedChild(i)
		switch kind := child.Type(); {
		case spec.containers[kind]:
			out = append(out, chunkContainer(repo, path, lang, src, child, spec)...)
		case spec.kinds[kind]:
			out = append(out, nodeChunk(repo, path, lang, src, child))
		default:
			out = append(out, chunkDecls(repo, path, lang, src, child, spec)...)
		}
	}
	return out
}

// chunkContainer splits a container (java class/interface/record, protobuf
// service, markdown section) into a header chunk — the container from its start
// to the first extracted member, covering the signature and any leading fields
// — plus one chunk per member, recursing into nested containers. The header
// stops where the first member starts so no source line is embedded twice.
// Containers whose grammar has no body field (protobuf service, markdown
// section) hold members as direct children; a container with no members at all
// is emitted whole.
func chunkContainer(repo, path, lang string, src []byte, n *sitter.Node, spec langSpec) []Chunk {
	body := n.ChildByFieldName("body")
	if body == nil {
		body = n
	}

	var inner []Chunk
	var firstByte uint32
	for i := 0; i < int(body.NamedChildCount()); i++ {
		child := body.NamedChild(i)
		var got []Chunk
		switch kind := child.Type(); {
		case spec.members[kind] || spec.kinds[kind]:
			got = []Chunk{nodeChunk(repo, path, lang, src, child)}
		case spec.containers[kind]:
			got = chunkContainer(repo, path, lang, src, child, spec)
		default:
			continue
		}
		if len(inner) == 0 {
			firstByte = child.StartByte()
		}
		inner = append(inner, got...)
	}
	if len(inner) == 0 {
		return []Chunk{nodeChunk(repo, path, lang, src, n)}
	}

	chunks := make([]Chunk, 0, len(inner)+1)
	if header := strings.TrimRight(string(src[n.StartByte():firstByte]), " \t\n"); header != "" {
		start := int(n.StartPoint().Row) + 1
		end := start + strings.Count(header, "\n")
		chunks = append(chunks, newChunk(repo, path, lang, n.Type(), start, end, header))
	}
	return append(chunks, inner...)
}

// chunkStages splits a dockerfile-shaped tree into one chunk per build stage:
// each group of root children from a stageKind node (FROM) to the next. Lines
// before the first stage — ARGs, comments — belong to the first chunk. Files
// with no stage at all yield nothing, deferring to the window fallback.
func chunkStages(repo, path, lang string, src []byte, root *sitter.Node, stageKind string) []Chunk {
	var starts []int
	for i := 0; i < int(root.NamedChildCount()); i++ {
		if child := root.NamedChild(i); child.Type() == stageKind {
			starts = append(starts, int(child.StartPoint().Row)+1)
		}
	}
	if len(starts) == 0 {
		return nil
	}
	starts[0] = 1

	lines := splitLines(src)
	var chunks []Chunk
	for i, start := range starts {
		end := len(lines)
		if i+1 < len(starts) {
			end = starts[i+1] - 1
		}
		text := strings.Join(lines[start-1:end], "\n")
		chunks = append(chunks, newChunk(repo, path, lang, "stage", start, end, text))
	}
	return chunks
}

// nodeChunk emits one node verbatim as a chunk.
func nodeChunk(repo, path, lang string, src []byte, n *sitter.Node) Chunk {
	text := string(src[n.StartByte():n.EndByte()])
	start := int(n.StartPoint().Row) + 1
	end := int(n.EndPoint().Row) + 1
	return newChunk(repo, path, lang, n.Type(), start, end, text)
}

// chunkWindows splits src into overlapping fixed-size line windows.
func chunkWindows(repo, path, lang string, src []byte) []Chunk {
	lines := splitLines(src)
	total := len(lines)
	if total == 0 {
		return nil
	}

	stride := windowSize - windowOverlap
	var chunks []Chunk
	for start := 1; start <= total; start += stride {
		end := start + windowSize - 1
		if end > total {
			end = total
		}
		text := strings.Join(lines[start-1:end], "\n")
		chunks = append(chunks, newChunk(repo, path, lang, "window", start, end, text))
	}
	return chunks
}

// splitLines splits src into lines, dropping a single trailing empty line that a
// terminating newline would otherwise produce.
func splitLines(src []byte) []string {
	lines := strings.Split(string(src), "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines
}

// newChunk assembles a Chunk and its breadcrumb-prefixed EmbedText.
func newChunk(repo, path, lang, kind string, start, end int, text string) Chunk {
	embed := fmt.Sprintf("// File: %s\n// Lang: %s  Kind: %s\n%s", path, lang, kind, text)
	return Chunk{
		Repo:      repo,
		Path:      path,
		Lang:      lang,
		Kind:      kind,
		StartLine: start,
		EndLine:   end,
		Text:      text,
		EmbedText: embed,
	}
}
