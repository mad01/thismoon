package cslignore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAndMatch(t *testing.T) {
	root := t.TempDir()
	ignoreFile := `# comment
PetPete/Resources/Models/
*.generated.swift
/docs/internal.md
data/**/*.csv
`
	if err := os.WriteFile(filepath.Join(root, ".cslignore"), []byte(ignoreFile), 0o644); err != nil {
		t.Fatalf("write .cslignore: %v", err)
	}
	m := Load(root)
	if m == nil {
		t.Fatal("Load returned nil for a valid file")
	}

	matched := []string{
		"PetPete/Resources/Models/Qwen3/merges.txt",
		"App/API.generated.swift",
		"Deep/Nested/Thing.generated.swift",
		"docs/internal.md",
		"data/exports/2024/big.csv",
	}
	for _, p := range matched {
		if !m.Match(p) {
			t.Errorf("Match(%q) = false, want true", p)
		}
	}

	unmatched := []string{
		"PetPete/Sources/Models.swift", // Models/ is a dir pattern, not a name substring
		"App/API.swift",
		"other/docs/internal.md", // /docs/... is root-anchored
		"data/readme.md",
	}
	for _, p := range unmatched {
		if m.Match(p) {
			t.Errorf("Match(%q) = true, want false", p)
		}
	}
}

func TestLoadMissingAndNil(t *testing.T) {
	if Load(t.TempDir()) != nil {
		t.Error("Load = non-nil for a dir without .cslignore")
	}
	var nilMatcher *Matcher
	if nilMatcher.Match("anything") {
		t.Error("nil matcher matched")
	}
}
