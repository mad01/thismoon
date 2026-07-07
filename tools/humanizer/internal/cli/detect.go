package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/humanizer/internal/rules"
	"github.com/mad01/thismoon/tools/humanizer/internal/voice"
)

var (
	detectMinSeverity string
	detectRules       []string
	detectJSON        bool
	detectStatistical bool
)

var detectCmd = &cobra.Command{
	Use:   "detect [file]",
	Short: "Scan text for AI-writing patterns",
	Long: `Scan text (from a file or stdin) for AI-writing patterns using the bundled
Vale Humanizer style pack. Prints findings grouped by severity.

With --json, emits the same payload humanizer_detect would return over MCP.
With --statistical, runs the size-gated statistical checks (sentence-length
uniformity, contraction rate, lexical diversity, anaphora, ...) instead of
the Vale span rules — the humanizer_detect_statistical payload.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDetect,
}

func init() {
	detectCmd.Flags().
		StringVar(&detectMinSeverity, "min-severity", "", "filter findings below this severity (suggestion|warning|error)")
	detectCmd.Flags().
		StringSliceVar(&detectRules, "rule", nil, "restrict detection to this rule ID (repeatable)")
	detectCmd.Flags().BoolVar(&detectJSON, "json", false, "emit findings as JSON")
	detectCmd.Flags().
		BoolVar(&detectStatistical, "statistical", false, "run statistical checks instead of Vale span rules")
	rootCmd.AddCommand(detectCmd)
}

func runDetect(cmd *cobra.Command, args []string) error {
	text, err := readInput(args)
	if err != nil {
		return err
	}
	if detectStatistical {
		return runDetectStatistical(cmd, text)
	}
	findings, err := rules.Detect(context.Background(), text, rules.DetectOptions{
		Rules:       detectRules,
		MinSeverity: detectMinSeverity,
	})
	if err != nil {
		notInstalled := &rules.ValeNotInstalledError{}
		if errors.As(err, &notInstalled) {
			fmt.Fprintln(cmd.ErrOrStderr(), err)
			return err
		}
		return err
	}
	if detectJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(findings)
	}
	printFindings(cmd.OutOrStdout(), findings)
	return nil
}

func runDetectStatistical(cmd *cobra.Command, text string) error {
	findings := voice.DetectStatistical(text)
	if detectJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(findings)
	}
	printStatFindings(cmd.OutOrStdout(), findings)
	return nil
}

func printStatFindings(w io.Writer, findings []voice.StatFinding) {
	if len(findings) == 0 {
		fmt.Fprintln(w, "no statistical findings")
		return
	}
	counts := map[string]int{}
	for _, f := range findings {
		counts[f.Severity]++
		fmt.Fprintf(w, "%s  [%s]  %s\n", severityBadge(f.Severity), f.RuleID, f.Message)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%d findings (error=%d warning=%d suggestion=%d)\n",
		len(findings), counts["error"], counts["warning"], counts["suggestion"])
}

func readInput(args []string) (string, error) {
	if len(args) == 0 || args[0] == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		return string(data), nil
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		return "", fmt.Errorf("read %s: %w", args[0], err)
	}
	return string(data), nil
}

func printFindings(w io.Writer, findings []rules.Finding) {
	if len(findings) == 0 {
		fmt.Fprintln(w, "no findings")
		return
	}
	counts := map[string]int{}
	for _, f := range findings {
		counts[f.Severity]++
		line := fmt.Sprintf(
			"%s:%d:%d  [%s]  %s: %s",
			severityBadge(f.Severity),
			f.Line,
			f.Column,
			f.RuleID,
			f.Match,
			f.Message,
		)
		fmt.Fprintln(w, line)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%d findings (error=%d warning=%d suggestion=%d)\n",
		len(findings), counts["error"], counts["warning"], counts["suggestion"])
}

func severityBadge(s string) string {
	switch s {
	case "error":
		return "ERR "
	case "warning":
		return "WARN"
	case "suggestion":
		return "INFO"
	}
	return " ?  "
}
