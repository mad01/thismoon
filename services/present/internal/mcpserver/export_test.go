package mcpserver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/present/internal/store"
)

// present_export writes into exports/ in the page store, the one place the
// MCP sandbox lets it write, and returns where.
func TestExportWritesIntoTheStore(t *testing.T) {
	ctx := context.Background()
	workdir := t.TempDir()
	st, err := store.NewFS(workdir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.Create(ctx, store.Draft{Title: "Notes", Content: "<p>exported prose</p>"})
	if err != nil {
		t.Fatal(err)
	}
	h := &handlers{store: st, mode: ModeLocal, workdir: workdir}

	_, out, err := h.handleExport(ctx, nil, exportInput{ID: p.ID})
	if err != nil {
		t.Fatalf("handleExport: %v", err)
	}
	if want := filepath.Join(workdir, "exports", p.ID+".html"); out.Path != want {
		t.Errorf("path = %s, want %s", out.Path, want)
	}
	data, err := os.ReadFile(out.Path)
	if err != nil {
		t.Fatal(err)
	}
	if out.Bytes != len(data) || !strings.Contains(string(data), "<p>exported prose</p>") {
		t.Errorf("the file does not hold the page (%d bytes, reported %d)", len(data), out.Bytes)
	}
	if out.Rendition != "brief" || len(out.Libraries) != 0 || len(out.Notes) != 1 {
		t.Errorf("out = %+v; want a brief, no libraries, and the missing-fonts note", out)
	}

	_, _, err = h.handleExport(ctx, nil, exportInput{ID: p.ID, Deck: true})
	if !errors.Is(err, store.ErrNoDeck) {
		t.Errorf("deck export of a page without a deck: err = %v, want ErrNoDeck", err)
	}
}
