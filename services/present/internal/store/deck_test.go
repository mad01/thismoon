package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreatePersistsDeckBesideContent(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p, err := s.Create(ctx, Draft{
		Title: "Both", Content: "<p>brief</p>", Deck: "<wk-section>slide</wk-section>",
		DeckSource: []byte(`{"sections":[{"h":"S","blocks":[]}]}`),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !p.HasDeck || !p.HasBrief || p.Deck != "<wk-section>slide</wk-section>" {
		t.Fatalf("Create returned %+v, want HasDeck and HasBrief", p)
	}
	dir := s.pageDir(p.ID)
	for _, f := range []string{contentFile, deckFile, deckSourceFile} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("expected %s on disk: %v", f, err)
		}
	}
	got, err := s.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Deck != p.Deck || !got.HasDeck || got.Content != "<p>brief</p>" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	meta, err := s.GetMeta(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if meta.Deck != "" || !meta.HasDeck {
		t.Fatalf("GetMeta = Deck %q HasDeck %v, want no body and the flag", meta.Deck, meta.HasDeck)
	}
	metas, err := s.ListMeta(ctx)
	if err != nil || len(metas) != 1 || !metas[0].HasDeck || metas[0].Deck != "" {
		t.Fatalf("ListMeta = %+v, %v; want one page flagged has_deck without its body", metas, err)
	}
	if src, err := s.LoadDeckSource(ctx, p.ID); err != nil ||
		string(src) != `{"sections":[{"h":"S","blocks":[]}]}` {
		t.Fatalf("LoadDeckSource = %q, %v; want the draft deck source", src, err)
	}
}

func TestDeckOnlyPageHasNoBrief(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p, err := s.Create(ctx, Draft{Title: "Deck only", Deck: "<wk-section>1</wk-section>"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.HasBrief || !p.HasDeck {
		t.Fatalf("deck-only page: HasBrief %v HasDeck %v", p.HasBrief, p.HasDeck)
	}
	got, err := s.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Content != "" || got.Deck != "<wk-section>1</wk-section>" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(s.pageDir(p.ID), deckSourceFile)); !errors.Is(
		err, os.ErrNotExist,
	) {
		t.Fatalf("deck.json must not exist without a DeckSource: %v", err)
	}
}

func TestUpdatePatchesAndClearsDeck(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p, _ := s.Create(ctx, Draft{Title: "T", Content: "<p>x</p>"})
	if p.HasDeck {
		t.Fatal("HasDeck should be false initially")
	}
	deck := "<wk-section>added</wk-section>"
	got, err := s.Update(ctx, p.ID, Patch{Deck: &deck})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !got.HasDeck || got.Deck != deck || got.Version != 2 {
		t.Fatalf("deck not patched: %+v", got)
	}
	reloaded, _ := s.Get(ctx, p.ID)
	if reloaded.Deck != deck || !reloaded.HasDeck {
		t.Fatalf("deck not persisted: %+v", reloaded)
	}
	empty := ""
	got, err = s.Update(ctx, p.ID, Patch{Deck: &empty})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.HasDeck || got.Deck != "" || got.Version != 3 {
		t.Fatalf("deck not cleared: %+v", got)
	}
	if reloaded, _ := s.Get(ctx, p.ID); reloaded.HasDeck || reloaded.Deck != "" {
		t.Fatalf("cleared deck came back on Get: %+v", reloaded)
	}
}

func TestDeckSourceRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p, _ := s.Create(ctx, Draft{Title: "T", Content: "<p>x</p>"})
	if s.HasDeckSource(ctx, p.ID) {
		t.Fatal("new page should have no deck source")
	}
	if _, err := s.LoadDeckSource(ctx, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("LoadDeckSource absent: err = %v, want ErrNotFound", err)
	}
	src := []byte(`{"sections":[{"h":"One","blocks":[]}]}`)
	if err := s.SaveDeckSource(ctx, p.ID, src); err != nil {
		t.Fatalf("SaveDeckSource: %v", err)
	}
	if !s.HasDeckSource(ctx, p.ID) {
		t.Fatal("HasDeckSource = false after save")
	}
	got, err := s.LoadDeckSource(ctx, p.ID)
	if err != nil || string(got) != string(src) {
		t.Fatalf("LoadDeckSource = %q, %v", got, err)
	}
	if err := s.DeleteDeckSource(ctx, p.ID); err != nil {
		t.Fatalf("DeleteDeckSource: %v", err)
	}
	if s.HasDeckSource(ctx, p.ID) {
		t.Fatal("deck source still present after delete")
	}
	if err := s.DeleteDeckSource(ctx, p.ID); err != nil {
		t.Fatalf("DeleteDeckSource on absent file: %v", err)
	}
	if err := s.SaveDeckSource(ctx, "../escape", src); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SaveDeckSource invalid id: err = %v, want ErrNotFound", err)
	}
}

