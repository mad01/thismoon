package semantic

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

// Line-window fallback parameters (1-based, inclusive ranges).
const (
	windowSize    = 40
	windowOverlap = 10
)

// maxChunkBodyChars bounds a chunk's body so its breadcrumb-prefixed EmbedText
// stays under the embedding model's fixed sequence length (all-MiniLM-L6-v2
// caps at 512 tokens, which the model cannot exceed). Oversized chunks — long
// functions, big generated blocks — are split into line-aligned sub-chunks
// rather than truncated, so no source content is dropped. The budget is
// conservative because code tokenizes denser than prose.
const maxChunkBodyChars = 900

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

// LangForPath derives a language tag from a file extension. It returns an empty
// string for extensions without a tree-sitter chunker.
func LangForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".py":
		return "python"
	default:
		return ""
	}
}

// ChunkFile splits src into semantic chunks. For go/typescript/python it emits
// one chunk per relevant top-level node via tree-sitter. For any other lang, or
// when tree-sitter yields nothing, it falls back to overlapping line windows.
func ChunkFile(repo, path, lang string, src []byte) ([]Chunk, error) {
	if grammar, kinds, ok := grammarFor(lang); ok {
		chunks, err := chunkWithTreeSitter(repo, path, lang, src, grammar, kinds)
		if err != nil {
			return nil, err
		}
		if len(chunks) > 0 {
			return splitOversized(chunks), nil
		}
	}
	return splitOversized(chunkWindows(repo, path, lang, src)), nil
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

// grammarFor returns the tree-sitter grammar and the set of relevant top-level
// node types for a language tag.
func grammarFor(lang string) (*sitter.Language, map[string]bool, bool) {
	switch lang {
	case "go":
		return golang.GetLanguage(), map[string]bool{
			"function_declaration": true,
			"method_declaration":   true,
			"type_declaration":     true,
		}, true
	case "typescript":
		return typescript.GetLanguage(), map[string]bool{
			"function_declaration": true,
			"class_declaration":    true,
			"export_statement":     true,
		}, true
	case "python":
		return python.GetLanguage(), map[string]bool{
			"function_definition": true,
			"class_definition":    true,
		}, true
	default:
		return nil, nil, false
	}
}

// chunkWithTreeSitter parses src and emits one chunk per relevant top-level node.
func chunkWithTreeSitter(repo, path, lang string, src []byte, grammar *sitter.Language, kinds map[string]bool) ([]Chunk, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(grammar)

	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	defer tree.Close()

	root := tree.RootNode()
	var chunks []Chunk
	for i := 0; i < int(root.NamedChildCount()); i++ {
		n := root.NamedChild(i)
		kind := n.Type()
		if !kinds[kind] {
			continue
		}
		text := string(src[n.StartByte():n.EndByte()])
		start := int(n.StartPoint().Row) + 1
		end := int(n.EndPoint().Row) + 1
		chunks = append(chunks, newChunk(repo, path, lang, kind, start, end, text))
	}
	return chunks, nil
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
