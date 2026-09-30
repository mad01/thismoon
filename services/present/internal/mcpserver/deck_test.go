package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/kit/mcptest"
	"github.com/mad01/thismoon/services/present/internal/author"
	"github.com/mad01/thismoon/services/present/internal/render"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// deckDoc is a two-slide deck in the Doc shape.
func deckDoc(heading string) string {
	return toJSON(map[string]any{
		"summary": "Deck summary.",
		"sections": []map[string]any{
			{"h": heading, "blocks": []map[string]any{{"t": "p", "text": "slide one"}}},
			{"h": "Second", "blocks": []map[string]any{{"t": "list", "items": []string{"a", "b"}}}},
		},
	})
}

// briefDoc is a one-section brief in the Doc shape.
func briefDoc() string {
	return toJSON(map[string]any{
		"sections": []map[string]any{
			{"h": "Brief", "blocks": []map[string]any{{"t": "p", "text": "prose"}}},
		},
	})
}

// pageFiles lists the files under the page's directory in the test store.
func pageFiles(t *testing.T, h *handlers, id string) []string {
	t.Helper()
	fs, ok := h.store.(*store.FS)
	if !ok {
		t.Fatal("test store is not the filesystem store")
	}
	// The FS keeps pages under <dir>/pages/<id>; GetMeta proves the id and
	// the directory name agree.
	if _, err := fs.GetMeta(t.Context(), id); err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	dir := filepath.Join(fsDir(t, h), "pages", id)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read page dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	slices.Sort(names)
	return names
}

// deckHandlers builds local handlers over a filesystem store rooted at a
// known directory, so tests can look at the files a tool wrote.
func deckHandlers(t *testing.T) (*handlers, *[]string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.NewFS(dir)
	if err != nil {
		t.Fatalf("store.NewFS: %v", err)
	}
	opened := &[]string{}
	h := &handlers{
		store:   st,
		baseURL: "http://localhost:7423",
		now:     time.Now,
		open:    func(url string) error { *opened = append(*opened, url); return nil },
	}
	testDirs[h] = dir
	return h, opened
}

// testDirs remembers each handlers' store directory for pageFiles.
var testDirs = map[*handlers]string{}

func fsDir(t *testing.T, h *handlers) string {
	t.Helper()
	dir, ok := testDirs[h]
	if !ok {
		t.Fatal("handlers were not built by deckHandlers")
	}
	return dir
}

func TestCreateDeckOnlyPage(t *testing.T) {
	h, _ := deckHandlers(t)
	ctx := context.Background()

	_, created, err := h.handleCreate(ctx, nil, createInput{Title: "D", Deck: deckDoc("First")})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !created.HasDeck {
		t.Error("has_deck = false for a deck-only page")
	}
	wantDeck := "http://localhost:7423/p/" + created.ID + "/deck"
	if created.DeckURL != wantDeck {
		t.Errorf("deck_url = %q, want %q", created.DeckURL, wantDeck)
	}
	if created.URL != wantDeck {
		t.Errorf("url = %q, want the deck url %q for a page without content", created.URL, wantDeck)
	}
	files := pageFiles(t, h, created.ID)
	for _, want := range []string{"deck.html", "deck.json"} {
		if !slices.Contains(files, want) {
			t.Errorf("page dir %v lacks %s", files, want)
		}
	}
	if slices.Contains(files, "doc.json") {
		t.Errorf("page dir %v has a doc.json for a page without content", files)
	}

	_, read, err := h.handleRead(ctx, nil, readInput{ID: created.ID})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if read.Content != "" || !read.HasDeck || read.DeckURL != wantDeck || read.URL != wantDeck {
		t.Errorf("read = %+v, want empty content and deck urls", read)
	}
	if !strings.Contains(
		read.Deck,
		"<wk-section-heading data-fixation>First</wk-section-heading>",
	) ||
		!strings.Contains(read.Deck, `class="brief-title"`) {
		t.Errorf("deck html not rendered from the Doc:\n%s", read.Deck)
	}
}

