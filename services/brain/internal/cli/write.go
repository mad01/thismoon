package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/vault"
)

func newWriteCmd(a *app) *cobra.Command {
	var (
		opts     vault.WriteOptions
		tier     string
		body     string
		bodyFile string
		fields   fieldFlags
		distinct []string
	)
	c := &cobra.Command{
		Use:   "write",
		Short: "Create a new note in a tier",
		Long: `Create a new note in a tier (inbox, notes, outputs, or MOCs).

If --body is "-" or omitted, the body is read from stdin; --body-file reads
it from a file instead. The note, frontmatter and body, is scanned for
secrets first, and one found refuses the write with exit code 8. Use --tag and
--alias multiple times to add tags/aliases. --set key=value sets any
frontmatter key (the value is YAML, so --set 'sources=[{url: https://x}]'
stores a list); --set and --unset apply after the tier defaults. The note
goes to the default vault, or the one --vault names; a shared vault refuses
it with exit code 6. Fails with exit code 4 when the note already exists.

Before writing, brain looks for a note the new one likely duplicates, in the
target vault and in every shared vault: the same title or alias once case
and punctuation are ignored, or, when a semantic index exists, content at
least 0.92 alike. A hit refuses the write with exit code 9 and names the
candidates; extend one with brain edit, or name each with --distinct-from
<note> (repeatable), which stores it in distinct_from and lets the write
through. With -o json, prints {"vault": ..., "path": ...}.`,
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
			if bodyFile != "" && cmd.Flags().Changed("body") {
				return usageErrorf("--body and --body-file are mutually exclusive")
			}
			if opts.Body, err = readBody(cmd.InOrStdin(), body, bodyFile); err != nil {
				return err
			}
			opts.Tier = t
			opts.Evidence = fields.evidence
			return a.createNote(cmd, opts, distinct)
		},
	}
	f := c.Flags()
	f.StringVar(&tier, "tier", "inbox", "target tier (inbox, notes, outputs, MOCs)")
	f.StringVar(&opts.Title, "title", "", "note title (required)")
	f.StringVar(&body, "body", "-", `markdown body; "-" reads stdin`)
	f.StringVar(&bodyFile, "body-file", "", "read the markdown body from this file")
	f.StringSliceVar(&opts.Tags, "tag", nil, "frontmatter tag (repeatable)")
	f.StringSliceVar(&opts.Aliases, "alias", nil, "frontmatter alias (repeatable)")
	f.StringVar(&opts.Type, "type", "", "frontmatter type (note | source | moc | output)")
	f.StringVar(
		&opts.Status,
		"status",
		"",
		"frontmatter status (raw, validated, promoting, disputed, ...); defaults by tier",
	)
	f.StringVar(&opts.Source, "source", "", "source URL")
	f.StringVar(&opts.Filename, "filename", "", "override filename (without .md)")
	f.StringArrayVar(&distinct, "distinct-from", nil,
		"a note the new one is distinct from, though alike (repeatable); stored in distinct_from")
	fields.register(c)
	return c
}

// createNote writes a new note to the --vault vault (or the default one)
// through the duplicate check, prints its warnings on stderr and its path.
func (a *app) createNote(cmd *cobra.Command, opts vault.WriteOptions, distinct []string) error {
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
	abs, warnings, err := lib.Write(context.Background(), v, opts, distinct)
	if err != nil {
		return err
	}
	res := vault.PathResult{Vault: v.Name, Path: v.Rel(abs)}
	res.Warnings = append(warnings,
		lib.CitationWarnings(vault.Ref{Vault: v, Path: res.Path}, time.Now())...)
	warn(cmd, res.Warnings)
	return a.emit(cmd.OutOrStdout(), res, func(w io.Writer) error {
		fmt.Fprintln(w, a.show(res.Vault, res.Path))
		return nil
	})
}
