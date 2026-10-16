package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/humanizer/internal/glossary"
	"github.com/mad01/thismoon/tools/humanizer/internal/rules"
)

const (
	// steExitFindings is the exit code when the text has findings.
	steExitFindings = 1
	// steExitError is the exit code when the check could not run.
	steExitError = 2
)

var (
	steProfile     string
	steRepo        string
	steJSON        bool
	steMinSeverity string
	steGlossaryOut string
)

var steCmd = &cobra.Command{
	Use:   "ste [file|-]",
	Short: "Check an agent brief or report against the STE-lite rules (experimental)",
	Long: `Check text (from a file or stdin) against the STE-lite rules: a vale style
shaped on Simplified Technical English (ASD-STE100) for messages one agent
writes for another. --profile picks the rule set: procedure for task briefs
and steps, description for status reports and summaries. The command is
experimental: rules, profiles, and output may change.

With --repo, the _Avoid_ lists of every CONTEXT.md under that path become
STE.Glossary substitutions, reported at suggestion level.

Exit codes: 0 clean, 1 findings, 2 error. Integrations parse the --json
payload, not the exit code.`,
	Example: `  humanizer ste --profile procedure brief.md
  printf '%s' "$MSG" | humanizer ste --profile procedure --repo . --json
  humanizer ste glossary --repo . -o /tmp/Glossary.yml`,
	Args: steArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return withExitCode(steExitError, runSTE(cmd, args))
	},
}

var steGlossaryCmd = &cobra.Command{
	Use:   "glossary",
	Short: "Print the STE.Glossary rule generated from a repo's CONTEXT.md files (experimental)",
	Args:  steNoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return withExitCode(steExitError, runSTEGlossary(cmd))
	},
}

func init() {
	steCmd.PersistentFlags().StringVar(&steRepo, "repo", "",
		"repo whose CONTEXT.md glossaries become STE.Glossary substitutions")
	steCmd.Flags().StringVar(&steProfile, "profile", "",
		"rule set: procedure (briefs, steps) or description (reports, summaries)")
	steCmd.Flags().BoolVar(&steJSON, "json", false, "emit findings as JSON")
	steCmd.Flags().StringVar(&steMinSeverity, "min-severity", "",
		"filter findings below this severity (suggestion|warning|error)")
	steGlossaryCmd.Flags().StringVarP(&steGlossaryOut, "output", "o", "",
		"write the rule to this file instead of stdout")
	steCmd.SetFlagErrorFunc(steFlagError)
	steGlossaryCmd.SetFlagErrorFunc(steFlagError)
	steCmd.AddCommand(steGlossaryCmd)
	rootCmd.AddCommand(steCmd)
}

// steArgs, steNoArgs, and steFlagError give usage errors the error exit
// code, so exit 1 always means findings.
func steArgs(cmd *cobra.Command, args []string) error {
	return withExitCode(steExitError, cobra.MaximumNArgs(1)(cmd, args))
}

func steNoArgs(cmd *cobra.Command, args []string) error {
	return withExitCode(steExitError, cobra.NoArgs(cmd, args))
}

func steFlagError(_ *cobra.Command, err error) error {
	return withExitCode(steExitError, err)
}

// stePayload is the --json output: the humanizer_detect shape plus the
// profile that ran.
type stePayload struct {
	Findings []rules.Finding `json:"findings"`
	Summary  rules.Summary   `json:"summary"`
	Engine   string          `json:"engine"`
	Profile  rules.Profile   `json:"profile"`
}

func runSTE(cmd *cobra.Command, args []string) error {
	if steProfile == "" {
		return errors.New("--profile is required: procedure or description")
	}
	profile, err := rules.ParseProfile(steProfile)
	if err != nil {
		return err
	}
	text, err := readInput(args)
	if err != nil {
		return err
	}
	gloss, err := repoGlossary(steRepo)
	if err != nil {
		return err
	}
	findings, err := rules.Detect(context.Background(), text, rules.DetectOptions{
		Profile:     profile,
		Glossary:    gloss,
		MinSeverity: steMinSeverity,
	})
	if err != nil {
		return err
	}
	if err := printSTE(cmd.OutOrStdout(), profile, findings); err != nil {
		return err
	}
	if len(findings) > 0 {
		pendingExitCode = steExitFindings
	}
	return nil
}

// repoGlossary returns the generated glossary rule for repo, or nil when no
// repo is given.
func repoGlossary(repo string) ([]byte, error) {
	if repo == "" {
		return nil, nil
	}
	return glossary.FromRepo(repo)
}

func printSTE(w io.Writer, profile rules.Profile, findings []rules.Finding) error {
	if !steJSON {
		printFindings(w, findings)
		return nil
	}
	if findings == nil {
		findings = []rules.Finding{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(stePayload{
		Findings: findings,
		Summary:  rules.Summarize(findings),
		Engine:   "vale",
		Profile:  profile,
	})
}

func runSTEGlossary(cmd *cobra.Command) error {
	if steRepo == "" {
		return errors.New("--repo is required")
	}
	rule, err := glossary.FromRepo(steRepo)
	if err != nil {
		return err
	}
	if rule == nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "no CONTEXT.md glossary entries under %s\n", steRepo)
		return nil
	}
	if steGlossaryOut == "" {
		_, err := cmd.OutOrStdout().Write(rule)
		return err
	}
	if err := os.WriteFile(steGlossaryOut, rule, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", steGlossaryOut, err)
	}
	return nil
}
