package sharedclient

import (
	"context"
	"testing"
	"time"

	present "github.com/mad01/thismoon/services/present"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// A share to an instance behind this binary's render level lands and is
// recorded like any other, and the result says the copy may draw wrong.
func TestShareWarnsWhenTheInstanceIsBehind(t *testing.T) {
	cur := present.RenderLevel
	cases := []struct {
		name     string
		instance int // the level the instance answers with; 0 answers none
		local    int // the client's level; 0 is this binary's
		want     string
	}{
		{"same level", cur, 0, ""},
		{"instance ahead", cur + 1, 0, ""},
		{"instance behind", cur, cur + 1, levelWarning(cur, cur+1)},
		{"instance predates render levels", 0, 0, levelWarning(0, cur)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeShared(t, "k")
			f.level = tc.instance
			c := New(f.ts.URL, "k")
			c.level = tc.local
			st, err := store.NewFS(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			p, _ := st.Create(ctx, store.Draft{Title: "T", Content: "<p>x</p>"})

			got, err := Share(ctx, st, c, p.ID, false, time.Now())
			if err != nil {
				t.Fatalf("share: %v", err)
			}
			if got.Warning != tc.want {
				t.Errorf("warning = %q, want %q", got.Warning, tc.want)
			}
			if pushed := f.pages[got.ID]; pushed.RenderLevel != c.renderLevel() {
				t.Errorf("bundle render_level = %d, want %d", pushed.RenderLevel, c.renderLevel())
			}
			if rec, _ := st.Get(ctx, p.ID); rec.Shared == nil || rec.Shared.URL != got.URL {
				t.Errorf("share not recorded: %+v", rec.Shared)
			}
		})
	}
}

// TestLevelWarningWording pins the two sentences an author reads.
func TestLevelWarningWording(t *testing.T) {
	cases := map[[2]int]string{
		{3, 3}: "",
		{4, 3}: "",
		{2, 3}: "the shared instance draws pages at render level 2 and this present renders at level 3, " +
			"so the copy may draw wrong there; update the shared instance, then share again",
		{0, 3}: "the shared instance predates render levels and this present renders at level 3, " +
			"so the copy may draw wrong there; update the shared instance, then share again",
	}
	for in, want := range cases {
		if got := levelWarning(in[0], in[1]); got != want {
			t.Errorf("levelWarning(%d, %d) = %q, want %q", in[0], in[1], got, want)
		}
	}
}
