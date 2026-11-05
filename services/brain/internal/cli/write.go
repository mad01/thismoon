package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/vault"
)

func newWriteCmd(a *app) *cobra.Command {
	var (
		opts   vault.WriteOptions
		tier   string
		body   string
		fields fieldFlags
	)
	c := &cobra.Command{
		Use:   "write",
		Short: "Create a new note in a tier",
		Long: `Create a new note in a tier (inbox, notes, outputs, or MOCs).

If --body is "-" or omitted, the body is read from stdin. Use --tag and
--alias multiple times to add tags/aliases. --set key=value sets any
frontmatter key (the value is YAML, so --set 'sources=[{url: https://x}]'
stores a list); --set and --unset apply after the tier defaults. The note
goes to the default vault, or the one --vault names; a shared vault refuses
it with exit code 6. Fails with exit code 4 when the note already exists.
With -o json, prints {"vault": ..., "path": ...}.`,
		Args: exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(opts.Title) == "" {
				return usageErrorf("--title is required")
			}
			t, err := vault.ParseTier(tier)
			if err != nil {
				return err
			}
			if opts.Set, opts.Unset, err = fields.parse(); err != nil {
				return err
			}
			if opts.Body, err = readMaybeStdin(cmd.InOrStdin(), body); err != nil {
				return fmt.Errorf("read stdin: %w", err)
			}
			name, err := a.writeVault()
			if err != nil {
				return err
			}
			lib, err := a.open()
			if err != nil {
				return err
			}
			v, err := lib.WriteTarget(name)
			if err != nil {
				return err
			}
			opts.Tier = t
			opts.Evidence = fields.evidence
			abs, err := v.Write(opts)
			if err != nil {
				return err
			}
			res := vault.PathResult{Vault: v.Name, Path: v.Rel(abs)}
			return a.emit(cmd.OutOrStdout(), res, func(w io.Writer) error {
				fmt.Fprintln(w, a.show(res.Vault, res.Path))
				return nil
			})
		},
	}
	f := c.Flags()
	f.StringVar(&tier, "tier", "inbox", "target tier (inbox, notes, outputs, MOCs)")
	f.StringVar(&opts.Title, "title", "", "note title (required)")
	f.StringVar(&body, "body", "-", `markdown body; "-" reads stdin`)
	f.StringSliceVar(&opts.Tags, "tag", nil, "frontmatter tag (repeatable)")
	f.StringSliceVar(&opts.Aliases, "alias", nil, "frontmatter alias (repeatable)")
	f.StringVar(&opts.Type, "type", "", "frontmatter type (note | source | moc | output)")
	f.StringVar(&opts.Status, "status", "", "frontmatter status (raw | validated | archived)")
	f.StringVar(&opts.Source, "source", "", "source URL")
	f.StringVar(&opts.Filename, "filename", "", "override filename (without .md)")
	fields.register(c)
	return c
}
