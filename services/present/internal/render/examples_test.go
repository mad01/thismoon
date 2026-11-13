package render_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/present/internal/lint"
	"github.com/mad01/thismoon/services/present/internal/render"
)

// skillRoot is the present skill's directory, from this package.
var skillRoot = filepath.Join("..", "..", "..", "..", "skills", "present")

// skillCoreWordCap caps SKILL.md, the part of the skill every session loads
// whole. Detail that one rendition or one block kind needs goes in a file
// under references/ that the workflow names at the step it is needed.
const skillCoreWordCap = 10000

// createArgs is a present_create argument object as the skill's example,
// gallery, and skeleton files write it: content and deck as Doc objects, or
// as the JSON strings the tool takes.
type createArgs struct {
	Title   string          `json:"title"`
	Content json.RawMessage `json:"content"`
	Deck    json.RawMessage `json:"deck"`
	Graph   json.RawMessage `json:"graph"`
}

// TestSkillExamplesCompile keeps the worked examples beside the skill
// compiling. Each file is a present_create argument object: its content and
// deck are Docs the renderer accepts, its graph (when it has one) compiles,
// and no deck slide keeps a card around a chart, which deck rule 17 forbids.
func TestSkillExamplesCompile(t *testing.T) {
	paths := skillGlob(t, "examples/*.json")
	if len(paths) == 0 {
		t.Fatal("no examples under examples/")
	}
	for _, path := range paths {
		name := filepath.Base(path)
		args := readArgs(t, path)
		if args.Title == "" || len(args.Content) == 0 || len(args.Deck) == 0 {
			t.Errorf("%s: want title, content, and deck", name)
			continue
		}
		if err := compileArgs(args); err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if framedChart(args.Deck) {
			t.Errorf("%s deck: a slide keeps a card around a chart (deck rule 17)", name)
		}
	}
}

// TestGalleryIsClean holds every gallery page to what the skill asks of an
// author: it compiles, and the lint finds nothing in it. A gallery page's
// <name>.source.json beside it is not a page and is skipped.
func TestGalleryIsClean(t *testing.T) {
	for _, path := range galleryPages(t) {
		name := filepath.Base(path)
		args := readArgs(t, path)
		if args.Title == "" || (len(args.Content) == 0 && len(args.Deck) == 0) {
			t.Errorf("%s: want a title and a content or deck", name)
			continue
		}
		if err := compileArgs(args); err != nil {
			t.Errorf("%s: %v", name, err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		page, err := lint.ParseArgs(raw)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		for _, f := range lint.Check(page) {
			t.Errorf("%s: %s [%s] %s", name, f.Where, f.Rule, f.Message)
		}
	}
}

// TestGallerySourcesAreNotEvalCases keeps the gallery out of the looks
// eval: a page built from a case's source would teach the author the answer
// the eval grades. Every gallery page has a <name>.source.json, shaped like a
// case's source ({repo, path, commit}), and no path equals a case's.
func TestGallerySourcesAreNotEvalCases(t *testing.T) {
	pages := galleryPages(t)
	if len(pages) == 0 {
		return
	}
	cases := map[string]string{}
	for _, path := range skillGlob(t, "evals/looks/cases/*/case.json") {
		var c struct {
			Source struct {
				Path string `json:"path"`
			} `json:"source"`
		}
		readJSON(t, path, &c)
		cases[c.Source.Path] = filepath.Base(filepath.Dir(path))
	}
	for _, page := range pages {
		srcPath := strings.TrimSuffix(page, ".json") + ".source.json"
		var src struct {
			Repo   string `json:"repo"`
			Path   string `json:"path"`
			Commit string `json:"commit"`
		}
		readJSON(t, srcPath, &src)
		name := filepath.Base(srcPath)
		if src.Repo == "" || src.Path == "" || src.Commit == "" {
			t.Errorf("%s: want repo, path, and commit", name)
		}
		if c, ok := cases[src.Path]; ok {
			t.Errorf("%s: %s is the source of the eval case %s", name, src.Path, c)
		}
	}
}

// TestSkillCoreSize keeps SKILL.md under its word cap.
func TestSkillCoreSize(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(skillRoot, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(strings.Fields(string(raw))); n > skillCoreWordCap {
		t.Errorf("SKILL.md holds %d words, past the cap of %d; move detail into references/", n, skillCoreWordCap)
	}
}

// mdLink is a markdown link's target; the target ends at the first space or
// closing parenthesis. codeSpan is an inline code span, whose text is not a
// link.
var (
	mdLink   = regexp.MustCompile(`\]\(([^)\s]+)\)`)
	codeSpan = regexp.MustCompile("`[^`\n]*`")
)

// TestSkillLinksResolve: every file SKILL.md or a reference file links to
// exists, so a step that says to read a file never sends the author to
// nothing. Gallery links are checked once the gallery holds a page.
func TestSkillLinksResolve(t *testing.T) {
	galleryFilled := len(galleryPages(t)) > 0
	for _, path := range skillMarkdown(t) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(skillRoot, path)
		prose := jsonFence.ReplaceAllString(string(raw), "")
		prose = codeSpan.ReplaceAllString(prose, "")
		for _, m := range mdLink.FindAllStringSubmatch(prose, -1) {
			target, _, _ := strings.Cut(m[1], "#")
			if target == "" || strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			resolved := filepath.Join(filepath.Dir(path), target)
			if !galleryFilled && strings.HasPrefix(filepath.ToSlash(resolved), filepath.ToSlash(filepath.Join(skillRoot, "gallery"))+"/") {
				continue
			}
			if _, err := os.Stat(resolved); err != nil {
				t.Errorf("%s links to %s, which does not exist", rel, m[1])
			}
		}
	}
}

// jsonFence is a fenced json block in a markdown file.
var jsonFence = regexp.MustCompile("(?s)```json\n(.*?)```")

// TestSkillJSONCompiles compiles every json block in SKILL.md and the
// reference files: argument objects, Docs, sections, blocks, block lists,
// and graphs. A skeleton or an example the renderer refuses would teach an
// author a shape that fails. A placeholder between << and >> sits only in
// strings, so the skeletons compile as written.
func TestSkillJSONCompiles(t *testing.T) {
	for _, path := range skillMarkdown(t) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(skillRoot, path)
		for i, m := range jsonFence.FindAllStringSubmatch(string(raw), -1) {
			if err := compileFence([]byte(m[1])); err != nil {
				t.Errorf("%s, json block %d: %v", rel, i+1, err)
			}
		}
	}
}

// compileFence compiles one json block by what it holds.
func compileFence(data []byte) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	wrapBlocks := func(blocks json.RawMessage) error {
		_, err := render.Compile([]byte(`{"sections":[{"h":"S","blocks":`+string(blocks)+`}]}`), "T")
		return err
	}
	switch x := v.(type) {
	case []any:
		if len(x) > 0 && isReference(x[0]) {
			return nil
		}
		return wrapBlocks(data)
	case map[string]any:
		switch {
		case x["title"] != nil:
			var args createArgs
			if err := json.Unmarshal(data, &args); err != nil {
				return err
			}
			return compileArgs(args)
		case x["t"] != nil:
			return wrapBlocks(json.RawMessage("[" + string(data) + "]"))
		case x["h"] != nil:
			_, err := render.CompileDeck([]byte(`{"sections":[`+string(data)+`]}`), "T")
			return err
		case x["sections"] != nil:
			_, err := render.CompileDeck(data, "T")
			return err
		case x["nodes"] != nil:
			return compileGraph(data)
		}
	}
	return errors.New("not an argument object, Doc, section, block, block list, or graph")
}

