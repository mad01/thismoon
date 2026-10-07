package render

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestSkillExamplesCompile keeps the worked examples beside the skill
// compiling. Each file is a present_create argument object: its content and
// deck are Docs the renderer accepts, its graph (when it has one) compiles,
// and no deck slide keeps a card around a chart, which deck rule 17 forbids.
func TestSkillExamplesCompile(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "..", "skills", "present", "examples")
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatalf("no examples under %s", dir)
	}
	for _, path := range paths {
		name := filepath.Base(path)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var ex struct {
			Title   string          `json:"title"`
			Content json.RawMessage `json:"content"`
			Deck    json.RawMessage `json:"deck"`
			Graph   json.RawMessage `json:"graph"`
		}
		if err := json.Unmarshal(raw, &ex); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if ex.Title == "" || len(ex.Content) == 0 || len(ex.Deck) == 0 {
			t.Errorf("%s: want title, content, and deck", name)
			continue
		}
		if _, err := Compile(ex.Content, ex.Title); err != nil {
			t.Errorf("%s content: %v", name, err)
		}
		if _, err := CompileDeck(ex.Deck, ex.Title); err != nil {
			t.Errorf("%s deck: %v", name, err)
		}
		if framedChart(ex.Deck) {
			t.Errorf("%s deck: a slide keeps a card around a chart (deck rule 17)", name)
		}
		if len(ex.Graph) == 0 {
			continue
		}
		var g GraphInput
		if err := json.Unmarshal(ex.Graph, &g); err != nil {
			t.Errorf("%s graph: %v", name, err)
			continue
		}
		if _, _, err := CompileGraph(g); err != nil {
			t.Errorf("%s graph: %v", name, err)
		}
	}
}

// framedChart reports whether any chart block in the raw Doc sets frame to
// true, walking the JSON generically so the check needs no block field names.
func framedChart(raw json.RawMessage) bool {
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return false
	}
	var walk func(v any) bool
	walk = func(v any) bool {
		switch x := v.(type) {
		case map[string]any:
			if x["t"] == "chart" && x["frame"] == true {
				return true
			}
			for _, child := range x {
				if walk(child) {
					return true
				}
			}
		case []any:
			for _, child := range x {
				if walk(child) {
					return true
				}
			}
		}
		return false
	}
	return walk(doc)
}
