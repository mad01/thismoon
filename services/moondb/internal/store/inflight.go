package store

import (
	"cmp"
	"slices"
	"time"
)

// InFlightFile is one path of a worktree whose in-flight edits include one
// by a session other than the asking one. An edit is in flight when no
// commit link covers it: neither it nor a later edit of the same path has a
// link from a commit made in that worktree. A commit takes a file's whole
// content, so the link of the newest committed edit covers every edit
// before it, whichever session made them. An edit with no after-content to
// match (a delete, an excluded path, a file over the chunk cap) never gets
// a link, so it stays in flight for the window.
type InFlightFile struct {
	// Path is relative to the worktree with forward slashes.
	Path string `json:"path"`
	// Sessions are the other sessions with in-flight edits of the path,
	// the most recent edit first.
	Sessions []InFlightSession `json:"sessions"`
	// LastEditAt is the newest in-flight edit by another session.
	LastEditAt time.Time `json:"last_edit_at"`
	// LinkedCommit is the commit the path's newest committed edit landed
	// in, nil when no edit the query read has a commit link.
	LinkedCommit *string `json:"linked_commit"`
}

// InFlightSession is one session's in-flight edits of one path.
type InFlightSession struct {
	ID string `json:"id"`
	// Title names the session the way the listing does; the server fills
	// it, since it reads the Session record.
	Title      string    `json:"title"`
	LastEditAt time.Time `json:"last_edit_at"`
	// Edits counts the session's in-flight edits of the path.
	Edits int `json:"edits"`
}

// InFlightQuery names the worktree whose edits InFlight reads and the
// asking session, whose own edits never count.
type InFlightQuery struct {
	Worktree string
	Exclude  string
}

// InFlight groups one worktree's edits by path and keeps the paths with an
// in-flight edit by a session other than q.Exclude, newest LastEditAt
// first, then by path. edits come newest first, as Edits lists them. Only
// a link of a commit made in q.Worktree counts: a commit in another
// checkout of the repository can link an edit here by an identical blob,
// and it says nothing about the files here. It is pure: the caller reads
// the edits and links from the store.
func InFlight(edits []Edit, links []CommitLink, q InFlightQuery) []InFlightFile {
	commits := linkedCommits(links, q.Worktree)
	var order []string
	byPath := map[string][]Edit{}
	for _, e := range edits {
		if _, seen := byPath[e.Path]; !seen {
			order = append(order, e.Path)
		}
		byPath[e.Path] = append(byPath[e.Path], e)
	}
	out := []InFlightFile{}
	for _, path := range order {
		if f, ok := inFlightFile(path, byPath[path], commits, q.Exclude); ok {
			out = append(out, f)
		}
	}
	slices.SortStableFunc(out, func(a, b InFlightFile) int {
		return cmp.Or(b.LastEditAt.Compare(a.LastEditAt), cmp.Compare(a.Path, b.Path))
	})
	return out
}

// linkedCommits maps each edit a link of worktree names to the newest
// commit that took it.
func linkedCommits(links []CommitLink, worktree string) map[string]string {
	newest := map[string]CommitLink{}
	for _, l := range links {
		if l.Worktree != worktree {
			continue
		}
		if cur, ok := newest[l.EditID]; !ok || l.At.After(cur.At) {
			newest[l.EditID] = l
		}
	}
	commits := make(map[string]string, len(newest))
	for id, l := range newest {
		commits[id] = l.Commit
	}
	return commits
}

// inFlightFile walks one path's edits newest first up to the newest one
// with a commit link, and counts the edits before it per other session.
func inFlightFile(
	path string,
	edits []Edit,
	commits map[string]string,
	exclude string,
) (InFlightFile, bool) {
	f := InFlightFile{Path: path}
	for _, e := range edits {
		if commit, ok := commits[e.ID]; ok {
			f.LinkedCommit = &commit
			break
		}
		if e.SessionID == exclude {
			continue
		}
		f.addEdit(e)
	}
	return f, len(f.Sessions) > 0
}

// addEdit counts an in-flight edit; edits arrive newest first, so the first
// one seen of each session is its latest.
func (f *InFlightFile) addEdit(e Edit) {
	if f.LastEditAt.IsZero() {
		f.LastEditAt = e.At
	}
	i := slices.IndexFunc(f.Sessions, func(s InFlightSession) bool { return s.ID == e.SessionID })
	if i < 0 {
		f.Sessions = append(f.Sessions, InFlightSession{ID: e.SessionID, LastEditAt: e.At})
		i = len(f.Sessions) - 1
	}
	f.Sessions[i].Edits++
}
