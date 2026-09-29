package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/present/internal/author"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// noControl hides the filesystem store's DeckController behind the plain
// Store interface, the way the wrapped store of a shared instance or the
// cluster store would: the server must then leave the command route out.
type noControl struct{ store.Store }

// getNoFollow performs a GET without following redirects.
func getNoFollow(t *testing.T, url string) *http.Response {
	t.Helper()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	_ = resp.Body.Close()
	return resp
}

func TestDeckPageServesShellOnlyWithADeck(t *testing.T) {
	ts, st := setup(t)
	both, _ := st.Create(t.Context(), store.Draft{
		Title: "Both", Content: "<p>brief</p>", Deck: "<h1 class=\"brief-title\">Both</h1>",
	})
	briefOnly, _ := st.Create(t.Context(), store.Draft{Title: "Brief", Content: "<p>brief</p>"})

	code, body := get(t, ts.URL+"/p/"+both.ID+"/deck")
	if code != http.StatusOK {
		t.Fatalf("deck view status = %d, want 200", code)
	}
	for _, want := range []string{`id="root"`, "/app.js", `id="deckLink"`, `id="briefLink"`} {
		if !contains(body, want) {
			t.Errorf("deck shell missing %q", want)
		}
	}
	if resp := getNoFollow(t, ts.URL+"/p/"+briefOnly.ID+"/deck"); resp.StatusCode != http.StatusFound {
		t.Errorf("deck view of a page without a deck = %d, want 302 to the brief", resp.StatusCode)
	} else if loc := resp.Header.Get("Location"); loc != "/p/"+briefOnly.ID {
		t.Errorf("Location = %q, want the brief", loc)
	}
	if code, _ := get(t, ts.URL+"/p/deadbeef00/deck"); code != http.StatusNotFound {
		t.Errorf("deck view of an unknown page = %d, want 404", code)
	}
}

func TestDeckOnlyPageRedirectsToItsDeck(t *testing.T) {
	ts, st := setup(t)
	deckOnly, _ := st.Create(t.Context(), store.Draft{
		Title: "Deck", Deck: "<h1 class=\"brief-title\">Deck</h1>",
	})
	both, _ := st.Create(t.Context(), store.Draft{
		Title: "Both", Content: "<p>brief</p>", Deck: "<h1 class=\"brief-title\">Both</h1>",
	})

	resp := getNoFollow(t, ts.URL+"/p/"+deckOnly.ID)
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("deck-only page status = %d, want 302", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/p/"+deckOnly.ID+"/deck" {
		t.Errorf("Location = %q, want the deck view", loc)
	}
	if resp := getNoFollow(t, ts.URL+"/p/"+both.ID); resp.StatusCode != http.StatusOK {
		t.Errorf("page with a brief status = %d, want 200", resp.StatusCode)
	}
}

func TestAPIPageCarriesTheDeck(t *testing.T) {
	ts, st := setup(t)
	p, _ := st.Create(t.Context(), store.Draft{
		Title: "Both", Content: "<p>brief-marker</p>", Deck: "<p>deck-marker</p>",
	})
	plain, _ := st.Create(t.Context(), store.Draft{Title: "Brief", Content: "<p>b</p>"})

	_, body := get(t, ts.URL+"/api/p/"+p.ID)
	var got apiPage
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("api body not JSON: %v (body=%q)", err, body)
	}
	if !got.HasDeck || !contains(got.Deck, "deck-marker") ||
		!contains(got.Content, "brief-marker") {
		t.Errorf("api page = %+v, want both renditions", got)
	}
	if !got.DeckControl {
		t.Error("deck_control must be true on the filesystem store")
	}

	_, body = get(t, ts.URL+"/api/p/"+plain.ID)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatalf("api body not JSON: %v", err)
	}
	if string(raw["has_deck"]) != "false" || string(raw["deck"]) != `""` {
		t.Errorf("page without a deck: has_deck=%s deck=%s", raw["has_deck"], raw["deck"])
	}
	// The keys the server emits are the ones app.js reads; pin them together.
	for _, want := range []string{"data.deck", "data.has_deck", "data.deck_control"} {
		if !contains(string(appJS), want) {
			t.Errorf("app.js does not read %s from the page JSON", want)
		}
	}
}