func TestValidateDeckAction(t *testing.T) {
	cases := []struct {
		action string
		slide  int
		ok     bool
	}{
		{DeckStart, 0, true},
		{DeckStop, 0, true},
		{DeckNext, 0, true},
		{DeckPrev, 0, true},
		{DeckNext, 7, true}, // slide is ignored outside goto
		{DeckGoto, 1, true},
		{DeckGoto, 12, true},
		{DeckGoto, 0, false},
		{DeckGoto, -1, false},
		{"", 0, false},
		{"jump", 1, false},
		{"START", 0, false},
	}
	for _, tc := range cases {
		err := ValidateDeckAction(tc.action, tc.slide)
		if (err == nil) != tc.ok {
			t.Errorf("ValidateDeckAction(%q, %d) = %v, want ok=%v", tc.action, tc.slide, err, tc.ok)
		}
	}
}

func TestSendDeckCommandIncrementsSeq(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	base := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return base }
	p, _ := s.Create(ctx, Draft{Title: "T", Deck: "<wk-section>1</wk-section>"})

	if cmd, err := s.DeckCommand(ctx, p.ID); err != nil || cmd != (DeckCommand{}) {
		t.Fatalf("DeckCommand before any send = %+v, %v; want the zero value", cmd, err)
	}
	first, err := s.SendDeckCommand(ctx, p.ID, DeckStart, 0)
	if err != nil {
		t.Fatalf("SendDeckCommand: %v", err)
	}
	if first.Seq != 1 || first.Action != DeckStart || first.Slide != 0 || !first.At.Equal(base) {
		t.Fatalf("first command = %+v", first)
	}
	second, err := s.SendDeckCommand(ctx, p.ID, DeckGoto, 4)
	if err != nil {
		t.Fatalf("SendDeckCommand goto: %v", err)
	}
	if second.Seq != 2 || second.Action != DeckGoto || second.Slide != 4 {
		t.Fatalf("second command = %+v", second)
	}
	// next carries no slide even when one is passed.
	third, err := s.SendDeckCommand(ctx, p.ID, DeckNext, 9)
	if err != nil {
		t.Fatalf("SendDeckCommand next: %v", err)
	}
	if third.Seq != 3 || third.Slide != 0 {
		t.Fatalf("third command = %+v, want seq 3 and no slide", third)
	}
	got, err := s.DeckCommand(ctx, p.ID)
	if err != nil || got != third {
		t.Fatalf("DeckCommand = %+v, %v; want the last sent %+v", got, err, third)
	}
	if _, err := os.Stat(filepath.Join(s.pageDir(p.ID), deckCommandFile)); err != nil {
		t.Fatalf("deck-command.json not written: %v", err)
	}
	// Sending a command never bumps the page version.
	if meta, _ := s.GetMeta(ctx, p.ID); meta.Version != 1 {
		t.Fatalf("version = %d after commands, want 1", meta.Version)
	}
}

