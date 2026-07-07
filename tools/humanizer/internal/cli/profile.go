package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/humanizer/internal/voice"
)

var (
	profileJSON        bool
	profileDiffAgainst string
)

var profileCmd = &cobra.Command{
	Use:   "profile [file]",
	Short: "Compute a quantitative voice profile (sentence stats, punctuation density, vocabulary)",
	Long: `Compute metrics about a writing sample: sentence length distribution,
punctuation density, contraction rate, hyphenated-pair rate, type-token ratio,
Flesch reading ease, and top bigrams/trigrams.

With --diff=<sample-file>, also prints a metric-by-metric delta between
the input and the sample — useful for matching a user's voice.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runProfile,
}

func init() {
	profileCmd.Flags().BoolVar(&profileJSON, "json", false, "emit the profile as JSON")
	profileCmd.Flags().
		StringVar(&profileDiffAgainst, "diff", "", "path to a sample file; also print the delta")
	rootCmd.AddCommand(profileCmd)
}

func runProfile(cmd *cobra.Command, args []string) error {
	text, err := readInput(args)
	if err != nil {
		return err
	}
	p := voice.Compute(text)

	if profileDiffAgainst == "" {
		if profileJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(p)
		}
		printProfile(cmd.OutOrStdout(), "input", p)
		return nil
	}

	sampleText, err := readInput([]string{profileDiffAgainst})
	if err != nil {
		return err
	}
	sample := voice.Compute(sampleText)
	diff := voice.DiffProfiles(p, sample)

	payload := struct {
		Draft  voice.Profile `json:"draft_profile"`
		Sample voice.Profile `json:"sample_profile"`
		Diff   voice.Diff    `json:"diff"`
	}{p, sample, diff}
	if profileJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(payload)
	}
	out := cmd.OutOrStdout()
	printProfile(out, "draft", p)
	fmt.Fprintln(out)
	printProfile(out, "sample", sample)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Diff (draft - sample, sorted by magnitude):")
	for _, m := range diff.Metrics {
		fmt.Fprintf(out, "  %-40s  %+8.2f  (draft=%.2f  sample=%.2f)  %s\n",
			m.Metric, m.Delta, m.Draft, m.Sample, m.Direction)
	}
	return nil
}

func printProfile(w io.Writer, label string, p voice.Profile) {
	fmt.Fprintf(w, "Profile: %s\n", label)
	fmt.Fprintf(w, "  words=%d  sentences=%d  paragraphs=%d  unique=%d  TTR=%.3f\n",
		p.WordCount, p.SentenceCount, p.ParagraphCount, p.UniqueWords, p.TypeTokenRatio)
	fmt.Fprintf(w, "  sentence_length mean=%.1f stddev=%.1f p50=%d p90=%d\n",
		p.SentenceLengthMean, p.SentenceLengthStdDev, p.SentenceLengthP50, p.SentenceLengthP90)
	fmt.Fprintf(
		w,
		"  density/100w  em-dash=%.2f semicolon=%.2f colon=%.2f paren=%.2f comma=%.2f hyphenated-pair=%.2f bold=%.2f contractions=%.2f\n",
		p.EmDashDensity,
		p.SemicolonDensity,
		p.ColonDensity,
		p.ParenDensity,
		p.CommaDensity,
		p.HyphenatedDensity,
		p.BoldDensity,
		p.ContractionRate,
	)
	fmt.Fprintf(w, "  flesch=%.1f\n", p.FleschReadingEase)
	if len(p.TopBigrams) > 0 {
		fmt.Fprintf(w, "  top-bigrams:")
		for _, g := range p.TopBigrams {
			fmt.Fprintf(w, " %q:%d", g.Text, g.Count)
		}
		fmt.Fprintln(w)
	}
	if len(p.TopTrigrams) > 0 {
		fmt.Fprintf(w, "  top-trigrams:")
		for _, g := range p.TopTrigrams {
			fmt.Fprintf(w, " %q:%d", g.Text, g.Count)
		}
		fmt.Fprintln(w)
	}
}
