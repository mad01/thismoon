package grammar

import (
	"errors"
	"testing"
)

func TestLangForPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"a/b/main.go", "go"},
		{"x.ts", "typescript"},
		{"x.tsx", "typescript"},
		{"y.py", "python"},
		{"src/Main.java", "java"},
		{"main.tf", "hcl"},
		{"vars.hcl", "hcl"},
		{"run.sh", "bash"},
		{"env.bash", "bash"},
		{"Dockerfile", "dockerfile"},
		{"docker/Dockerfile.dev", "dockerfile"},
		{"build.dockerfile", "dockerfile"},
		{"README.md", "markdown"},
		{"api.proto", "protobuf"},
		{"schema.sql", "sql"},
		{"config.yaml", "yaml"},
		{"ci.yml", "yaml"},
		{"noext", ""},
	}
	for _, tt := range tests {
		if got := LangForPath(tt.path); got != tt.want {
			t.Errorf("LangForPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// TestLookupCoversEveryLang pins that every tag LangForPath can return has a
// grammar, so a new extension cannot be added without linking its parser.
func TestLookupCoversEveryLang(t *testing.T) {
	for _, path := range []string{
		"a.go", "a.ts", "a.py", "a.java", "a.tf", "a.sh", "Dockerfile",
		"a.md", "a.proto", "a.sql", "a.yaml",
	} {
		lang := LangForPath(path)
		if _, ok := Lookup(lang); !ok {
			t.Errorf("LangForPath(%q) = %q has no grammar", path, lang)
		}
	}
	if _, ok := Lookup("cobol"); ok {
		t.Error("Lookup(cobol) found a grammar")
	}
}

func TestParse(t *testing.T) {
	tree, err := Parse("go", []byte("package p\nfunc F() {}\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Close()
	if got := tree.RootNode().Type(); got != "source_file" {
		t.Errorf("root type = %q, want source_file", got)
	}

	if _, err := Parse("cobol", nil); !errors.Is(err, ErrNoGrammar) {
		t.Errorf("Parse(cobol) error = %v, want ErrNoGrammar", err)
	}

	// Markdown parses with the block grammar: headings are block nodes.
	md, err := Parse("markdown", []byte("# Title\n\ntext\n"))
	if err != nil {
		t.Fatalf("Parse markdown: %v", err)
	}
	defer md.Close()
	if got := md.RootNode().NamedChild(0).NamedChild(0).Type(); got != "atx_heading" {
		t.Errorf("first block = %q, want atx_heading", got)
	}
}
