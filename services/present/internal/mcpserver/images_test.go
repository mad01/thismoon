package mcpserver

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	present "github.com/mad01/thismoon/services/present"
	"github.com/mad01/thismoon/services/present/internal/images"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// imageHandlers builds local-mode handlers with an image store beside the
// page store, the way present mcp runs, and returns the workdir.
func imageHandlers(t *testing.T) (*handlers, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.NewFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	h := &handlers{
		store:   st,
		baseURL: "http://localhost:7423",
		now:     time.Now,
		open:    func(string) error { return nil },
		images:  images.New(dir),
	}
	return h, dir
}

// writePNG writes a small PNG under dir and returns its path and bytes.
func writePNG(t *testing.T, dir, name string) (string, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path, buf.Bytes()
}

func imageDoc(src string) string {
	return `{"sections":[{"h":"S","blocks":[{"t":"columns","cols":[` +
		`[{"t":"image","src":"` + src + `","alt":"a shot","caption":"the shot"}],` +
		`[{"t":"p","text":"beside it"}]]}]}]}`
}

func storedFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, "images"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// A local file named by an image block is copied into the image store
// under its hash, and the page keeps the served path in its source and
// its HTML. The same file brought in twice is stored once.
func TestCreateStoresLocalImage(t *testing.T) {
	h, dir := imageHandlers(t)
	ctx := context.Background()
	src, data := writePNG(t, t.TempDir(), "shot.png")
	want := images.Name(data, "png")

	_, created, err := h.handleCreate(ctx, nil, createInput{Title: "T", Content: imageDoc(src)})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	doc, err := h.store.LoadDoc(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doc), `"src":"/img/`+want+`"`) ||
		strings.Contains(string(doc), src) {
		t.Errorf("doc.json keeps the local path: %s", doc)
	}
	p, _ := h.store.Get(ctx, created.ID)
	if !strings.Contains(p.Content, `<img src="/img/`+want+`" alt="a shot" loading="lazy">`) {
		t.Errorf("content.html: %s", p.Content)
	}
	got, err := os.ReadFile(filepath.Join(dir, "images", want))
	if err != nil || !bytes.Equal(got, data) {
		t.Errorf("stored image: %v", err)
	}

	if _, _, err := h.handleCreate(ctx, nil, createInput{Title: "U", Deck: imageDoc(src)}); err != nil {
		t.Fatalf("second create: %v", err)
	}
	if files := storedFiles(t, dir); len(files) != 1 || files[0] != want {
		t.Errorf("images dir = %v, want [%s]", files, want)
	}
}

