package store

import (
	"errors"
	"fmt"
	"testing"
)

func TestHistoryRecordsEveryVersion(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p, err := s.Create(ctx, Draft{Title: "T", Content: "<p>v1</p>", Doc: []byte(`{"v":1}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Edit(ctx, p.ID, func(cur Snapshot) (Snapshot, error) {
		cur.Page.Title, cur.Page.Content, cur.Sources.Doc = "T2", "<p>v2</p>", []byte(`{"v":2}`)
		return cur, nil
	}); err != nil {
		t.Fatal(err)
	}
	list, err := s.Revisions(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Version != 2 || list[0].Title != "T2" || list[1].Version != 1 {
		t.Fatalf("revisions = %+v, want versions 2 then 1", list)
	}
	r, err := s.Revision(ctx, p.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if format, content := r.ContentSource(); format != "doc" || content != `{"v":1}` ||
		r.Title != "T" {
		t.Fatalf("revision 1 = %s %s %q, want the first Doc", format, content, r.Title)
	}
	refused := errors.New("refused")
	refuse := func(Snapshot) (Snapshot, error) { return Snapshot{}, refused }
	if _, err := s.Edit(ctx, p.ID, refuse); !errors.Is(err, refused) {
		t.Fatal(err)
	}
	if list, _ := s.Revisions(ctx, p.ID); len(list) != 2 {
		t.Errorf("a refused edit recorded a revision: %+v", list)
	}
}

func TestHistoryKeepsTheNewestTwenty(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p, err := s.Create(ctx, Draft{Title: "T", Content: "<p>0</p>"})
	if err != nil {
		t.Fatal(err)
	}
	const edits = historyKeep + 5
	for i := range edits {
		content := fmt.Sprintf("<p>%d</p>", i+1)
		if _, err := s.Update(ctx, p.ID, Patch{Content: &content}); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.Revisions(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	newest, oldest := edits+1, edits+2-historyKeep
	if len(list) != historyKeep || list[0].Version != newest ||
		list[len(list)-1].Version != oldest {
		t.Fatalf("kept %d revisions from v%d to v%d, want %d from v%d to v%d",
			len(list), list[len(list)-1].Version, list[0].Version, historyKeep, oldest, newest)
	}
	if _, err := s.Revision(ctx, p.ID, oldest-1); !errors.Is(err, ErrRevisionNotKept) {
		t.Errorf("a pruned version: err = %v, want ErrRevisionNotKept", err)
	}
}

// A page with no Doc or graph source keeps its HTML and script, which is
// the only form present_source can hand back for it.
func TestHistoryKeepsLegacyBodies(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p, err := s.Create(ctx, Draft{Title: "T", Content: "<p>legacy</p>", Graph: "cy.init();"})
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Revision(ctx, p.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	format, content := r.ContentSource()
	gformat, graph := r.GraphSourceOf()
	if format != "html" || content != "<p>legacy</p>" || gformat != "js" || graph != "cy.init();" {
		t.Fatalf("legacy revision = %s %q, %s %q", format, content, gformat, graph)
	}
}

func TestHistoryMissingPagesAndVersions(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	if _, err := s.Revisions(ctx, NewID()); !errors.Is(err, ErrNotFound) {
		t.Errorf("Revisions of a missing page: err = %v, want ErrNotFound", err)
	}
	p, err := s.Create(ctx, Draft{Title: "T", Content: "<p>x</p>"})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []int{0, -1, 2} {
		if _, err := s.Revision(ctx, p.ID, v); !errors.Is(err, ErrRevisionNotKept) {
			t.Errorf("Revision(%d): err = %v, want ErrRevisionNotKept", v, err)
		}
	}
}
