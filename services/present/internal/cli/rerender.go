package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/present/internal/render"
	"github.com/mad01/thismoon/services/present/internal/store"
)

var rerenderCmd = &cobra.Command{
	Use:   "rerender [id...]",
	Short: "Re-render stored pages to pick up renderer/template changes",
	Long: `Re-render presentation pages so they reflect the current renderer and
webkit markup. With no arguments, every page is processed; pass one or more ids
to limit the scope.

For each page:
  - if a Doc source (doc.json) exists, the page is re-rendered from that source
    (a true re-render that picks up renderer/template changes);
  - otherwise the stored content.html is upgraded in place via a deterministic
    legacy-class → wk-* tag rewrite;
  - a slide deck (deck.json) and a structured graph (graph.json) are
    re-rendered from their sources the same way.

Pages whose markup is already current are left unchanged. Modified pages get a
bumped version so any open browser tab live-reloads.`,
	RunE: runRerender,
}

func init() {
	rootCmd.AddCommand(rerenderCmd)
}

func runRerender(_ *cobra.Command, args []string) error {
	st, err := store.NewFS(flagWorkdir)
	if err != nil {
		return err
	}
	return rerenderPages(context.Background(), st, args, os.Stdout)
}

// rerenderPages re-renders the given pages (all pages when ids is empty),
// writing a one-line outcome per page to w. It is the testable core of the
// rerender command.
func rerenderPages(ctx context.Context, st store.Store, ids []string, w io.Writer) error {
	if len(ids) == 0 {
		metas, err := st.ListMeta(ctx)
		if err != nil {
			return err
		}
		for _, m := range metas {
			ids = append(ids, m.ID)
		}
	}
	for _, id := range ids {
		outcome, err := rerenderOne(ctx, st, id)
		if err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
		fmt.Fprintf(w, "%s: %s\n", id, outcome)
	}
	return nil
}

// rerenderSweep re-renders every page and logs one summary line, plus one
// line per page that failed. serve runs it in the background at startup so a
// binary with a newer renderer or webkit refreshes the stored pages on its
// own; rerenderOne writes only when the output changed, so a sweep over
// current pages is read-only.
func rerenderSweep(ctx context.Context, st store.Store, logf func(string, ...any)) {
	metas, err := st.ListMeta(ctx)
	if err != nil {
		logf("present: rerender sweep: list pages: %v", err)
		return
	}
	changed, failed := 0, 0
	for _, m := range metas {
		if ctx.Err() != nil {
			return
		}
		outcome, err := rerenderOne(ctx, st, m.ID)
		switch {
		case err != nil:
			failed++
			logf("present: rerender sweep: %s: %v", m.ID, err)
		case outcome != outcomeUnchanged:
			changed++
		}
	}
	logf(
		"present: rerender sweep: %d pages, %d re-rendered, %d failed",
		len(metas),
		changed,
		failed,
	)
}

const (
	outcomeFromDoc   = "re-rendered-from-doc"
	outcomeUpgraded  = "upgraded-legacy-html"
	outcomeGraph     = "re-rendered-graph"
	outcomeDeck      = "re-rendered-deck"
	outcomeUnchanged = "unchanged"
)

// rerenderOne re-renders a single page and returns its outcome. A page with a
// persisted Doc is re-rendered from source; otherwise its stored HTML is run
// through the legacy upgrader. A deck (deck.json) and a graph with a persisted
// source (graph.json) are re-rendered too; legacy JS graphs are left as-is.
// The render runs inside a store edit, so it reads the sources and writes the
// output under the page lock, and an edit landing meanwhile is never
// overwritten with output from older sources. Files are overwritten and the
// version bumped only when the rendered output actually changed.
func rerenderOne(ctx context.Context, st store.Store, id string) (string, error) {
	var outcome string
	var renderErr error
	_, err := st.Edit(ctx, id, func(cur store.Snapshot) (store.Snapshot, error) {
		next, moved, err := rerenderSnapshot(cur)
		if err != nil {
			renderErr = err
			return store.Snapshot{}, err
		}
		if moved == "" {
			return store.Snapshot{}, store.ErrUnchanged
		}
		outcome = moved
		return next, nil
	})
	switch {
	case errors.Is(err, store.ErrUnchanged):
		return outcomeUnchanged, nil
	case renderErr != nil && errors.Is(err, renderErr):
		return "", err
	case err != nil:
		// The store failed reading the page and its sources (a doc read
		// failure carries "load doc:") or writing the result.
		return "", fmt.Errorf("update page: %w", err)
	}
	return outcome, nil
}

// rerenderSnapshot renders cur's renditions afresh and names what moved: the
// brief from its Doc (or the legacy upgrade of its HTML), the deck and the
// graph from their sources. An empty outcome means the output is unchanged.
// It is pure, so it can run inside an edit.
func rerenderSnapshot(cur store.Snapshot) (store.Snapshot, string, error) {
	p := cur.Page
	newContent, fromDoc, err := rerenderBrief(cur)
	if err != nil {
		return store.Snapshot{}, "", err
	}
	newDeck := p.Deck
	if cur.Sources.Deck != nil {
		c, err := render.CompileDeck(cur.Sources.Deck, p.Title)
		if err != nil {
			return store.Snapshot{}, "", fmt.Errorf("render deck: %w", err)
		}
		newDeck = c.HTML
	}
	newGraph := p.Graph
	if cur.Sources.Graph != nil {
		var g render.GraphInput
		if err := json.Unmarshal(cur.Sources.Graph, &g); err != nil {
			return store.Snapshot{}, "", fmt.Errorf("parse graph source: %w", err)
		}
		if newGraph, err = render.RenderGraph(g); err != nil {
			return store.Snapshot{}, "", fmt.Errorf("render graph: %w", err)
		}
	}

	var outcome string
	switch {
	case newContent != p.Content && fromDoc:
		outcome = outcomeFromDoc
	case newContent != p.Content:
		outcome = outcomeUpgraded
	}
	outcome = joinOutcome(outcome, newDeck != p.Deck, outcomeDeck, "+deck")
	outcome = joinOutcome(outcome, newGraph != p.Graph, outcomeGraph, "+graph")
	next := cur
	next.Page.Content, next.Page.Deck, next.Page.Graph = newContent, newDeck, newGraph
	return next, outcome, nil
}

// rerenderBrief renders the brief afresh: from the Doc when the page has
// one, else as the legacy upgrade of its stored HTML. fromDoc says which.
func rerenderBrief(cur store.Snapshot) (html string, fromDoc bool, err error) {
	switch {
	case cur.Sources.Doc != nil:
		c, err := render.Compile(cur.Sources.Doc, cur.Page.Title)
		if err != nil {
			return "", false, err
		}
		return c.HTML, true, nil
	case cur.Page.HasBrief:
		return render.UpgradeLegacyHTML(cur.Page.Content), false, nil
	default:
		return cur.Page.Content, false, nil
	}
}

// joinOutcome appends a changed artifact to the outcome: the full name when
// it is the first change reported, the short suffix after another one.
func joinOutcome(outcome string, changed bool, alone, suffix string) string {
	switch {
	case !changed:
		return outcome
	case outcome == "":
		return alone
	default:
		return outcome + suffix
	}
}