// A ~ path reaches the same file as its absolute form.
func TestCreateImageTildePath(t *testing.T) {
	h, _ := imageHandlers(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	_, data := writePNG(t, home, "shot.png")

	_, created, err := h.handleCreate(
		context.Background(), nil, createInput{Title: "T", Content: imageDoc("~/shot.png")},
	)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	doc, _ := h.store.LoadDoc(context.Background(), created.ID)
	if !strings.Contains(string(doc), images.Served(images.Name(data, "png"))) {
		t.Errorf("doc.json: %s", doc)
	}
}

// A file the store refuses leaves nothing behind, and so does a Doc that
// fails to compile after its image was read.
func TestCreateImageRefusals(t *testing.T) {
	h, dir := imageHandlers(t)
	ctx := context.Background()
	files := t.TempDir()
	text := filepath.Join(files, "notes.txt")
	if err := os.WriteFile(text, []byte("not an image at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	svg := filepath.Join(files, "mark.svg")
	if err := os.WriteFile(svg, []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"/>`), 0o644); err != nil {
		t.Fatal(err)
	}
	big := filepath.Join(files, "big.png")
	if err := os.WriteFile(big, make([]byte, present.MaxImageBytes+1), 0o644); err != nil {
		t.Fatal(err)
	}
	good, _ := writePNG(t, files, "good.png")

	cases := []struct {
		name    string
		content string
		want    string
	}{
		{"relative path", imageDoc("shot.png"), "want an absolute path"},
		{"missing file", imageDoc(filepath.Join(files, "nope.png")), "no such file"},
		{"text file", imageDoc(text), "want png, jpeg, gif, or webp"},
		{"svg", imageDoc(svg), "want png, jpeg, gif, or webp"},
		{"over the cap", imageDoc(big), "over the 2 MiB cap"},
		{
			"compile fails after the read",
			`{"sections":[{"h":"S","blocks":[{"t":"image","src":"` + good + `","alt":""}]}]}`,
			"alt is required",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := h.handleCreate(ctx, nil, createInput{Title: "T", Content: c.content})
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want %q", err, c.want)
			}
			if !strings.HasPrefix(err.Error(), "content: ") {
				t.Errorf("err = %v, want the content prefix", err)
			}
		})
	}
	if files := storedFiles(t, dir); files != nil {
		t.Errorf("refused images were stored: %v", files)
	}
	if n, _ := h.store.ListMeta(ctx); len(n) != 0 {
		t.Errorf("a refused create stored a page")
	}
}

// present_update brings a local image in the same way, for the content and
// the deck, and a Doc that names URLs or stored paths only passes through
// byte for byte.
func TestUpdateStoresLocalImage(t *testing.T) {
	h, dir := imageHandlers(t)
	ctx := context.Background()
	src, data := writePNG(t, t.TempDir(), "shot.png")
	want := images.Served(images.Name(data, "png"))

	_, created, err := h.handleCreate(ctx, nil, createInput{Title: "T", Content: "<p>v1</p>"})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = h.handleUpdate(
		ctx,
		nil,
		updateInput{ID: created.ID, Content: ptrStr(imageDoc(src)), Deck: ptrStr(imageDoc(src))},
	)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	doc, _ := h.store.LoadDoc(ctx, created.ID)
	deck, _ := h.store.LoadDeckSource(ctx, created.ID)
	for name, got := range map[string][]byte{"doc.json": doc, "deck.json": deck} {
		if !strings.Contains(string(got), `"src":"`+want+`"`) {
			t.Errorf("%s: %s", name, got)
		}
	}
	if files := storedFiles(t, dir); len(files) != 1 {
		t.Errorf("images dir = %v", files)
	}

	// A Doc already on stored paths and URLs is passed to the compiler as
	// given: the canonical JSON is what Compile makes of the input alone.
	passthrough := `{"sections":[{"h":"S","blocks":[` +
		`{"t":"image","src":"` + want + `","alt":"a"},` +
		`{"t":"image","src":"https://example.com/b.png","alt":"b"}]}]}`
	if _, _, err := h.handleUpdate(ctx, nil, updateInput{ID: created.ID, Content: ptrStr(passthrough)}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if doc, _ := h.store.LoadDoc(ctx, created.ID); string(doc) != passthrough {
		t.Errorf("doc.json = %s, want the input unchanged", doc)
	}
}

// A shared instance has no image store: a local path, and a stored path
// it could not serve, are refused with the URL as the way out; a URL is
// taken as on a local instance.
func TestSharedRefusesLocalImage(t *testing.T) {
	h, _ := sharedHandlers(t)
	ctx := context.Background()
	src, _ := writePNG(t, t.TempDir(), "shot.png")
	for _, bad := range []string{src, "~/shot.png", images.Served(images.Name([]byte("x"), "png"))} {
		_, _, err := h.handleCreateShared(
			ctx,
			request("k", nil),
			sharedCreateInput{createInput: createInput{Title: "T", Content: imageDoc(bad)}},
		)
		if err == nil || !strings.Contains(err.Error(), "use an http or https URL") {
			t.Errorf("%s: err = %v, want the URL hint", bad, err)
		}
	}
	_, _, err := h.handleCreateShared(
		ctx,
		request("k", nil),
		sharedCreateInput{
			createInput: createInput{Title: "T", Content: imageDoc("https://example.com/a.png")},
		},
	)
	if err != nil {
		t.Errorf("URL image refused on a shared instance: %v", err)
	}
}

// Legacy HTML content and an unparsable Doc are not the ingest's to judge:
// they reach the compiler as they came.
func TestImageIngestPassesNonDocsThrough(t *testing.T) {
	g := (&handlers{}).newImageIngest()
	for _, s := range []string{"", "  ", "<p>html</p>", "{not json", `{"sections":[]}`} {
		if out, err := g.doc(s); err != nil || out != s {
			t.Errorf("doc(%q) = %q, %v", s, out, err)
		}
	}
}
