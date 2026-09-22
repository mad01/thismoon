package cli

import (
	"context"
	"encoding/json"
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
    legacy-class → wk-* tag rewrite.

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

const (
	outcomeFromDoc   = "re-rendered-from-doc"
	outcomeUpgraded  = "upgraded-legacy-html"
	outcomeGraph     = "re-rendered-graph"
	outcomeUnchanged = "unchanged"
)

// rerenderOne re-renders a single page and returns its outcome. A page with a
// persisted Doc is re-rendered from source; otherwise its stored HTML is run
// through the legacy upgrader. A graph with a persisted source (graph.json) is
// re-rendered too; legacy JS graphs are left as-is. Files are overwritten and
// the version bumped only when the rendered output actually changed.
func rerenderOne(ctx context.Context, st store.Store, id string) (string, error) {
	p, err := st.Get(ctx, id)
	if err != nil {
		return "", err
	}

	var (
		newContent string
		fromDoc    bool
	)
	if st.HasDoc(ctx, id) {
		fromDoc = true
		raw, err := st.LoadDoc(ctx, id)
		if err != nil {
			return "", fmt.Errorf("load doc: %w", err)
		}
		var doc render.Doc
		if err := json.Unmarshal(raw, &doc); err != nil {
			return "", fmt.Errorf("parse doc: %w", err)
		}
		newContent, err = render.RenderDoc(doc, p.Title)
		if err != nil {
			return "", fmt.Errorf("render doc: %w", err)
		}
	} else {
		newContent = render.UpgradeLegacyHTML(p.Content)
	}

	newGraph := p.Graph
	if st.HasGraphSource(ctx, id) {
		raw, err := st.LoadGraphSource(ctx, id)
		if err != nil {
			return "", fmt.Errorf("load graph source: %w", err)
		}
		var g render.GraphInput
		if err := json.Unmarshal(raw, &g); err != nil {
			return "", fmt.Errorf("parse graph source: %w", err)
		}
		newGraph, err = render.RenderGraph(g)
		if err != nil {
			return "", fmt.Errorf("render graph: %w", err)
		}
	}

	var patch store.Patch
	if newContent != p.Content {
		patch.Content = &newContent
	}
	if newGraph != p.Graph {
		patch.Graph = &newGraph
	}
	if patch.Content == nil && patch.Graph == nil {
		return outcomeUnchanged, nil
	}
	if _, err := st.Update(ctx, id, patch); err != nil {
		return "", fmt.Errorf("update page: %w", err)
	}

	var outcome string
	switch {
	case patch.Content != nil && fromDoc:
		outcome = outcomeFromDoc
	case patch.Content != nil:
		outcome = outcomeUpgraded
	}
	if patch.Graph != nil {
		if outcome == "" {
			return outcomeGraph, nil
		}
		outcome += "+graph"
	}
	return outcome, nil
}
