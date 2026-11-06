package library

import (
	"context"
	"hash/fnv"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/semantic"
	"github.com/mad01/thismoon/services/brain/internal/testhome"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// countingEmbed is a deterministic bag-of-words embedder that counts calls.
type countingEmbed struct{ calls atomic.Int64 }

func (e *countingEmbed) embed(_ context.Context, text string) ([]float32, error) {
	e.calls.Add(1)
	const dim = 16
	v := make([]float32, dim)
	for _, w := range strings.Fields(strings.ToLower(text)) {
		h := fnv.New32a()
		h.Write([]byte(w))
		v[h.Sum32()%dim]++
	}
	var sum float64
	for _, x := range v {
		sum += float64(x * x)
	}
	if sum > 0 {
		for i := range v {
			v[i] /= float32(math.Sqrt(sum))
		}
	}
	return v, nil
}

type semanticFixture struct {
	t     *testing.T
	root  string
	embed *countingEmbed
}

func newSemanticFixture(t *testing.T, notes map[string]string) *semanticFixture {
	t.Helper()
	testhome.Isolate(t)
	f := &semanticFixture{t: t, root: t.TempDir(), embed: &countingEmbed{}}
	for rel, content := range notes {
		f.write(rel, content)
	}
	return f
}

func (f *semanticFixture) write(rel, content string) {
	f.t.Helper()
	full := filepath.Join(f.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

// open opens a library over the fixture vault embedding with model.
func (f *semanticFixture) open(model string) (*Library, *vault.Vault) {
	f.t.Helper()
	cfg := &config.Config{
		Vaults:   []config.Vault{{Name: "personal", Path: f.root, Default: true}},
		Semantic: config.SemanticConfig{Enabled: true, EmbedModel: model},
	}
	lib, err := Open(Options{Config: cfg, Embed: f.embed.embed})
	if err != nil {
		f.t.Fatal(err)
	}
	return lib, lib.Set().Vaults()[0]
}

func (f *semanticFixture) refresh(lib *Library, v *vault.Vault, force bool) Refresh {
	f.t.Helper()
	res, err := lib.RefreshSemantic(context.Background(), v, force, nil)
	if err != nil {
		f.t.Fatal(err)
	}
	return res
}

// paths returns the notes a semantic query over every chunk finds.
func (f *semanticFixture) paths(lib *Library, text string) map[string]bool {
	f.t.Helper()
	res, err := lib.SemanticSearch(context.Background(), lib.Set(), SemanticQuery{
		Text:   text,
		Mode:   semantic.ModePaths,
		Filter: semantic.QueryOptions{K: 50},
	})
	if err != nil {
		f.t.Fatal(err)
	}
	out := map[string]bool{}
	for _, h := range res.Hits {
		out[h.Path] = true
	}
	return out
}

var semanticNotes = map[string]string{
	"notes/a.md": "---\ntitle: A\n---\nalpha apple orchard\n",
	"notes/b.md": "---\ntitle: B\n---\nbravo zebra stripes\n",
	"inbox/c.md": "---\ntitle: C\n---\ncharlie cherry tree\n",
}

func TestRefreshSemanticReembedsOnlyChangedNotes(t *testing.T) {
	f := newSemanticFixture(t, semanticNotes)
	lib, v := f.open("model-a")
	if res := f.refresh(lib, v, false); !res.Built || res.Notes != 3 {
		t.Fatalf("first refresh = %+v, want three notes embedded", res)
	}
	if res := f.refresh(lib, v, false); res.Built {
		t.Errorf("refresh of an unchanged vault = %+v, want nothing done", res)
	}

	f.write("notes/a.md", "---\ntitle: A\n---\nalpha apple orchard, now with pears\n")
	before := f.embed.calls.Load()
	res := f.refresh(lib, v, false)
	if !res.Built || res.Notes != 1 || res.Chunks != 1 {
		t.Errorf("refresh after one edit = %+v, want one note, one chunk", res)
	}
	if calls := f.embed.calls.Load() - before; calls != 1 {
		t.Errorf("embedded %d chunk(s) after one edit, want 1", calls)
	}
	if got := f.paths(lib, "pears"); !got["notes/a.md"] || len(got) != 3 {
		t.Errorf("hits = %v, want every note once", got)
	}
}

func TestRefreshSemanticPurgesVanishedNotes(t *testing.T) {
	f := newSemanticFixture(t, semanticNotes)
	lib, v := f.open("model-a")
	f.refresh(lib, v, false)
	if err := os.Remove(filepath.Join(f.root, "notes", "b.md")); err != nil {
		t.Fatal(err)
	}
	res := f.refresh(lib, v, false)
	if res.Removed != 1 || res.Notes != 0 {
		t.Errorf("refresh after a delete = %+v, want one removed, none embedded", res)
	}
	if got := f.paths(lib, "zebra stripes"); got["notes/b.md"] {
		t.Errorf("a deleted note still answers: %v", got)
	}
}

func TestRefreshSemanticRebuilds(t *testing.T) {
	tests := []struct {
		name   string
		change func(t *testing.T, f *semanticFixture, dir string)
		model  string
	}{
		{name: "another embed model", model: "model-b"},
		{
			name:  "chunks without a manifest",
			model: "model-a",
			change: func(t *testing.T, _ *semanticFixture, dir string) {
				if err := os.Remove(filepath.Join(dir, "manifest.json")); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newSemanticFixture(t, semanticNotes)
			lib, v := f.open("model-a")
			f.refresh(lib, v, false)
			dir, err := lib.Config().Semantic.ResolvedIndexDir(v.Name)
			if err != nil {
				t.Fatal(err)
			}
			if tc.change != nil {
				tc.change(t, f, dir)
			}
			lib, v = f.open(tc.model)
			if res := f.refresh(lib, v, false); res.Notes != 3 {
				t.Errorf("refresh = %+v, want every note re-embedded", res)
			}
			idx, err := semantic.Open(dir, f.embed.embed)
			if err != nil {
				t.Fatal(err)
			}
			if n := idx.Count(); n != 3 {
				t.Errorf("index holds %d chunks after the rebuild, want 3 (no duplicates)", n)
			}
		})
	}
}

// TestRefreshSeesASameSizeEditWithinASecond edits a note to new text of the
// same size within the second of its last write: both indexes refresh, and
// the semantic sync re-embeds that note alone by its content hash.
func TestRefreshSeesASameSizeEditWithinASecond(t *testing.T) {
	f := newSemanticFixture(t, semanticNotes)
	second := time.Unix(1_700_000_000, 0)
	target := filepath.Join(f.root, "notes", "a.md")
	setMtime := func(at time.Time) {
		t.Helper()
		if err := os.Chtimes(target, at, at); err != nil {
			t.Fatal(err)
		}
	}
	setMtime(second.Add(100 * time.Millisecond))
	lib, v := f.open("model-a")
	if res, err := lib.RefreshLiteral(v, false, nil); err != nil || !res.Built {
		t.Fatalf("first literal refresh = %+v, %v", res, err)
	}
	f.refresh(lib, v, false)

	f.write("notes/a.md", "---\ntitle: A\n---\nalpha apple orchids\n")
	setMtime(second.Add(600 * time.Millisecond))
	if res, err := lib.RefreshLiteral(v, false, nil); err != nil || !res.Built {
		t.Errorf("literal refresh after the edit = %+v, %v; want a rebuild", res, err)
	}
	if res := f.refresh(lib, v, false); !res.Built || res.Notes != 1 {
		t.Errorf("semantic refresh after the edit = %+v, want one note re-embedded", res)
	}
	if got := f.paths(lib, "orchids"); !got["notes/a.md"] {
		t.Errorf("hits = %v, want the edited note", got)
	}
}