func TestCreateWithContentAndDeckOpensOnContent(t *testing.T) {
	h, _ := deckHandlers(t)
	ctx := context.Background()

	_, created, err := h.handleCreate(ctx, nil, createInput{
		Title: "Both", Content: briefDoc(), Deck: deckDoc("First"),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if want := "http://localhost:7423/p/" + created.ID; created.URL != want {
		t.Errorf("url = %q, want the content url %q", created.URL, want)
	}
	if created.DeckURL != created.URL+"/deck" || !created.HasDeck {
		t.Errorf("deck fields = %+v", created)
	}
	files := pageFiles(t, h, created.ID)
	for _, want := range []string{"content.html", "doc.json", "deck.html", "deck.json"} {
		if !slices.Contains(files, want) {
			t.Errorf("page dir %v lacks %s", files, want)
		}
	}
}

func TestCreateRefusesNeitherContentNorDeck(t *testing.T) {
	h, _ := deckHandlers(t)
	ctx := context.Background()
	for _, in := range []createInput{
		{Title: "Empty"},
		{Title: "Blank", Content: "  ", Deck: "\n"},
	} {
		_, _, err := h.handleCreate(ctx, nil, in)
		if !errors.Is(err, errNoRendition) {
			t.Errorf("create %+v: err = %v, want %v", in, err, errNoRendition)
		}
	}
	if _, err := os.ReadDir(filepath.Join(fsDir(t, h), "pages")); err != nil {
		t.Fatal(err)
	} else if _, out, err := h.handleList(ctx, nil, struct{}{}); err != nil || len(out.Pages) != 0 {
		t.Errorf("a refused create left pages behind: %+v, %v", out, err)
	}
}

func TestCreateRefusesADeckThatIsNotDocJSON(t *testing.T) {
	h, _ := deckHandlers(t)
	ctx := context.Background()

	_, _, err := h.handleCreate(ctx, nil, createInput{Title: "D", Deck: "<p>not a doc</p>"})
	if err == nil || !strings.Contains(err.Error(), "deck: want Doc JSON") {
		t.Errorf("html deck: err = %v, want a want-Doc-JSON error", err)
	}
	_, _, err = h.handleCreate(ctx, nil, createInput{Title: "D", Deck: `{"sections": nope}`})
	if err == nil || !strings.HasPrefix(err.Error(), "deck:") {
		t.Errorf("malformed deck: err = %v, want a deck: parse error", err)
	}
}

func TestUpdateReplacesAndRemovesTheDeck(t *testing.T) {
	h, _ := deckHandlers(t)
	ctx := context.Background()
	_, created, err := h.handleCreate(ctx, nil, createInput{
		Title: "Both", Content: briefDoc(), Deck: deckDoc("First"),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, updated, err := h.handleUpdate(ctx, nil, updateInput{
		ID: created.ID, Deck: ptrStr(deckDoc("Replaced")),
	})
	if err != nil {
		t.Fatalf("update deck: %v", err)
	}
	if updated.Version != 2 || !updated.HasDeck || updated.DeckURL == "" {
		t.Errorf("update output = %+v, want version 2 with deck urls", updated)
	}
	src, err := h.store.LoadDeckSource(ctx, created.ID)
	if err != nil {
		t.Fatalf("LoadDeckSource: %v", err)
	}
	var doc render.Doc
	if err := json.Unmarshal(src, &doc); err != nil || doc.Sections[0].Heading != "Replaced" {
		t.Errorf("deck.json after update = %s (%v), want the replaced deck", src, err)
	}
	_, read, _ := h.handleRead(ctx, nil, readInput{ID: created.ID})
	if !strings.Contains(read.Deck, "Replaced") || strings.Contains(read.Deck, "First") {
		t.Errorf("deck html not replaced:\n%s", read.Deck)
	}

	_, removed, err := h.handleUpdate(ctx, nil, updateInput{ID: created.ID, Deck: ptrStr("")})
	if err != nil {
		t.Fatalf("remove deck: %v", err)
	}
	if removed.Version != 3 || removed.HasDeck || removed.DeckURL != "" {
		t.Errorf("remove output = %+v, want version 3 without deck", removed)
	}
	if removed.URL != "http://localhost:7423/p/"+created.ID {
		t.Errorf("url after removing the deck = %q", removed.URL)
	}
	if h.store.HasDeckSource(ctx, created.ID) {
		t.Error("deck.json survived removing the deck")
	}
	_, read, _ = h.handleRead(ctx, nil, readInput{ID: created.ID})
	if read.Deck != "" || read.HasDeck {
		t.Errorf("read after remove = has_deck %v deck %q", read.HasDeck, read.Deck)
	}
}

func TestUpdateRefusesToStripTheLastRendition(t *testing.T) {
	h, _ := deckHandlers(t)
	ctx := context.Background()

	// Deck-only page: removing the deck leaves nothing.
	_, deckOnly, err := h.handleCreate(ctx, nil, createInput{Title: "D", Deck: deckDoc("Only")})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, _, err = h.handleUpdate(ctx, nil, updateInput{ID: deckOnly.ID, Deck: ptrStr("")})
	if !errors.Is(err, errLastRendition) {
		t.Errorf("remove the only deck: err = %v, want %v", err, errLastRendition)
	}
	p, _ := h.store.Get(ctx, deckOnly.ID)
	if p.Version != 1 || !p.HasDeck || !h.store.HasDeckSource(ctx, deckOnly.ID) {
		t.Errorf("refused update changed the page: version %d has_deck %v", p.Version, p.HasDeck)
	}

	// Content-only page: clearing the content leaves nothing.
	_, briefOnly, err := h.handleCreate(ctx, nil, createInput{Title: "B", Content: briefDoc()})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, _, err = h.handleUpdate(ctx, nil, updateInput{ID: briefOnly.ID, Content: ptrStr("")})
	if !errors.Is(err, errLastRendition) {
		t.Errorf("clear the only content: err = %v, want %v", err, errLastRendition)
	}

	// Both at once is refused too; swapping one for the other is fine.
	_, both, _ := h.handleCreate(ctx, nil, createInput{
		Title: "Both", Content: briefDoc(), Deck: deckDoc("X"),
	})
	_, _, err = h.handleUpdate(ctx, nil, updateInput{
		ID: both.ID, Content: ptrStr(""), Deck: ptrStr(""),
	})
	if !errors.Is(err, errLastRendition) {
		t.Errorf("clear both: err = %v, want %v", err, errLastRendition)
	}
	if _, out, err := h.handleUpdate(ctx, nil, updateInput{ID: both.ID, Content: ptrStr("")}); err != nil {
		t.Errorf("clear content while the deck stays: %v", err)
	} else if out.URL != out.DeckURL {
		t.Errorf("url after clearing content = %q, want the deck url %q", out.URL, out.DeckURL)
	}
}

func TestSourceReturnsTheDeckDocForRoundTrip(t *testing.T) {
	h, _ := deckHandlers(t)
	ctx := context.Background()
	_, created, err := h.handleCreate(ctx, nil, createInput{
		Title: "Both", Content: briefDoc(), Deck: deckDoc("First"),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, src, err := h.handleSource(ctx, nil, sourceInput{ID: created.ID})
	if err != nil {
		t.Fatalf("source: %v", err)
	}
	if src.ContentFormat != "doc" || src.Deck == "" || src.DeckURL != created.DeckURL {
		t.Fatalf("source = %+v, want doc content, a deck, and the deck url", src)
	}
	var doc render.Doc
	if err := json.Unmarshal([]byte(src.Deck), &doc); err != nil {
		t.Fatalf("deck source is not Doc JSON: %v", err)
	}
	if len(doc.Sections) != 2 || doc.Sections[0].Heading != "First" {
		t.Errorf("deck source lost data: %+v", doc)
	}
	if _, _, err := h.handleUpdate(ctx, nil, updateInput{ID: created.ID, Deck: &src.Deck}); err != nil {
		t.Fatalf("round-trip update with the deck source failed: %v", err)
	}
	_, again, _ := h.handleSource(ctx, nil, sourceInput{ID: created.ID})
	if again.Deck != src.Deck {
		t.Errorf("deck source changed across a round trip:\n%s\n%s", src.Deck, again.Deck)
	}

	_, briefOnly, _ := h.handleCreate(ctx, nil, createInput{Title: "B", Content: briefDoc()})
	_, src, err = h.handleSource(ctx, nil, sourceInput{ID: briefOnly.ID})
	if err != nil {
		t.Fatalf("source: %v", err)
	}
	if src.Deck != "" || src.DeckURL != "" {
		t.Errorf("deck fields on a page without a deck: %+v", src)
	}
}

func TestListCarriesTheRenditionFlags(t *testing.T) {
	h, _ := deckHandlers(t)
	ctx := context.Background()
	_, deckOnly, _ := h.handleCreate(ctx, nil, createInput{Title: "D", Deck: deckDoc("X")})
	_, briefOnly, _ := h.handleCreate(ctx, nil, createInput{Title: "B", Content: briefDoc()})
	_, both, _ := h.handleCreate(ctx, nil, createInput{
		Title: "Both", Content: briefDoc(), Deck: deckDoc("Y"),
	})

	_, out, err := h.handleList(ctx, nil, struct{}{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	byID := map[string]listItem{}
	for _, p := range out.Pages {
		byID[p.ID] = p
	}
	cases := []struct {
		id                string
		brief, deck       bool
		urlIsDeck, hasURL bool
	}{
		{deckOnly.ID, false, true, true, true},
		{briefOnly.ID, true, false, false, false},
		{both.ID, true, true, false, true},
	}
	for _, tc := range cases {
		item, ok := byID[tc.id]
		if !ok {
			t.Fatalf("page %s missing from the list", tc.id)
		}
		if item.HasBrief != tc.brief || item.HasDeck != tc.deck {
			t.Errorf(
				"%s: has_brief %v has_deck %v, want %v %v",
				tc.id,
				item.HasBrief,
				item.HasDeck,
				tc.brief,
				tc.deck,
			)
		}
		deckURL := "http://localhost:7423/p/" + tc.id + "/deck"
		if (item.DeckURL != "") != tc.hasURL || (tc.hasURL && item.DeckURL != deckURL) {
			t.Errorf("%s: deck_url = %q, want set=%v", tc.id, item.DeckURL, tc.hasURL)
		}
		if (item.URL == deckURL) != tc.urlIsDeck {
			t.Errorf("%s: url = %q, want deck url = %v", tc.id, item.URL, tc.urlIsDeck)
		}
	}
}

func TestOpenDeck(t *testing.T) {
	h, opened := deckHandlers(t)
	ctx := context.Background()
	_, both, _ := h.handleCreate(ctx, nil, createInput{
		Title: "Both", Content: briefDoc(), Deck: deckDoc("X"),
	})
	_, briefOnly, _ := h.handleCreate(ctx, nil, createInput{Title: "B", Content: briefDoc()})

	_, out, err := h.handleOpen(ctx, nil, openInput{ID: both.ID, Deck: true})
	if err != nil {
		t.Fatalf("open deck: %v", err)
	}
	if out.URL != both.DeckURL || !out.Opened {
		t.Errorf("open deck = %+v, want %s", out, both.DeckURL)
	}
	_, out, err = h.handleOpen(ctx, nil, openInput{ID: both.ID})
	if err != nil || out.URL != both.URL {
		t.Errorf("open content = %+v, %v; want %s", out, err, both.URL)
	}
	if !slices.Equal(*opened, []string{both.DeckURL, both.URL}) {
		t.Errorf("opener called with %v", *opened)
	}

	_, _, err = h.handleOpen(ctx, nil, openInput{ID: briefOnly.ID, Deck: true})
	if !errors.Is(err, store.ErrNoDeck) {
		t.Errorf("open deck of a page without one: err = %v, want ErrNoDeck", err)
	}
	if len(*opened) != 2 {
		t.Errorf("opener called for a missing deck: %v", *opened)
	}
}

func TestDeckToolWritesCommands(t *testing.T) {
	h, _ := deckHandlers(t)
	ctx := context.Background()
	_, page, _ := h.handleCreate(ctx, nil, createInput{Title: "D", Deck: deckDoc("X")})
	_, briefOnly, _ := h.handleCreate(ctx, nil, createInput{Title: "B", Content: briefDoc()})

	_, out, err := h.handleDeck(ctx, nil, deckInput{ID: page.ID, Action: "start"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if out.Seq != 1 || out.Action != store.DeckStart || out.Slide != 0 ||
		out.DeckURL != page.DeckURL {
		t.Errorf("start = %+v", out)
	}
	_, out, err = h.handleDeck(ctx, nil, deckInput{ID: page.ID, Action: "goto", Slide: 3})
	if err != nil {
		t.Fatalf("goto: %v", err)
	}
	if out.Seq != 2 || out.Action != store.DeckGoto || out.Slide != 3 {
		t.Errorf("goto = %+v", out)
	}
	dc, ok := h.store.(store.DeckController)
	if !ok {
		t.Fatal("test store does not relay deck commands")
	}
	cmd, err := dc.DeckCommand(ctx, page.ID)
	if err != nil || cmd.Seq != 2 || cmd.Action != store.DeckGoto || cmd.Slide != 3 {
		t.Errorf("stored command = %+v, %v", cmd, err)
	}
	p, _ := h.store.Get(ctx, page.ID)
	if p.Version != 1 {
		t.Errorf("a deck command bumped the version to %d", p.Version)
	}

	_, _, err = h.handleDeck(ctx, nil, deckInput{ID: page.ID, Action: "rewind"})
	if err == nil || !strings.Contains(err.Error(), "unknown deck action") {
		t.Errorf("bad action: err = %v", err)
	}
	_, _, err = h.handleDeck(ctx, nil, deckInput{ID: page.ID, Action: "goto"})
	if err == nil || !strings.Contains(err.Error(), "goto needs a slide") {
		t.Errorf("goto without slide: err = %v", err)
	}
	_, _, err = h.handleDeck(ctx, nil, deckInput{ID: briefOnly.ID, Action: "next"})
	if !errors.Is(err, store.ErrNoDeck) {
		t.Errorf("command to a page without a deck: err = %v, want ErrNoDeck", err)
	}
	_, _, err = h.handleDeck(ctx, nil, deckInput{ID: "missing0000", Action: "next"})
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("command to a missing page: err = %v, want ErrNotFound", err)
	}
}

func TestDeckToolNeedsARelayingStore(t *testing.T) {
	fs, err := store.NewFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// A wrapped store hides the filesystem store's relay, the way shared
	// mode wraps it; the tool must then be absent and its handler refuse.
	wrapped := &handlers{
		store: store.WithoutExpired(fs, time.Now),
		now:   time.Now,
		open:  func(string) error { return nil },
	}
	if _, ok := wrapped.deckController(); ok {
		t.Fatal("a wrapped store must not relay deck commands")
	}
	_, _, err = wrapped.handleDeck(context.Background(), nil, deckInput{ID: "x", Action: "next"})
	if err == nil || !strings.Contains(err.Error(), "cannot relay") {
		t.Errorf("handler over a wrapped store: err = %v", err)
	}

	// Registration follows the same test: local over the raw store has the
	// tool, local over a wrapped store and shared mode do not.
	s, err := New("test", Config{Workdir: t.TempDir(), Port: 7423, Checks: noChecks})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(toolNames(t, s), "present_deck") {
		t.Error("local mode over the filesystem store must offer present_deck")
	}
	mcptest.VerifyToolAnnotations(t, s)

	s, err = New("test", Config{
		Store: store.WithoutExpired(fs, time.Now), Port: 7423, Checks: noChecks,
	})
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(toolNames(t, s), "present_deck") {
		t.Error("a wrapped store must not offer present_deck")
	}

	s, err = New("test", Config{Workdir: t.TempDir(), Mode: ModeShared, Checks: noChecks})
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(toolNames(t, s), "present_deck") {
		t.Error("a shared instance must not offer present_deck")
	}
}

func TestSharedCreateAndUpdateCarryTheDeck(t *testing.T) {
	h, _ := sharedHandlers(t)
	ctx := context.Background()
	key := author.NewKey()
	req := request(key, map[string]string{
		"X-Forwarded-Host": "present.example.com", "X-Forwarded-Proto": "https",
	})

	_, created, err := h.handleCreateShared(ctx, req, sharedCreateInput{
		createInput: createInput{Title: "D", Deck: deckDoc("Shared")},
	})
	if err != nil {
		t.Fatalf("shared create: %v", err)
	}
	wantDeck := "https://present.example.com/p/" + created.ID + "/deck"
	if created.URL != wantDeck || created.DeckURL != wantDeck || !created.HasDeck {
		t.Errorf("shared deck-only create = %+v, want %s", created, wantDeck)
	}
	if !h.store.HasDeckSource(ctx, created.ID) {
		t.Error("deck.json not persisted on a shared instance")
	}

	_, updated, err := h.handleUpdate(ctx, req, updateInput{
		ID: created.ID, Content: ptrStr(briefDoc()), Deck: ptrStr(deckDoc("Again")),
	})
	if err != nil {
		t.Fatalf("shared update: %v", err)
	}
	if updated.Version != 2 || updated.URL != "https://present.example.com/p/"+created.ID {
		t.Errorf("shared update = %+v, want version 2 opening on the content", updated)
	}
	_, src, err := h.handleSource(ctx, req, sourceInput{ID: created.ID})
	if err != nil || !strings.Contains(src.Deck, "Again") {
		t.Errorf("shared source = %+v, %v; want the replaced deck", src, err)
	}
	_, _, err = h.handleUpdate(ctx, request(author.NewKey(), nil), updateInput{
		ID: created.ID, Deck: ptrStr(""),
	})
	if !errors.Is(err, author.ErrMismatch) {
		t.Errorf("deck update with another key: err = %v, want ErrMismatch", err)
	}
}

// The create and update descriptions must teach the deck; the annotation
// contract checks the rest.
func TestDeckDescriptionsReachTheSchemas(t *testing.T) {
	s, err := New("test", Config{Workdir: t.TempDir(), Port: 7423, Checks: noChecks})
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range mcptest.ListTools(t, s) {
		schema := string(mustJSON(t, tool.InputSchema))
		switch tool.Name {
		case "present_create", "present_update":
			if !strings.Contains(tool.Description, "deck") || !strings.Contains(schema, `"deck"`) {
				t.Errorf("%s must describe and expose deck: %q", tool.Name, tool.Description)
			}
		case "present_open":
			if !strings.Contains(schema, `"deck"`) {
				t.Errorf("present_open must expose deck: %s", schema)
			}
		case "present_deck":
			for _, key := range []string{"start", "stop", "next", "prev", "goto", "Esc", "Space"} {
				if !strings.Contains(tool.Description, key) {
					t.Errorf("present_deck description lacks %q: %q", key, tool.Description)
				}
			}
			if !strings.Contains(schema, `"slide"`) || !strings.Contains(schema, `"action"`) {
				t.Errorf("present_deck schema = %s", schema)
			}
		}
	}
}

// Clearing the content with an empty string removes the brief and its
// source: a doc.json left behind would let present_source hand the old brief
// back and present rerender resurrect it.
func TestUpdateClearingContentDropsTheDocSource(t *testing.T) {
	h, _ := deckHandlers(t)
	ctx := context.Background()
	_, both, err := h.handleCreate(ctx, nil, createInput{
		Title: "Both", Content: briefDoc(), Deck: deckDoc("Keep"),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !h.store.HasDoc(ctx, both.ID) {
		t.Fatal("create with a Doc did not persist doc.json")
	}
	if _, _, err := h.handleUpdate(ctx, nil, updateInput{ID: both.ID, Content: ptrStr("")}); err != nil {
		t.Fatalf("clear content: %v", err)
	}
	if h.store.HasDoc(ctx, both.ID) {
		t.Error("doc.json survived clearing the content")
	}
	_, src, err := h.handleSource(ctx, nil, sourceInput{ID: both.ID})
	if err != nil {
		t.Fatalf("source: %v", err)
	}
	if src.Content != "" || src.ContentFormat != "html" || src.Deck == "" {
		t.Errorf("source after clearing content = %+v, want no content and the deck", src)
	}
}

// An update that leaves the title alone still renders under it: the Doc
// template writes the title into the hero of both renditions.
func TestUpdateWithoutTitleKeepsTheTitleInTheHero(t *testing.T) {
	h, _ := deckHandlers(t)
	ctx := context.Background()
	_, both, err := h.handleCreate(ctx, nil, createInput{
		Title: "Kept title", Content: briefDoc(), Deck: deckDoc("First"),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, _, err = h.handleUpdate(ctx, nil, updateInput{ID: both.ID, Deck: ptrStr(deckDoc("Second"))})
	if err != nil {
		t.Fatalf("update deck: %v", err)
	}
	_, _, err = h.handleUpdate(ctx, nil, updateInput{ID: both.ID, Content: ptrStr(briefDoc())})
	if err != nil {
		t.Fatalf("update content: %v", err)
	}
	p, err := h.store.Get(ctx, both.ID)
	if err != nil {
		t.Fatal(err)
	}
	const hero = `<h1 class="brief-title" data-fixation>Kept title</h1>`
	if !strings.Contains(p.Deck, hero) {
		t.Errorf("deck hero lost the title: %q", p.Deck[:min(len(p.Deck), 120)])
	}
	if !strings.Contains(p.Content, hero) {
		t.Errorf("brief hero lost the title: %q", p.Content[:min(len(p.Content), 120)])
	}
}

// A page from before decks may hold neither rendition (empty content was
// accepted then); it still takes the updates that do not touch renditions.
func TestUpdateOnAPageWithNeitherRenditionStillTakesMetadata(t *testing.T) {
	h, _ := deckHandlers(t)
	ctx := context.Background()
	old, err := h.store.Create(ctx, store.Draft{Title: "Old"})
	if err != nil {
		t.Fatal(err)
	}
	_, out, err := h.handleUpdate(ctx, nil, updateInput{ID: old.ID, Title: ptrStr("Renamed")})
	if err != nil {
		t.Fatalf("title-only update on a page with neither rendition: %v", err)
	}
	if out.Version != 2 {
		t.Errorf("version = %d, want 2", out.Version)
	}
}
