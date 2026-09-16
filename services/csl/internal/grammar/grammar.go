// Package grammar is the one table of tree-sitter grammars csl links and the
// file-name rule that picks one. The lexical symbol extractor and the semantic
// chunker both parse through it, so a language is added in exactly one place
// and lexical search never imports the semantic package (see why.md).
package grammar

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/bash"
	"github.com/smacker/go-tree-sitter/dockerfile"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/hcl"
	"github.com/smacker/go-tree-sitter/java"
	markdown "github.com/smacker/go-tree-sitter/markdown/tree-sitter-markdown"
	"github.com/smacker/go-tree-sitter/protobuf"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/sql"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
	"github.com/smacker/go-tree-sitter/yaml"
)

// ErrNoGrammar reports a language tag with no linked grammar.
var ErrNoGrammar = errors.New("grammar: no grammar for language")

// LangForPath derives a language tag from a file extension (or, for
// dockerfiles, the file name). It returns an empty string for files without a
// tree-sitter grammar.
func LangForPath(path string) string {
	base := strings.ToLower(filepath.Base(path))
	if base == "dockerfile" || strings.HasPrefix(base, "dockerfile.") ||
		strings.HasSuffix(base, ".dockerfile") {
		return "dockerfile"
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".py":
		return "python"
	case ".java":
		return "java"
	case ".tf", ".hcl":
		return "hcl"
	case ".sh", ".bash":
		return "bash"
	case ".md", ".markdown":
		return "markdown"
	case ".proto":
		return "protobuf"
	case ".sql":
		return "sql"
	case ".yml", ".yaml":
		return "yaml"
	default:
		return ""
	}
}

// Lookup returns the tree-sitter grammar for a language tag. Markdown is the
// block grammar: headings, sections, and paragraphs are block nodes, and
// neither caller needs the inline trees.
func Lookup(lang string) (*sitter.Language, bool) {
	switch lang {
	case "go":
		return golang.GetLanguage(), true
	case "typescript":
		return typescript.GetLanguage(), true
	case "python":
		return python.GetLanguage(), true
	case "java":
		return java.GetLanguage(), true
	case "hcl":
		return hcl.GetLanguage(), true
	case "bash":
		return bash.GetLanguage(), true
	case "dockerfile":
		return dockerfile.GetLanguage(), true
	case "markdown":
		return markdown.GetLanguage(), true
	case "protobuf":
		return protobuf.GetLanguage(), true
	case "sql":
		return sql.GetLanguage(), true
	case "yaml":
		return yaml.GetLanguage(), true
	default:
		return nil, false
	}
}

// Parse parses src with the grammar for lang. The caller owns the returned
// tree and closes it when done. tree-sitter recovers from syntax errors, so a
// malformed file still yields a tree with ERROR nodes rather than an error.
func Parse(lang string, src []byte) (*sitter.Tree, error) {
	grammar, ok := Lookup(lang)
	if !ok {
		return nil, fmt.Errorf("%w %q", ErrNoGrammar, lang)
	}
	parser := sitter.NewParser()
	parser.SetLanguage(grammar)
	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return nil, fmt.Errorf("grammar: parse %s: %w", lang, err)
	}
	return tree, nil
}