func TestDeckCommandRouteAnswersTheLastCommand(t *testing.T) {
	ts, st := setup(t)
	raw := st.(*store.FS)
	p, _ := st.Create(t.Context(), store.Draft{Title: "Deck", Deck: "<p>d</p>"})
	brief, _ := st.Create(t.Context(), store.Draft{Title: "Brief", Content: "<p>b</p>"})

	code, body := get(t, ts.URL+"/p/"+p.ID+"/deck/command")
	if code != http.StatusOK {
		t.Fatalf("command status = %d, want 200", code)
	}
	var got deckCommands
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("commands not JSON: %v (body=%q)", err, body)
	}
	if got.Seq != 0 || len(got.Commands) != 0 {
		t.Errorf("before any command = %+v, want seq 0 and no commands", got)
	}

	if _, err := raw.SendDeckCommand(t.Context(), p.ID, store.DeckGoto, 3); err != nil {
		t.Fatalf("send: %v", err)
	}
	if _, err := raw.SendDeckCommand(t.Context(), p.ID, store.DeckStart, 0); err != nil {
		t.Fatalf("send: %v", err)
	}
	_, body = get(t, ts.URL+"/p/"+p.ID+"/deck/command")
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("commands not JSON: %v", err)
	}
	if got.Seq != 2 || len(got.Commands) != 2 || got.Commands[0].Action != store.DeckGoto ||
		got.Commands[0].Slide != 3 || got.Commands[1].Action != store.DeckStart {
		t.Errorf("commands = %+v, want seq 2 with goto 3 then start", got)
	}
	// A tab that applied seq 1 asks for what came after it.
	_, body = get(t, ts.URL+"/p/"+p.ID+"/deck/command?after=1")
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("commands not JSON: %v", err)
	}
	if got.Seq != 2 || len(got.Commands) != 1 || got.Commands[0].Seq != 2 {
		t.Errorf("after=1: %+v, want only seq 2", got)
	}
	if code, _ := get(t, ts.URL+"/p/"+p.ID+"/deck/command?after=x"); code != http.StatusBadRequest {
		t.Errorf("after=x: %d, want 400", code)
	}

	if code, _ := get(t, ts.URL+"/p/"+brief.ID+"/deck/command"); code != http.StatusNotFound {
		t.Errorf("command for a page without a deck = %d, want 404", code)
	}
	if code, _ := get(t, ts.URL+"/p/deadbeef00/deck/command"); code != http.StatusNotFound {
		t.Errorf("command for an unknown page = %d, want 404", code)
	}
}

func TestDeckCommandRouteNeedsAControllingStore(t *testing.T) {
	dir := t.TempDir()
	fs, err := store.NewFS(dir)
	if err != nil {
		t.Fatalf("store.NewFS: %v", err)
	}
	ts := httptest.NewServer(New(noControl{fs}, Options{Workdir: dir, Info: testInfo}).Handler())
	t.Cleanup(ts.Close)
	p, _ := fs.Create(t.Context(), store.Draft{Title: "Deck", Deck: "<p>d</p>"})

	if code, _ := get(t, ts.URL+"/p/"+p.ID+"/deck/command"); code != http.StatusNotFound {
		t.Errorf("command route on a store without control = %d, want 404", code)
	}
	_, body := get(t, ts.URL+"/api/p/"+p.ID)
	var got apiPage
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("api body not JSON: %v", err)
	}
	if got.DeckControl {
		t.Error("deck_control must be false when the store cannot relay commands")
	}
	// The deck view itself still works: only the remote control is gone.
	if code, _ := get(t, ts.URL+"/p/"+p.ID+"/deck"); code != http.StatusOK {
		t.Errorf("deck view = %d, want 200", code)
	}
}

