package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/present/internal/render"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// progressWorkdir points the CLI at a temp workdir holding one Doc page
// with a Build section and returns the store and the page id.
func progressWorkdir(t *testing.T) (*store.FS, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.NewFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	c, err := render.CompileDoc(render.Doc{Sections: []render.Section{
		{Heading: "Build", Blocks: []render.Block{{T: "p", Text: "status"}}},
	}}, "T")
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.Create(t.Context(), store.Draft{Title: "T", Content: c.HTML, Doc: c.JSON})
	if err != nil {
		t.Fatal(err)
	}
	prev := flagWorkdir
	flagWorkdir = dir
	t.Cleanup(func() {
		flagWorkdir = prev
		flagProgressNote, flagProgressSection, flagProgressDeck = "", "", false
	})
	return st, p.ID
}

func runProgressCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	progressCmd.SetOut(&out)
	t.Cleanup(func() { progressCmd.SetOut(nil) })
	err := runProgress(progressCmd, args)
	return out.String(), err
}

func TestProgressCommandWritesTheStore(t *testing.T) {
	st, id := progressWorkdir(t)
	flagProgressSection, flagProgressNote = "build", "compiling"
	out, err := runProgressCmd(t, id, "Compile", "40")
	if err != nil {
		t.Fatalf("progress: %v", err)
	}
	if want := id + ": Compile 40% in build, version 2\n"; out != want {
		t.Errorf("printed %q, want %q", out, want)
	}
	flagProgressSection, flagProgressNote = "", ""
	if _, err := runProgressCmd(t, id, "Compile", "100"); err != nil {
		t.Fatalf("second report: %v", err)
	}
	raw, err := st.LoadDoc(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	var d render.Doc
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	blocks := d.Sections[0].Blocks
	if len(d.Sections) != 1 || len(blocks) != 2 || blocks[1].Percent != 100 ||
		blocks[1].Note != "" {
		t.Fatalf("stored Doc = %+v, want the Build block updated in place", d.Sections)
	}
}

func TestProgressCommandRefusals(t *testing.T) {
	_, id := progressWorkdir(t)
	if _, err := runProgressCmd(t, id, "Compile", "lots"); err == nil ||
		!strings.Contains(err.Error(), "want a number") {
		t.Errorf("non-numeric pct: err = %v", err)
	}
	if _, err := runProgressCmd(t, id, "Compile", "150"); err == nil {
		t.Error("pct over 100 must be refused")
	}
	if _, err := runProgressCmd(t, "0123456789", "Compile", "5"); !errors.Is(
		err,
		store.ErrNotFound,
	) {
		t.Errorf("unknown page: err = %v, want ErrNotFound", err)
	}
}
