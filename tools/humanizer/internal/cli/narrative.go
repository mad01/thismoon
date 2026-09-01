package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/humanizer/internal/narrative"
)

var (
	narrativeJSON   bool
	narrativePrompt bool
)

var narrativeCmd = &cobra.Command{
	Use:   "narrative",
	Short: "Print the StoryScope narrative-feature rubric for LLM-judged fiction detection",
	Long: `Print the StoryScope rubric: 30 discourse-level narrative features
(thematic over-explanation, plot linearity, embodied emotion, intertextual
reference) that separate human-written from AI-generated fiction. These
features need a reader's judgment, not a regex: the command only serves
the rubric. To score a passage, run the --prompt output through an LLM
with the passage on stdin:

  printf '%s\n' "$PASSAGE" | claude -p --model sonnet "$(humanizer narrative --prompt)"`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		out := cmd.OutOrStdout()
		switch {
		case narrativeJSON:
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			return enc.Encode(narrative.BuildRubric())
		case narrativePrompt:
			fmt.Fprintln(out, narrative.Prompt())
			return nil
		default:
			features := narrative.Features()
			for _, theme := range narrative.Themes() {
				fmt.Fprintf(out, "%s\n", theme)
				for _, f := range features {
					if f.Theme != theme {
						continue
					}
					fmt.Fprintf(
						out,
						"  %-8s %s\n    %s\n    match: %s\n",
						f.Signal,
						f.Name,
						f.Question,
						f.Direction,
					)
					if f.Stat != "" {
						fmt.Fprintf(out, "    stat:  %s\n", f.Stat)
					}
				}
				fmt.Fprintln(out)
			}
			fmt.Fprintf(out, "source: %s\n", narrative.Source)
			return nil
		}
	},
}

func init() {
	narrativeCmd.Flags().BoolVar(&narrativeJSON, "json", false, "emit the rubric as JSON")
	narrativeCmd.Flags().
		BoolVar(&narrativePrompt, "prompt", false, "print only the judge prompt, ready to pipe into an LLM")
	narrativeCmd.MarkFlagsMutuallyExclusive("json", "prompt")
	rootCmd.AddCommand(narrativeCmd)
}
