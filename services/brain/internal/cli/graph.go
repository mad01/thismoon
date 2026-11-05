package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/vault"
)

func newGraphCmd(a *app) *cobra.Command {
	var (
		center string
		opts   vault.GraphOptions
	)
	c := &cobra.Command{
		Use:   "graph",
		Short: "The wikilink graph of the vaults",
		Long: `Print the wikilink graph of the configured vaults, links resolved the way
backlinks resolves them. Text output lists one edge per line, then the node
and edge counts. --vault walks only that vault's notes; a note it links to in
another vault still appears. --center keeps the notes within --depth hops of
one note, links followed in either direction. With -o json, prints {nodes,
edges}: a node is {id, vault, path, stem, title, tier, type, status, tags,
evidence, review_by, degree, dangling} and an edge is {source, target, kind,
resolved}. A note's id is id:<note id>, or vault:path when it has none.`,
		Args: exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if opts.Depth < 0 {
				return usageErrorf("--depth must be >= 0")
			}
			lib, err := a.open()
			if err != nil {
				return err
			}
			opts.Sources = a.vaults
			if center != "" {
				ref, err := lib.Locate(a.vaults, center)
				if err != nil {
					return withNoteHint(err, refPath(center))
				}
				opts.Center = &ref
			}
			g, err := lib.Set().Graph(opts)
			if err != nil {
				return err
			}
			return a.emit(cmd.OutOrStdout(), g, func(w io.Writer) error {
				a.writeGraphText(w, g)
				return nil
			})
		},
	}
	c.Flags().StringVar(&center, "center", "", "keep the notes near this note")
	c.Flags().IntVar(&opts.Depth, "depth", 0, "hops from --center (default 1)")
	c.Flags().BoolVar(&opts.TagsAsNodes, "tags-as-nodes", false, "add a node per tag")
	return c
}

func (a *app) writeGraphText(w io.Writer, g vault.Graph) {
	byID := make(map[string]vault.GraphNode, len(g.Nodes))
	for _, n := range g.Nodes {
		byID[n.ID] = n
	}
	for _, e := range g.Edges {
		fmt.Fprintf(w, "%s -> %s\n", a.showNode(byID[e.Source]), a.showNode(byID[e.Target]))
	}
	fmt.Fprintf(w, "%d node(s), %d edge(s)\n", len(g.Nodes), len(g.Edges))
}

// showNode names a graph node the way the rest of the text output names
// notes: a bare path with one vault configured.
func (a *app) showNode(n vault.GraphNode) string {
	if n.Path != "" {
		return a.show(n.Vault, n.Path)
	}
	return n.String()
}
