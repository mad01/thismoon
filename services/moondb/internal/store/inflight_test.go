package store

import (
	"fmt"
	"strings"
	"testing"
)

// inflightWT is the worktree every TestInFlight case asks about.
const inflightWT = "/wt"

// pathEdit is edit id of path by session at minute m.
func pathEdit(id, session, path string, m int) Edit {
	return Edit{ID: id, SessionID: session, Path: path, At: minutes(m)}
}

// linkTo is a link of a commit made in worktree at minute m that took
// edit.
func linkTo(edit, commit, worktree string, m int) CommitLink {
	return CommitLink{EditID: edit, Commit: commit, Worktree: worktree, At: minutes(m)}
}

// newestFirst reverses edits written oldest first into the order Edits
// lists them.
func newestFirst(edits ...Edit) []Edit {
	out := make([]Edit, 0, len(edits))
	for i := len(edits) - 1; i >= 0; i-- {
		out = append(out, edits[i])
	}
	return out
}

// describe renders the files as "path@minute[commit]:session×n,..." for
// compact comparison.
func describe(files []InFlightFile) string {
	var parts []string
	for _, f := range files {
		var b strings.Builder
		fmt.Fprintf(&b, "%s@%d", f.Path, int(f.LastEditAt.Sub(minutes(0)).Minutes()))
		if f.LinkedCommit != nil {
			fmt.Fprintf(&b, "[%s]", *f.LinkedCommit)
		}
		for i, s := range f.Sessions {
			sep := ","
			if i == 0 {
				sep = ":"
			}
			fmt.Fprintf(&b, "%s%s×%d@%d", sep, s.ID, s.Edits,
				int(s.LastEditAt.Sub(minutes(0)).Minutes()))
		}
		parts = append(parts, b.String())
	}
	return strings.Join(parts, " ")
}

func TestInFlight(t *testing.T) {
	tests := []struct {
		name  string
		edits []Edit // oldest first
		links []CommitLink
		want  string
	}{
		{
			name:  "another session's edit without a link shows",
			edits: []Edit{pathEdit("e1", "other", "a.go", 1)},
			want:  "a.go@1:other×1@1",
		},
		{
			name:  "the asking session's own edit stays hidden",
			edits: []Edit{pathEdit("e1", "me", "a.go", 1)},
			want:  "",
		},
		{
			name:  "a linked edit stays hidden",
			edits: []Edit{pathEdit("e1", "other", "a.go", 1)},
			links: []CommitLink{linkTo("e1", "c1", inflightWT, 2)},
			want:  "",
		},
		{
			name: "a later link covers the edits before it, whoever made them",
			edits: []Edit{
				pathEdit("e1", "other", "a.go", 1),
				pathEdit("e2", "other", "a.go", 2),
				pathEdit("e3", "me", "a.go", 3),
			},
			links: []CommitLink{linkTo("e3", "c1", inflightWT, 4)},
			want:  "",
		},
		{
			name: "an edit after the newest link shows with that commit",
			edits: []Edit{
				pathEdit("e1", "other", "a.go", 1),
				pathEdit("e2", "other", "a.go", 2),
				pathEdit("e3", "other", "a.go", 3),
			},
			links: []CommitLink{
				linkTo("e1", "c0", inflightWT, 1),
				linkTo("e2", "c1", inflightWT, 2),
			},
			want: "a.go@3[c1]:other×1@3",
		},
		{
			name:  "a link from a commit in another worktree does not count",
			edits: []Edit{pathEdit("e1", "other", "a.go", 1)},
			links: []CommitLink{linkTo("e1", "c1", "/other-wt", 2)},
			want:  "a.go@1:other×1@1",
		},
		{
			name: "the newest of two links to one edit names the commit",
			edits: []Edit{
				pathEdit("e1", "other", "a.go", 1),
				pathEdit("e2", "other", "a.go", 6),
			},
			links: []CommitLink{
				linkTo("e1", "c2", inflightWT, 5),
				linkTo("e1", "c1", inflightWT, 2),
			},
			want: "a.go@6[c2]:other×1@6",
		},
		{
			name: "a delete has no content to link, so a commit that took it stays unseen",
			edits: []Edit{
				pathEdit("e1", "other", "a.go", 1),
				{ID: "e2", SessionID: "other", Path: "a.go", At: minutes(2), Kind: EditDelete},
			},
			links: []CommitLink{linkTo("e1", "c1", inflightWT, 1)},
			want:  "a.go@2[c1]:other×1@2",
		},
		{
			name: "the asking session's newer edit leaves another's earlier one in flight",
			edits: []Edit{
				pathEdit("e1", "other", "a.go", 1),
				pathEdit("e2", "me", "a.go", 5),
			},
			want: "a.go@1:other×1@1",
		},
		{
			name: "sessions are counted per path, latest edit first",
			edits: []Edit{
				pathEdit("e1", "s1", "a.go", 1),
				pathEdit("e2", "s2", "a.go", 2),
				pathEdit("e3", "s1", "a.go", 3),
			},
			want: "a.go@3:s1×2@3,s2×1@2",
		},
		{
			name: "files sort by their latest edit, then by path",
			edits: []Edit{
				pathEdit("e1", "s1", "c.go", 1),
				pathEdit("e2", "s1", "b.go", 4),
				pathEdit("e3", "s2", "a.go", 4),
			},
			want: "a.go@4:s2×1@4 b.go@4:s1×1@4 c.go@1:s1×1@1",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := InFlightQuery{Worktree: inflightWT, Exclude: "me"}
			got := describe(InFlight(newestFirst(tc.edits...), tc.links, q))
			if got != tc.want {
				t.Errorf("InFlight = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestInFlightNeverReturnsNil(t *testing.T) {
	if got := InFlight(nil, nil, InFlightQuery{}); got == nil {
		t.Error("InFlight(nil) = nil, want an empty list so the API answers []")
	}
}
