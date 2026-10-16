package store

import "time"

// Sources are the canonical JSON a page's renditions were rendered from:
// Doc for the brief (doc.json), Deck for the deck (deck.json), and Graph
// for the graph script (graph.json). A nil field means the page has no such
// source, because that rendition is absent or came from legacy HTML or JS.
type Sources struct {
	Doc   []byte
	Deck  []byte
	Graph []byte
}

// Snapshot is a page as an edit reads and writes it: the page fully loaded,
// bodies included, plus its sources.
type Snapshot struct {
	Page    Page
	Sources Sources
}

// EditFunc computes a page's next state from its current one. A store runs
// it while it holds the page, under the filesystem store's lock or inside
// the cluster store's conflict retry, so it may run more than once and must
// do no I/O of its own. Of the page it returns, only the fields a Patch
// carries are kept (title, content, deck, graph, references, ephemeral,
// expiry); the store owns the id, version, timestamps, author, and share
// record. The returned sources replace the stored ones, and a nil source is
// deleted. An error aborts the edit with nothing written.
type EditFunc func(cur Snapshot) (Snapshot, error)

// ApplyEdit runs fn over cur and returns the state to persist: the editable
// fields and sources fn returned, folded into cur, with the version bumped
// and UpdatedAt set to now. Every store runs its edits through it so the
// semantics cannot drift between them.
func ApplyEdit(cur Snapshot, fn EditFunc, now time.Time) (Snapshot, error) {
	edited, err := fn(cur)
	if err != nil {
		return Snapshot{}, err
	}
	next := Snapshot{Page: foldEdit(cur.Page, edited.Page), Sources: edited.Sources}
	next.Page.HasDoc = next.Sources.Doc != nil
	next.Page.Version = cur.Page.Version + 1
	next.Page.UpdatedAt = now
	return next, nil
}

// PatchEdit returns the edit Update runs: patch applied to the page, the
// sources left as they are.
func PatchEdit(patch Patch) EditFunc {
	return func(cur Snapshot) (Snapshot, error) {
		cur.Page.Apply(patch)
		return cur, nil
	}
}

// foldEdit takes the editable fields of edited into cur through Apply, so
// the derived flags follow the rules a patch follows.
func foldEdit(cur, edited Page) Page {
	cur.Apply(Patch{
		Title:      &edited.Title,
		Content:    &edited.Content,
		Deck:       &edited.Deck,
		Graph:      &edited.Graph,
		References: &edited.References,
		Ephemeral:  &edited.Ephemeral,
		ExpiresAt:  edited.ExpiresAt,
	})
	return cur
}