func TestAPIPagesFlagTheDeck(t *testing.T) {
	ts, st := setup(t)
	both, _ := st.Create(
		t.Context(),
		store.Draft{Title: "Both", Content: "<p>b</p>", Deck: "<p>d</p>"},
	)
	deckOnly, _ := st.Create(t.Context(), store.Draft{Title: "Deck", Deck: "<p>d</p>"})
	briefOnly, _ := st.Create(t.Context(), store.Draft{Title: "Brief", Content: "<p>b</p>"})

	pages := getAPIPages(t, ts.URL+"/api/pages")
	flags := map[string]bool{}
	for _, p := range pages.Pages {
		flags[p.ID] = p.HasDeck
	}
	want := map[string]bool{both.ID: true, deckOnly.ID: true, briefOnly.ID: false}
	for id, w := range want {
		if flags[id] != w {
			t.Errorf("page %s has_deck = %v, want %v", id, flags[id], w)
		}
	}
	if !contains(string(indexJS), "has_deck") {
		t.Error("index.js does not read has_deck from the listing")
	}
}

func TestSharedCreateAndReplaceCarryTheDeck(t *testing.T) {
	f := setupShared(t)
	key := author.NewKey()
	ctx := t.Context()

	withDeck := page("Deck")
	withDeck["deck"] = "<p>slide</p>"
	withDeck["deck_source"] = json.RawMessage(`{"sections":[{"h":"S","blocks":[]}]}`)
	out := f.create(t, key, withDeck)

	got, err := f.raw.Get(ctx, out.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.HasDeck || got.Deck != "<p>slide</p>" {
		t.Fatalf("created page lost the deck: %+v", got)
	}
	if src, err := f.raw.LoadDeckSource(ctx, out.ID); err != nil ||
		!strings.Contains(string(src), `"h":"S"`) {
		t.Fatalf("deck source = %s, %v", src, err)
	}
	if code, _ := get(t, f.ts.URL+"/p/"+out.ID+"/deck"); code != http.StatusOK {
		t.Errorf("shared deck view = %d, want 200", code)
	}

	// A replace without a deck removes it and its source.
	resp, _ := f.call(t, http.MethodPut, "/api/p/"+out.ID, key, page("NoDeck"), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("replace: %d", resp.StatusCode)
	}
	got, _ = f.raw.Get(ctx, out.ID)
	if got.HasDeck || got.Deck != "" || f.raw.HasDeckSource(ctx, out.ID) {
		t.Fatalf("replace without a deck must clear it: %+v", got)
	}
	if resp := getNoFollow(t, f.ts.URL+"/p/"+out.ID+"/deck"); resp.StatusCode != http.StatusFound {
		t.Errorf(
			"deck view after the deck was removed = %d, want 302 to the brief",
			resp.StatusCode,
		)
	}

	// A deck-only replace keeps the page reachable through the deck view.
	deckOnly := map[string]any{"title": "Only", "content": "", "deck": "<p>only</p>"}
	resp, _ = f.call(t, http.MethodPut, "/api/p/"+out.ID, key, deckOnly, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("deck-only replace: %d", resp.StatusCode)
	}
	if resp := getNoFollow(t, f.ts.URL+"/p/"+out.ID); resp.StatusCode != http.StatusFound {
		t.Errorf("deck-only shared page = %d, want 302 to the deck", resp.StatusCode)
	}
}

func TestShareBundleCarriesTheDeck(t *testing.T) {
	f := setupLocalSharing(t, author.NewKey())
	ctx := t.Context()
	p, err := f.st.Create(ctx, store.Draft{
		Title: "Both", Content: "<p>b</p>", Doc: []byte(`{"sections":[]}`),
		Deck: "<p>slide</p>", DeckSource: []byte(`{"sections":[{"h":"S","blocks":[]}]}`),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	code, body := postShare(t, f.ts, p.ID, `{"ephemeral":false}`)
	if code != http.StatusOK {
		t.Fatalf("share: %d %s", code, body)
	}
	local, _ := f.st.Get(ctx, p.ID)
	if local.Shared == nil {
		t.Fatal("share must record the copy")
	}
	remote, err := f.shared.raw.Get(ctx, local.Shared.ID)
	if err != nil {
		t.Fatalf("shared copy: %v", err)
	}
	if !remote.HasDeck || remote.Deck != "<p>slide</p>" {
		t.Errorf("shared copy lost the deck: %+v", remote)
	}
	if src, err := f.shared.raw.LoadDeckSource(ctx, local.Shared.ID); err != nil ||
		!strings.Contains(string(src), `"h":"S"`) {
		t.Errorf("shared copy deck source = %s, %v", src, err)
	}
}
