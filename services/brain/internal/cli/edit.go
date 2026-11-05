package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/markdown"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

func newEditCmd(a *app) *cobra.Command {
	var (
		body, appendBody string
		patch            markdown.Frontmatter
		fields           fieldFlags
	)
	c := &cobra.Command{
		Use:   "edit <path>",
		Short: "Edit body or frontmatter of an existing note",
		Long: `Edit an existing note by vault-relative path.

Body handling (mutually exclusive):
  --body "..."      Replace the body entirely.
  --append "..."    Append to the body.
  --body -          Replace body from stdin.
  --append -        Append body from stdin.

Frontmatter patch flags merge into the existing frontmatter (non-empty wins):
  --status, --type, --title, --source, --evidence, --tag (repeatable),
  --alias (repeatable)
--set key=value (YAML value) and --unset key then change single keys; keys
brain does not model are kept as written.

At least one body or frontmatter flag must be provided. With -o json, prints
{"path": ...}.`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rel := args[0]
			if body != "" && appendBody != "" {
				return usageErrorf("--body and --append are mutually exclusive")
			}
			patch.Evidence = fields.evidence
			hasPatch := patch.Title != "" || patch.Type != "" || patch.Status != "" ||
				patch.Source != "" || patch.Evidence != "" ||
				len(patch.Tags) > 0 || len(patch.Aliases) > 0
			if body == "" && appendBody == "" && !hasPatch && !fields.given() {
				return usageErrorf("nothing to do: set --body, --append, or a frontmatter flag")
			}
			set, unset, err := fields.parse()
			if err != nil {
				return err
			}
			opts := vault.EditOptions{Set: set, Unset: unset}
			if hasPatch {
				opts.FrontmatterPatch = &patch
			}
			if opts.Body, err = readMaybeStdin(cmd.InOrStdin(), body); err != nil {
				return fmt.Errorf("read body: %w", err)
			}
			if opts.Append, err = readMaybeStdin(cmd.InOrStdin(), appendBody); err != nil {
				return fmt.Errorf("read append: %w", err)
			}
			_, v, err := a.openVault()
			if err != nil {
				return err
			}
			full, err := v.Resolve(rel)
			if err != nil {
				return err
			}
			if err := v.Edit(full, opts); err != nil {
				return withNoteHint(err, rel)
			}
			res := vault.PathResult{Path: rel}
			return a.emit(cmd.OutOrStdout(), res, func(w io.Writer) error {
				fmt.Fprintln(w, res.Path)
				return nil
			})
		},
	}
	f := c.Flags()
	f.StringVar(&body, "body", "", `replace body ("-" for stdin)`)
	f.StringVar(&appendBody, "append", "", `append to body ("-" for stdin)`)
	f.StringVar(&patch.Status, "status", "", "set frontmatter status")
	f.StringVar(&patch.Type, "type", "", "set frontmatter type")
	f.StringVar(&patch.Title, "title", "", "set frontmatter title")
	f.StringVar(&patch.Source, "source", "", "set frontmatter source")
	f.StringSliceVar(&patch.Tags, "tag", nil, "set frontmatter tags (repeatable, replaces all)")
	f.StringSliceVar(
		&patch.Aliases,
		"alias",
		nil,
		"set frontmatter aliases (repeatable, replaces all)",
	)
	fields.register(c)
	return c
}

// readMaybeStdin returns the flag value, or stdin's contents if the value is "-".
func readMaybeStdin(stdin io.Reader, val string) (string, error) {
	if val != "-" {
		return val, nil
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