func TestSendDeckCommandRefusesBadInput(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	deck, _ := s.Create(ctx, Draft{Title: "Deck", Deck: "<wk-section>1</wk-section>"})
	brief, _ := s.Create(ctx, Draft{Title: "Brief", Content: "<p>x</p>"})

	if _, err := s.SendDeckCommand(ctx, deck.ID, "jump", 0); err == nil {
		t.Error("unknown action must fail")
	}
	if _, err := s.SendDeckCommand(ctx, deck.ID, DeckGoto, 0); err == nil {
		t.Error("goto without a slide must fail")
	}
	if _, err := s.SendDeckCommand(ctx, brief.ID, DeckNext, 0); !errors.Is(err, ErrNoDeck) {
		t.Errorf("send to a page without a deck: err = %v, want ErrNoDeck", err)
	}
	if _, err := s.DeckCommand(ctx, brief.ID); !errors.Is(err, ErrNoDeck) {
		t.Errorf("read on a page without a deck: err = %v, want ErrNoDeck", err)
	}
	for _, id := range []string{"deadbeef00", "../escape"} {
		if _, err := s.SendDeckCommand(ctx, id, DeckNext, 0); !errors.Is(err, ErrNotFound) {
			t.Errorf("send to %q: err = %v, want ErrNotFound", id, err)
		}
		if _, err := s.DeckCommand(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("read on %q: err = %v, want ErrNotFound", id, err)
		}
	}
	if _, err := os.Stat(filepath.Join(s.pageDir(deck.ID), deckCommandFile)); !errors.Is(
		err, os.ErrNotExist,
	) {
		t.Fatalf("a refused command must write nothing: %v", err)
	}
}

// A meta.json written before decks existed has no has_brief key. Every page
// then had a brief, so metadata reads derive the flag from the content file
// until the next write persists it.
func TestHasBriefDerivedForLegacyMeta(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p, err := s.Create(ctx, Draft{Title: "legacy", Content: "<p>body</p>"})
	if err != nil {
		t.Fatal(err)
	}
	metaPath := filepath.Join(s.pageDir(p.ID), metaFile)
	raw, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["has_brief"]; !ok {
		t.Fatal("Create did not persist has_brief")
	}
	delete(m, "has_brief")
	legacy, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metaPath, legacy, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetMeta(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.HasBrief {
		t.Error("GetMeta: legacy meta without has_brief read as no brief")
	}
	metas, err := s.ListMeta(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 1 || !metas[0].HasBrief {
		t.Errorf("ListMeta: got %+v, want one page with a brief", metas)
	}

	// A deck-only page persists has_brief false, and metadata reads keep it.
	d, err := s.Create(ctx, Draft{Title: "deck", Deck: "<wk-section></wk-section>"})
	if err != nil {
		t.Fatal(err)
	}
	dm, err := s.GetMeta(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if dm.HasBrief || !dm.HasDeck {
		t.Errorf("deck-only GetMeta: has_brief=%v has_deck=%v", dm.HasBrief, dm.HasDeck)
	}
}

func TestDeckCommandsAfterKeepsABoundedHistory(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p, _ := s.Create(ctx, Draft{Title: "T", Deck: "<wk-section>1</wk-section>"})

	if got, err := s.DeckCommandsAfter(ctx, p.ID, -1); err != nil || got != nil {
		t.Fatalf("DeckCommandsAfter before any send = %v, %v; want nil", got, err)
	}
	for i := 0; i < deckCommandKeep+3; i++ {
		if _, err := s.SendDeckCommand(ctx, p.ID, DeckNext, 0); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	all, err := s.DeckCommandsAfter(ctx, p.ID, -1)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != deckCommandKeep || all[0].Seq != 4 ||
		all[len(all)-1].Seq != int64(deckCommandKeep+3) {
		t.Fatalf("kept %d commands, first seq %d, last seq %d; want %d from seq 4",
			len(all), all[0].Seq, all[len(all)-1].Seq, deckCommandKeep)
	}
	tail, err := s.DeckCommandsAfter(ctx, p.ID, int64(deckCommandKeep+1))
	if err != nil {
		t.Fatal(err)
	}
	if len(tail) != 2 || tail[0].Seq != int64(deckCommandKeep+2) {
		t.Fatalf("after %d: got %+v, want the last two", deckCommandKeep+1, tail)
	}
	if got, err := s.DeckCommandsAfter(ctx, p.ID, 1<<40); err != nil || got != nil {
		t.Fatalf("after a future seq = %v, %v; want nil", got, err)
	}
	if last, _ := s.DeckCommand(ctx, p.ID); last.Seq != int64(deckCommandKeep+3) {
		t.Fatalf("DeckCommand = seq %d, want the newest", last.Seq)
	}
}
