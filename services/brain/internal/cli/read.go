package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

func newReadCmd(a *app) *cobra.Command {
	var start, end int
	c := &cobra.Command{
		Use:   "read <note>",
		Short: "Read a note (frontmatter + optional body slice)",
		Long: `Read a note and print its frontmatter block followed by the requested body
lines. Name the note by vault:path, or by a vault-relative path found in
exactly one vault. Use --start / --end to slice (1-based inclusive over the
body). With -o json, prints the brain_read document.`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if start < 0 || end < 0 {
				return usageErrorf("--start and --end must be >= 0")
			}
			if start > 0 && end > 0 && start > end {
				return usageErrorf("--start (%d) must be <= --end (%d)", start, end)
			}
			_, ref, err := a.locate(args[0])
			if err != nil {
				return err
			}
			note, err := ref.Vault.Read(ref.Path, start, end)
			if err != nil {
				return withNoteHint(err, ref.Path)
			}
			return a.emit(cmd.OutOrStdout(), note, func(w io.Writer) error {
				if err := writeFrontmatter(w, note.Raw()); err != nil {
					return err
				}
				for _, l := range note.BodyLines {
					fmt.Fprintln(w, l.Text)
				}
				return nil
			})
		},
	}
	c.Flags().IntVar(&start, "start", 0, "first body line to print (1-based)")
	c.Flags().IntVar(&end, "end", 0, "last body line to print (1-based inclusive)")
	return c
}

func newPeekCmd(a *app) *cobra.Command {
	var lines int
	c := &cobra.Command{
		Use:   "peek <note>",
		Short: "Frontmatter, first body lines and backlink count of a note",
		Long: `Cheap overview of a note: its inbound backlink count, its frontmatter
and the first --lines body lines (default 20). Use it to decide whether a note
is worth a full read. With -o json, prints the brain_peek document.`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if lines < 0 {
				return usageErrorf("--lines must be >= 0")
			}
			_, ref, err := a.locate(args[0])
			if err != nil {
				return err
			}
			peek, err := ref.Vault.Peek(ref.Path, lines)
			if err != nil {
				return withNoteHint(err, ref.Path)
			}
			return a.emit(cmd.OutOrStdout(), peek, func(w io.Writer) error {
				fmt.Fprintf(w, "backlinks: %d\n", peek.BacklinkCount)
				if err := writeFrontmatter(w, peek.Raw()); err != nil {
					return err
				}
				for _, l := range peek.Preview {
					fmt.Fprintln(w, l)
				}
				return nil
			})
		},
	}
	c.Flags().IntVar(&lines, "lines", 20, "number of body lines to preview")
	return c
}
