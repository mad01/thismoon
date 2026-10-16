// Package storetest holds the store.Store edit contract as a test suite any
// store runs against itself: the filesystem store, the cluster store on its
// fake client and on a kind cluster, and the wrappers a shared instance
// stacks on top. It is a plain package so both store packages' tests can
// import it.
package storetest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/mad01/thismoon/services/present/internal/store"
)

// Two writers over eight rounds give every round a real race for the same
// version and keep the suite fast on a cluster.
const (
	concurrentWriters = 2
	concurrentRounds  = 8
)

// RunEditConformance exercises Store.Edit against the store newStore builds
// fresh for each subtest.
func RunEditConformance(t *testing.T, newStore func(t *testing.T) store.Store) {
	t.Helper()
	t.Run("EditReplacesPageAndSources", func(t *testing.T) { editReplaces(t, newStore(t)) })
	t.Run("EditKeepsStoreOwnedFields", func(t *testing.T) { editKeepsOwned(t, newStore(t)) })
	t.Run("EditErrorWritesNothing", func(t *testing.T) { editErrorWritesNothing(t, newStore(t)) })
	t.Run("EditMissingPage", func(t *testing.T) { editMissingPage(t, newStore(t)) })
	t.Run("UpdateKeepsSources", func(t *testing.T) { updateKeepsSources(t, newStore(t)) })
	t.Run("ConcurrentEditsLoseNothing", func(t *testing.T) {
		st := newStore(t)
		id := NewTokenPage(t, st)
		want := runConcurrentWriters(t, st, id)
		CheckTokens(t, st, id, want)
	})
}

