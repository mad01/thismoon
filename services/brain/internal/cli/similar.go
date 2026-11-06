package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/library"
)

func newSimilarCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "similar <note>",
		Short: "List notes alike to a note, across every vault",
		Long: `List the notes that look like the given one, in every configured vault: a
title or alias equal to the note's title or one of its aliases once case and
punctuation are folded (the duplicate check's rule), then, when a semantic
index exists, notes whose content is at least 0.85 alike to the note's
first chunk (lower than write's 0.92, since nothing is refused here). The
note itself, a copy with the same id in another vault, and revoked notes
are left out. The index is used as it is, never refreshed.

similar is a warning, never a gate: it exits 0 whatever it finds, so a
shared vault's CI can run it on the notes a pull request adds and comment
with the result. -o text prints one line per note, -o json {"vault",
"path", "similar": [{vault, path, title, match, score}], "warnings"}, and
-o github one warning annotation per similar note.`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			lib, ref, err := a.locate(args[0])
			if err != nil {
				return err
			}
			report, err := lib.Similar(context.Background(), ref)
			if err != nil {
				return withNoteHint(err, ref.Path)
			}
			warn(cmd, report.Warnings)
			w := cmd.OutOrStdout()
			switch a.output {
			case outputJSON:
				return writeJSON(w, report)
			case outputGitHub:
				writeSimilarGitHub(w, lib, report)
			default:
				a.writeSimilarText(w, report)
			}
			return nil
		},
	}
}

func (a *app) writeSimilarText(w io.Writer, r library.SimilarReport) {
	if len(r.Similar) == 0 {
		fmt.Fprintf(w, "no note looks like %s\n", a.show(r.Vault, r.Path))
		return
	}
	for _, s := range r.Similar {
		fmt.Fprintf(w, "%s  %s\n", a.show(s.Vault, s.Path), similarWhy(s))
	}
}

// writeSimilarGitHub prints one warning annotation per similar note, on
// the file of the note it looks like.
func writeSimilarGitHub(w io.Writer, lib *library.Library, r library.SimilarReport) {
	root := ""
	if v, err := lib.Set().Get(r.Vault); err == nil {
		root = v.Root
	}
	cwd, _ := os.Getwd()
	file := annotationFile(cwd, root, r.Path)
	for _, s := range r.Similar {
		fmt.Fprintf(w, "::warning file=%s,title=%s::%s\n",
			escapeProperty(file), escapeProperty("brain similar"),
			escapeData(fmt.Sprintf("looks like %s:%s, %s; extend that note instead, "+
				"or say in the pull request why both are needed", s.Vault, s.Path, similarWhy(s))))
	}
}

// similarWhy says how a similar note matched.
func similarWhy(s library.SimilarNote) string {
	switch s.Match {
	case library.MatchSemantic:
		return fmt.Sprintf("(similarity %.2f)", s.Score)
	case library.MatchAlias:
		return fmt.Sprintf("(an alias of %q)", s.Title)
	}
	return "(same title)"
}
