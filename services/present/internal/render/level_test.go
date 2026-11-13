package render

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"slices"
	"strings"
	"testing"

	present "github.com/mad01/thismoon/services/present"
	"github.com/mad01/thismoon/webkit"
)

// vocabularyLevels maps each fingerprint of the page vocabulary to the
// render level it shipped at. A vocabulary change adds a line at the next
// level and keeps the lines before it, so the raise shows in the diff.
var vocabularyLevels = map[string]int{
	"c1f83b1e51def023": 1,
	"6dbb4a0ac6561418": 2, // diagram key
}

// TestRenderLevelFollowsTheVocabulary holds present.RenderLevel to the
// vocabulary a stored page can use. A shared instance compares levels, not
// vocabularies, so a change that keeps the level makes a share to an older
// instance draw wrong with no warning.
func TestRenderLevelFollowsTheVocabulary(t *testing.T) {
	fp := vocabularyFingerprint()
	highest := 0
	for other, level := range vocabularyLevels {
		if other != fp {
			highest = max(highest, level)
		}
	}
	level, ok := vocabularyLevels[fp]
	switch {
	case !ok:
		next := max(present.RenderLevel, highest+1)
		t.Fatalf(
			"the page vocabulary changed: set present.RenderLevel (facts.go) to %d and add %q: %d to vocabularyLevels",
			next,
			fp,
			next,
		)
	case level != present.RenderLevel:
		t.Fatalf(
			"vocabularyLevels gives this vocabulary level %d, present.RenderLevel is %d: make them equal",
			level,
			present.RenderLevel,
		)
	case highest >= present.RenderLevel:
		t.Fatalf(
			"vocabularyLevels holds an earlier vocabulary at level %d, not below present.RenderLevel %d: "+
				"a new vocabulary needs a new level",
			highest,
			present.RenderLevel,
		)
	}
}

// vocabularyFingerprint hashes what a stored page can name: every field of
// the Doc and the graph input, every block kind, and every word list the
// compiler checks a field against. The lines are sorted, so moving a field
// or a word changes nothing.
func vocabularyFingerprint() string {
	var lines []string
	seen := map[reflect.Type]bool{}
	fieldPaths(reflect.TypeFor[Doc](), "doc", seen, &lines)
	fieldPaths(reflect.TypeFor[GraphInput](), "graph", seen, &lines)
	for _, tmpl := range blockTemplates.Templates() {
		if kind, ok := strings.CutPrefix(tmpl.Name(), "block-"); ok {
			lines = append(lines, "block "+kind)
		}
	}
	lists := map[string][]string{
		"layout":            layouts,
		"transition":        transitions,
		"progress":          progressKinds,
		"logo-position":     logoPositions,
		"chart-kind":        chartKinds,
		"ribbon-order":      ribbonOrders,
		"diagram-kind":      diagramKinds,
		"diagram-direction": diagramDirections,
		"graph-tone":        graphTones,
		"role":              webkit.Roles(),
	}
	for name, words := range lists {
		for _, w := range words {
			lines = append(lines, name+" "+w)
		}
	}
	slices.Sort(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:8])
}

// fieldPaths appends one line per exported field reachable from t, named
// by its JSON key (the Go name when it has none), each type expanded once.
func fieldPaths(t reflect.Type, prefix string, seen map[reflect.Type]bool, out *[]string) {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || seen[t] {
		return
	}
	seen[t] = true
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name := f.Name
		if key, _, _ := strings.Cut(f.Tag.Get("json"), ","); key != "" && key != "-" {
			name = key
		}
		path := prefix + "." + name
		*out = append(*out, "field "+path)
		fieldPaths(f.Type, path, seen, out)
	}
}
