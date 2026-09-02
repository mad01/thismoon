package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/humanizer/internal/backend"
	"github.com/mad01/thismoon/tools/humanizer/internal/judge"
)

var (
	judgeBackend string
	judgeModel   string
	judgeJSON    bool
)

const judgeTimeout = 90 * time.Second

var judgeCmd = &cobra.Command{
	Use:   "judge [file]",
	Short: "Holistic LLM judgment: does this passage read as AI-written?",
	Long: `Send text (from a file or stdin) to an LLM judge for a whole-passage
AI/human verdict. This is the fuzzy complement to the deterministic layers:
'detect' matches spans, 'detect --statistical' measures the sample, 'judge'
reads the passage the way a human reader does. Run all three for coverage.

The backend comes from the environment: HUMANIZER_BACKEND forces one,
otherwise the first configured provider wins. LITELLM_BASE_URL selects a
LiteLLM proxy (model claude-haiku-4-5-20251001, optional LITELLM_API_KEY);
else OPENROUTER_API_KEY selects OpenRouter (model anthropic/claude-haiku-4.5).
Override the model with --model or HUMANIZER_MODEL. Without any provider
configured the command fails; the deterministic commands keep working offline.

Verdicts are advisory. Treat likely_ai sections as rewrite targets alongside
detect findings, not as ground truth.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runJudge,
}

func init() {
	judgeCmd.Flags().
		StringVar(&judgeBackend, "backend", "", "force an LLM backend (litellm, openrouter); default auto-detects from env")
	judgeCmd.Flags().
		StringVar(&judgeModel, "model", "", "override the backend's default model id")
	judgeCmd.Flags().BoolVar(&judgeJSON, "json", false, "emit the verdict as JSON")
	rootCmd.AddCommand(judgeCmd)
}

func runJudge(cmd *cobra.Command, args []string) error {
	text, err := readInput(args)
	if err != nil {
		return err
	}
	b, err := backend.Select(judgeBackend, judgeModel)
	if err != nil {
		if errors.Is(err, backend.ErrNoBackend) {
			return fmt.Errorf(
				"%w; the deterministic passes (detect, detect --statistical) work without one",
				err,
			)
		}
		return err
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), judgeTimeout)
	defer cancel()
	v, err := judge.Run(ctx, b, text)
	if err != nil {
		return err
	}
	if judgeJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(
		out,
		"verdict: %s (confidence %.2f, %s %s)\n",
		v.Verdict,
		v.Confidence,
		v.Backend,
		v.Model,
	)
	fmt.Fprintf(out, "summary: %s\n", v.Summary)
	if len(v.Signals) > 0 {
		fmt.Fprintln(out)
		for _, s := range v.Signals {
			fmt.Fprintf(out, "%s  %s: %q\n", severityBadge(s.Severity), s.Pattern, s.Excerpt)
		}
	}
	return nil
}
