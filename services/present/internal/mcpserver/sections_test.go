package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/present/internal/author"
	"github.com/mad01/thismoon/services/present/internal/docedit"
	"github.com/mad01/thismoon/services/present/internal/render"
	"github.com/mad01/thismoon/services/present/internal/store"
)

const twoSectionDoc = `{"sections":[` +
	`{"h":"Alpha","blocks":[{"t":"p","text":"a"}]},` +
	`{"h":"Next steps","blocks":[{"t":"p","text":"b"}]}]}`

// docHeadings lists the section headings of a stored Doc source.
func docHeadings(t *testing.T, src string) []string {
	t.Helper()
	var d render.Doc
	if err := json.Unmarshal([]byte(src), &d); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, s := range d.Sections {
		out = append(out, s.Heading)
	}
	return out
}

// TestPatchSectionToolRunsTheDocumentedExample runs the example the tool
// description carries, then a markdown append, and reads both back.
func TestPatchSectionToolRunsTheDocumentedExample(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()
	_, created, err := h.handleCreate(ctx, nil, createInput{Title: "T", Content: twoSectionDoc})
	if err != nil {
		t.Fatal(err)
	}
	_, out, err := h.handlePatchSection(ctx, nil, patchSectionInput{
		ID: created.ID, Op: "replace", Anchor: "next-steps", BaseVersion: 1,
		Section: `{"h":"Next steps","blocks":[{"t":"list","items":["Ship the patch route","Add history"]}]}`,
	})
	if err != nil {
		t.Fatalf("documented example: %v", err)
	}
	if out.Version != 2 || out.Index != 1 || out.Anchor != "next-steps" ||
		out.Rendition != "content" {
		t.Fatalf("output = %+v", out)
	}
	_, out, err = h.handlePatchSection(ctx, nil, patchSectionInput{
		ID: created.ID, Op: "append", BodyFormat: "markdown", Section: "## Risks\n\nNone yet.\n",
	})
	if err != nil || out.Version != 3 || out.Anchor != "risks" {
		t.Fatalf("markdown append = %+v, %v", out, err)
	}
	_, src, err := h.handleSource(ctx, nil, sourceInput{ID: created.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got := docHeadings(t, src.Content); !slices.Equal(
		got,
		[]string{"Alpha", "Next steps", "Risks"},
	) {
		t.Errorf("headings = %v", got)
	}
	if !strings.Contains(src.Content, "Ship the patch route") {
		t.Error("the replaced section's list did not reach the source")
	}
}

func TestPatchSectionToolRefusesAStaleBase(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()
	_, created, err := h.handleCreate(ctx, nil, createInput{Title: "T", Content: twoSectionDoc})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = h.handlePatchSection(ctx, nil, patchSectionInput{
		ID: created.ID, Op: "remove", Anchor: "alpha", BaseVersion: 5,
	})
	var conflict *docedit.VersionConflictError
	if !errors.As(err, &conflict) || conflict.Current != 1 {
		t.Fatalf("stale base: err = %v, want a version conflict naming version 1", err)
	}
}

// A section's local image is stored only once the section compiles, and
// the stored section names the served path.
func TestPatchSectionToolStoresLocalImages(t *testing.T) {
	h, dir := imageHandlers(t)
	ctx := context.Background()
	_, created, err := h.handleCreate(ctx, nil, createInput{Title: "T", Content: twoSectionDoc})
	if err != nil {
		t.Fatal(err)
	}
	path, _ := writePNG(t, t.TempDir(), "shot.png")
	refused := `{"h":"Shot","tone":"chartreuse","blocks":[{"t":"image","src":"` + path + `","alt":"a shot"}]}`
	if _, _, err := h.handlePatchSection(ctx, nil, patchSectionInput{
		ID: created.ID, Op: "append", Section: refused,
	}); !errors.Is(err, docedit.ErrInvalid) {
		t.Fatalf("refused section: err = %v, want ErrInvalid", err)
	}
	if files := storedFiles(t, dir); len(files) != 0 {
		t.Fatalf("a refused section stored images %v", files)
	}
	good := `{"h":"Shot","blocks":[{"t":"image","src":"` + path + `","alt":"a shot"}]}`
	if _, _, err := h.handlePatchSection(ctx, nil, patchSectionInput{
		ID: created.ID, Op: "append", Section: good,
	}); err != nil {
		t.Fatal(err)
	}
	files := storedFiles(t, dir)
	_, src, _ := h.handleSource(ctx, nil, sourceInput{ID: created.ID})
	if len(files) != 1 || !strings.Contains(src.Content, "/img/"+files[0]) {
		t.Fatalf("stored images %v, source %s", files, src.Content)
	}
}

// TestRefusedPatchesStoreNoImage pins the dry run before the image commit:
// a patch carrying a local image that its edit would refuse, for a stale
// base_version, a missing anchor, or an anchor it would repeat, writes no
// file, and the page keeps its version.
func TestRefusedPatchesStoreNoImage(t *testing.T) {
	h, dir := imageHandlers(t)
	ctx := context.Background()
	_, created, err := h.handleCreate(ctx, nil, createInput{Title: "T", Content: twoSectionDoc})
	if err != nil {
		t.Fatal(err)
	}
	path, _ := writePNG(t, t.TempDir(), "shot.png")
	withImage := func(heading string) string {
		return `{"h":"` + heading + `","blocks":[{"t":"image","src":"` + path + `","alt":"a shot"}]}`
	}
	refusals := map[string]patchSectionInput{
		"stale base_version": {
			ID: created.ID, Op: "append", Section: withImage("Shot"), BaseVersion: 5,
		},
		"missing anchor": {
			ID: created.ID, Op: "insert_after", Anchor: "zeta", Section: withImage("Shot"),
		},
		"repeated anchor": {ID: created.ID, Op: "append", Section: withImage("Alpha")},
	}
	for name, in := range refusals {
		if _, _, err := h.handlePatchSection(ctx, nil, in); err == nil {
			t.Errorf("%s: the patch was accepted", name)
		}
	}
	if files := storedFiles(t, dir); len(files) != 0 {
		t.Fatalf("refused patches stored images %v", files)
	}
	if p, err := h.store.GetMeta(ctx, created.ID); err != nil || p.Version != 1 {
		t.Fatalf("page after refused patches = %+v, %v; want version 1", p, err)
	}
}

func TestSharedPatchSectionIsAuthorOnly(t *testing.T) {
	h, _ := sharedHandlers(t)
	ctx := context.Background()
	key := author.NewKey()
	_, created, err := h.handleCreateShared(ctx, request(key, nil), sharedCreateInput{
		createInput: createInput{Title: "T", Content: twoSectionDoc},
	})
	if err != nil {
		t.Fatal(err)
	}
	in := patchSectionInput{ID: created.ID, Op: "remove", Anchor: "alpha"}
	if _, _, err := h.handlePatchSection(ctx, request(author.NewKey(), nil), in); !errors.Is(
		err, author.ErrMismatch,
	) {
		t.Errorf("wrong key: err = %v, want ErrMismatch", err)
	}
	if _, out, err := h.handlePatchSection(ctx, request(key, nil), in); err != nil ||
		out.Version != 2 {
		t.Errorf("author: %+v, %v", out, err)
	}
}

// present_source with a version answers from the page's history, so an
// earlier version can be read back and restored.
func TestSourceReturnsAnEarlierVersion(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()
	_, created, err := h.handleCreate(ctx, nil, createInput{Title: "T", Content: twoSectionDoc})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.handlePatchSection(ctx, nil, patchSectionInput{
		ID: created.ID, Op: "remove", Anchor: "alpha",
	}); err != nil {
		t.Fatal(err)
	}
	_, old, err := h.handleSource(ctx, nil, sourceInput{ID: created.ID, Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	if old.Version != 1 || old.ContentFormat != "doc" ||
		!slices.Equal(
			docHeadings(t, old.Content),
			[]string{"Alpha", "Next steps"},
		) || old.URL == "" {
		t.Fatalf("version 1 source = %+v", old)
	}
	_, cur, _ := h.handleSource(ctx, nil, sourceInput{ID: created.ID})
	if cur.Version != 2 || !slices.Equal(docHeadings(t, cur.Content), []string{"Next steps"}) {
		t.Fatalf("current source = %+v", cur)
	}
	if _, _, err := h.handleSource(ctx, nil, sourceInput{ID: created.ID, Version: 9}); err == nil {
		t.Error("a version the page never had must be an error")
	}
	// A shared instance serves a wrapped store, which keeps no history,
	// even when the store under the wrapper records it.
	shared, _ := handlersOver(t, store.WithoutExpired)
	key := author.NewKey()
	_, sp, err := shared.handleCreateShared(ctx, request(key, nil), sharedCreateInput{
		createInput: createInput{Title: "T", Content: twoSectionDoc},
	})
	if err != nil {
		t.Fatal(err)
	}
	title := "T2"
	if _, _, err := shared.handleUpdate(ctx, request(key, nil), updateInput{ID: sp.ID, Title: &title}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := shared.handleSource(ctx, nil, sourceInput{ID: sp.ID, Version: 1}); !errors.Is(
		err,
		errNoHistory,
	) {
		t.Errorf("earlier version on a shared instance: err = %v, want errNoHistory", err)
	}
}