// isReference reports whether v is a references entry, {title, url}.
func isReference(v any) bool {
	m, ok := v.(map[string]any)
	return ok && m["url"] != nil && m["t"] == nil
}

// compileArgs compiles an argument object's content, deck, and graph, each
// when present.
func compileArgs(args createArgs) error {
	if len(args.Content) > 0 {
		doc, err := docJSON(args.Content)
		if err != nil {
			return fmt.Errorf("content: %w", err)
		}
		if _, err := render.Compile(doc, args.Title); err != nil {
			return fmt.Errorf("content: %w", err)
		}
	}
	if len(args.Deck) > 0 {
		doc, err := docJSON(args.Deck)
		if err != nil {
			return fmt.Errorf("deck: %w", err)
		}
		if _, err := render.CompileDeck(doc, args.Title); err != nil {
			return fmt.Errorf("deck: %w", err)
		}
	}
	if len(args.Graph) > 0 {
		if err := compileGraph(args.Graph); err != nil {
			return fmt.Errorf("graph: %w", err)
		}
	}
	return nil
}

// docJSON returns a Doc argument as Doc JSON: the object itself, or the
// string the tool takes it as.
func docJSON(raw json.RawMessage) ([]byte, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []byte(s), nil
	}
	return raw, nil
}

func compileGraph(raw []byte) error {
	var g render.GraphInput
	if err := json.Unmarshal(raw, &g); err != nil {
		return err
	}
	_, _, err := render.CompileGraph(g)
	return err
}

// galleryPages lists the gallery's pages: every json file in gallery/ but
// the <name>.source.json files beside them.
func galleryPages(t *testing.T) []string {
	t.Helper()
	var pages []string
	for _, path := range skillGlob(t, "gallery/*.json") {
		if !strings.HasSuffix(path, ".source.json") {
			pages = append(pages, path)
		}
	}
	return pages
}

// skillMarkdown lists SKILL.md and every markdown file under references/.
func skillMarkdown(t *testing.T) []string {
	t.Helper()
	return append([]string{filepath.Join(skillRoot, "SKILL.md")}, skillGlob(t, "references/*.md")...)
}

func skillGlob(t *testing.T, pattern string) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(skillRoot, pattern))
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

func readArgs(t *testing.T, path string) createArgs {
	t.Helper()
	var args createArgs
	readJSON(t, path, &args)
	return args
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("%s does not exist", path)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("%s: %v", path, err)
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
