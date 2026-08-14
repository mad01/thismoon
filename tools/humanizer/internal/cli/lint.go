package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/humanizer/internal/scrub"
)

var (
	lintAggressive     bool
	lintStripEmojiGlue bool
	lintJSON           bool
)

var lintCmd = &cobra.Command{
	Use:   "lint [file]",
	Short: "Report invisible/format Unicode and space-homoglyph watermark carriers",
	Long: `Scan text (from a file or stdin) for the deterministic "Layer A" watermark
carriers: zero-width and format controls, bidi overrides, tag characters,
variation selectors, and exotic space homoglyphs. Reports what it finds without
changing anything — use "humanizer fix" to apply the scrub.

Load-bearing invisibles (emoji ZWJ/variation selectors after an emoji base,
script joiners inside complex scripts, flag tag characters, orthographic
Arabic/Syriac marks) are preserved and not flagged unless --strip-emoji-glue is
set. Exits non-zero when any carrier is found.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runLint,
}

func init() {
	lintCmd.Flags().BoolVar(&lintAggressive, "aggressive", false,
		"also flag Cyrillic/fullwidth Latin confusable lookalikes")
	lintCmd.Flags().BoolVar(&lintStripEmojiGlue, "strip-emoji-glue", false,
		"paranoid: also flag load-bearing invisibles (emoji glue, script joiners, flag tags, orthographic Cf)")
	lintCmd.Flags().BoolVar(&lintJSON, "json", false, "emit the report as JSON")
	rootCmd.AddCommand(lintCmd)
}

func runLint(cmd *cobra.Command, args []string) error {
	text, err := readInput(args)
	if err != nil {
		return err
	}
	report := scrub.Inspect(text, scrub.Options{
		AggressiveHomoglyphs: lintAggressive,
		StripEmojiGlue:       lintStripEmojiGlue,
	})
	if lintJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return err
		}
	} else {
		printReport(cmd.OutOrStdout(), report)
	}
	if report.SuspiciousTotal > 0 {
		pendingExitCode = 1
	}
	return nil
}

func printReport(w io.Writer, r scrub.Report) {
	fmt.Fprintf(w, "Length: %d chars\n", r.Length)
	fmt.Fprintf(w, "Suspicious: %d\n", r.SuspiciousTotal)
	if len(r.Hits) > 0 {
		fmt.Fprintln(w, "Hits:")
		for _, h := range r.Hits {
			samples := h.SampleOffsets
			if len(samples) > 5 {
				samples = samples[:5]
			}
			fmt.Fprintf(w, "  [%s/%s] %s x%d @ %v\n", h.Kind, h.Confidence, h.Label, h.Count, samples)
		}
	}
	for _, n := range r.Notes {
		fmt.Fprintf(w, "Note: %s\n", n)
	}
}
