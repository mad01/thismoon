package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

func newReadCmd(a *app) *cobra.Command {
	var start, end int
	c := &cobra.Command{
		Use:   "read <path>",
		Short: "Read a note (frontmatter + optional body slice)",
		Long: `Read a note by vault-relative path and print its frontmatter block
followed by the requested body lines. Use --start / --end to slice (1-based
inclusive over the body). With -o json, prints the brain_read document.`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if start < 0 || end < 0 {
				return usageErrorf("--start and --end must be >= 0")
			}
			if start > 0 && end > 0 && start > end {
				return usageErrorf("--start (%d) must be <= --end (%d)", start, end)
			}
			_, v, err := a.openVault()
			if err != nil {
				return err
			}
			note, err := v.Read(args[0], start, end)
			if err != nil {
				return withNoteHint(err, args[0])
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
		Use:   "peek <path>",
		Short: "Frontmatter, first body lines and backlink count of a note",
		Long: `Cheap overview of a note: its inbound backlink count, its frontmatter
and the first --lines body lines (default 20). Use it to decide whether a note
is worth a full read. With -o json, prints the brain_peek document.`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if lines < 0 {
				return usageErrorf("--lines must be >= 0")
			}
			_, v, err := a.openVault()
			if err != nil {
				return err
			}
			peek, err := v.Peek(args[0], lines)
			if err != nil {
				return withNoteHint(err, args[0])
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