func editReplaces(t *testing.T, st store.Store) {
	ctx := context.Background()
	p, err := st.Create(ctx, store.Draft{
		Title: "T", Content: "<p>v1</p>", Deck: "<wk-section>d</wk-section>", Graph: "cy.init();",
		Doc:         []byte(`{"v":1}`),
		DeckSource:  []byte(`{"deck":1}`),
		GraphSource: []byte(`{"nodes":[]}`),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := st.Edit(ctx, p.ID, func(cur store.Snapshot) (store.Snapshot, error) {
		cur.Page.Title, cur.Page.Content, cur.Page.Deck = "T2", "<p>v2</p>", ""
		cur.Sources.Doc, cur.Sources.Deck = []byte(`{"v":2}`), nil
		return cur, nil
	})
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if got.Version != 2 || got.Title != "T2" || got.Content != "<p>v2</p>" || got.HasDeck ||
		!got.HasDoc || !got.HasGraph {
		t.Fatalf("Edit returned %+v", got)
	}
	read, err := st.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if read.Version != 2 || read.Content != "<p>v2</p>" || read.HasDeck || read.Deck != "" {
		t.Fatalf("Get after Edit = %+v", read)
	}
	wantSource(t, "doc", load(st.LoadDoc(ctx, p.ID)), `{"v":2}`)
	wantSource(t, "graph", load(st.LoadGraphSource(ctx, p.ID)), `{"nodes":[]}`)
	if _, err := st.LoadDeckSource(ctx, p.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deck source after a nil source: err = %v, want ErrNotFound", err)
	}

	got, err = st.Edit(ctx, p.ID, func(cur store.Snapshot) (store.Snapshot, error) {
		cur.Sources.Doc = nil
		return cur, nil
	})
	if err != nil || got.HasDoc || st.HasDoc(ctx, p.ID) {
		t.Fatalf("Edit clearing the Doc: has_doc %v, stored %v, err %v",
			got.HasDoc, st.HasDoc(ctx, p.ID), err)
	}
}

func editKeepsOwned(t *testing.T, st store.Store) {
	ctx := context.Background()
	p, err := st.Create(ctx, store.Draft{Title: "T", Content: "<p>x</p>", Author: "abc"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := st.Edit(ctx, p.ID, func(cur store.Snapshot) (store.Snapshot, error) {
		cur.Page.ID, cur.Page.Version, cur.Page.Author = "other", 99, "mallory"
		cur.Page.CreatedAt = cur.Page.CreatedAt.AddDate(-1, 0, 0)
		cur.Page.Shared = &store.SharedInfo{ID: "x", URL: "https://x"}
		cur.Page.Title = "kept"
		return cur, nil
	})
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	read, err := st.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	for _, page := range []store.Page{got, read} {
		if page.ID != p.ID || page.Version != 2 || page.Author != "abc" || page.Shared != nil ||
			!page.CreatedAt.Equal(p.CreatedAt) || page.Title != "kept" {
			t.Fatalf("store-owned fields moved: %+v", page)
		}
	}
}

func editErrorWritesNothing(t *testing.T, st store.Store) {
	ctx := context.Background()
	p, err := st.Create(ctx, store.Draft{Title: "T", Content: "<p>v1</p>", Doc: []byte(`{"v":1}`)})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	refuse := errors.New("refused")
	_, err = st.Edit(ctx, p.ID, func(cur store.Snapshot) (store.Snapshot, error) {
		cur.Page.Content, cur.Sources.Doc = "<p>v2</p>", []byte(`{"v":2}`)
		return cur, refuse
	})
	if !errors.Is(err, refuse) {
		t.Fatalf("Edit: err = %v, want the closure's error", err)
	}
	read, err := st.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if read.Version != 1 || read.Content != "<p>v1</p>" {
		t.Fatalf("a refused edit wrote: %+v", read)
	}
	wantSource(t, "doc", load(st.LoadDoc(ctx, p.ID)), `{"v":1}`)
}

func editMissingPage(t *testing.T, st store.Store) {
	for _, id := range []string{store.NewSharedID(), store.NewID(), "../x"} {
		ran := false
		_, err := st.Edit(
			context.Background(),
			id,
			func(cur store.Snapshot) (store.Snapshot, error) {
				ran = true
				return cur, nil
			},
		)
		if !errors.Is(err, store.ErrNotFound) || ran {
			t.Errorf(
				"Edit(%q): err = %v, closure ran %v; want ErrNotFound and no run",
				id,
				err,
				ran,
			)
		}
	}
}

func updateKeepsSources(t *testing.T, st store.Store) {
	ctx := context.Background()
	p, err := st.Create(ctx, store.Draft{
		Title: "T", Content: "<p>v1</p>", Doc: []byte(`{"v":1}`), GraphSource: []byte(`{"g":1}`),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	title := "T2"
	got, err := st.Update(ctx, p.ID, store.Patch{Title: &title})
	if err != nil || got.Version != 2 || got.Title != "T2" || !got.HasDoc {
		t.Fatalf("Update = %+v, %v", got, err)
	}
	wantSource(t, "doc", load(st.LoadDoc(ctx, p.ID)), `{"v":1}`)
	wantSource(t, "graph", load(st.LoadGraphSource(ctx, p.ID)), `{"g":1}`)
}

// runConcurrentWriters runs the writers round by round, all starting each
// round together, so every round has them racing for one version. It fails
// the test if an edit fails or a writer sees its versions go backwards, and
// returns every token the writers appended.
func runConcurrentWriters(t *testing.T, st store.Store, id string) []string {
	t.Helper()
	versions := make([][]int, concurrentWriters)
	errs := make([]error, concurrentWriters)
	var want []string
	for round := range concurrentRounds {
		start := make(chan struct{})
		var wg sync.WaitGroup
		for w := range concurrentWriters {
			token := fmt.Sprintf("w%d-r%d", w, round)
			want = append(want, token)
			wg.Go(func() {
				<-start
				p, err := st.Edit(context.Background(), id, AppendToken(token))
				if err != nil {
					errs[w] = errors.Join(errs[w], fmt.Errorf("%s: %w", token, err))
					return
				}
				versions[w] = append(versions[w], p.Version)
			})
		}
		close(start)
		wg.Wait()
	}
	if err := errors.Join(errs...); err != nil {
		t.Fatalf("concurrent edits failed: %v", err)
	}
	checkVersions(t, versions)
	return want
}

// checkVersions requires each writer's versions to rise and every version
// from 2 up to be handed out exactly once: two edits that got the same
// version built on the same state, and one of them was lost.
func checkVersions(t *testing.T, versions [][]int) {
	t.Helper()
	var all []int
	for w, vs := range versions {
		if !slices.IsSorted(vs) || len(slices.Compact(slices.Clone(vs))) != len(vs) {
			t.Errorf("writer %d saw versions %v, want strictly increasing", w, vs)
		}
		all = append(all, vs...)
	}
	slices.Sort(all)
	for i, v := range all {
		if v != i+2 {
			t.Fatalf("versions handed out = %v, want each of 2..%d once", all, len(all)+1)
		}
	}
}

// NewTokenPage creates the page the token edits work on: a Doc source that
// is a JSON list of tokens, and content rendered from it.
func NewTokenPage(t *testing.T, st store.Store) string {
	t.Helper()
	p, err := st.Create(context.Background(), store.Draft{
		Title: "tokens", Content: renderTokens(nil), Doc: []byte(`[]`),
	})
	if err != nil {
		t.Fatalf("Create token page: %v", err)
	}
	return p.ID
}

// AppendToken returns an edit that appends token to a token page's Doc and
// re-renders its content. It refuses to build on a state whose content was
// not rendered from its Doc, so a write that let the two drift apart fails
// the next edit that reads it.
func AppendToken(token string) store.EditFunc {
	return func(cur store.Snapshot) (store.Snapshot, error) {
		tokens, err := decodeTokens(cur.Sources.Doc)
		if err != nil {
			return store.Snapshot{}, err
		}
		if cur.Page.Content != renderTokens(tokens) {
			return store.Snapshot{}, fmt.Errorf(
				"edit read content %q over Doc %s", cur.Page.Content, cur.Sources.Doc,
			)
		}
		tokens = append(tokens, token)
		doc, err := json.Marshal(tokens)
		if err != nil {
			return store.Snapshot{}, fmt.Errorf("encode tokens: %w", err)
		}
		cur.Sources.Doc = doc
		cur.Page.Content = renderTokens(tokens)
		return cur, nil
	}
}

// CheckTokens requires the token page to hold exactly the tokens in want,
// each once and in any order, at version 1 plus one per token, with content
// rendered from its Doc.
func CheckTokens(t *testing.T, st store.Store, id string, want []string) {
	t.Helper()
	ctx := context.Background()
	p, err := st.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	doc, err := st.LoadDoc(ctx, id)
	if err != nil {
		t.Fatalf("LoadDoc: %v", err)
	}
	got, err := decodeTokens(doc)
	if err != nil {
		t.Fatal(err)
	}
	if p.Content != renderTokens(got) {
		t.Errorf("content %q was not rendered from Doc %s", p.Content, doc)
	}
	if p.Version != len(want)+1 {
		t.Errorf("version = %d, want %d (one bump per edit)", p.Version, len(want)+1)
	}
	got, want = slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(want))
	if !slices.Equal(got, want) {
		t.Errorf("tokens = %v, want %v: an edit was lost or applied twice", got, want)
	}
}

func decodeTokens(doc []byte) ([]string, error) {
	var tokens []string
	if err := json.Unmarshal(doc, &tokens); err != nil {
		return nil, fmt.Errorf("decode tokens %q: %w", doc, err)
	}
	return tokens, nil
}

func renderTokens(tokens []string) string {
	return "<p>" + strings.Join(tokens, ",") + "</p>"
}

// load turns a source read into the string a test compares, with a failed
// read spelled out so the mismatch message names the error.
func load(data []byte, err error) string {
	if err != nil {
		return "<error: " + err.Error() + ">"
	}
	return string(data)
}

func wantSource(t *testing.T, name, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s source = %s, want %s", name, got, want)
	}
}
