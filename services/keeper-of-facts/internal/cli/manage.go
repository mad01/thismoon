package cli

import (
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/keeper-of-facts/internal/client"
)

// apiClient targets the local serve agent. The CLI, like the MCP, is a thin
// client of serve (the single writer) rather than touching the store directly.
func apiClient() *client.Client {
	return client.New(fmt.Sprintf("http://localhost:%d", flagPort))
}

// parsePinFlag parses a --pin value of the form repo_path:file:start-end. It
// splits on the LAST two colons so a repo path may itself contain colons
// (the file path and line range never do). A leading ~ in repo_path is expanded.
func parsePinFlag(s string) (client.PinRef, error) {
	fileColon := strings.LastIndex(s, ":")
	if fileColon < 0 {
		return client.PinRef{}, fmt.Errorf("invalid --pin %q: want repo_path:file:start-end", s)
	}
	rangePart := s[fileColon+1:]
	rest := s[:fileColon]
	repoColon := strings.LastIndex(rest, ":")
	if repoColon < 0 {
		return client.PinRef{}, fmt.Errorf("invalid --pin %q: want repo_path:file:start-end", s)
	}
	repoPath := rest[:repoColon]
	file := rest[repoColon+1:]
	if repoPath == "" || file == "" {
		return client.PinRef{}, fmt.Errorf("invalid --pin %q: want repo_path:file:start-end", s)
	}

	dash := strings.Index(rangePart, "-")
	if dash < 0 {
		return client.PinRef{}, fmt.Errorf("invalid --pin %q: line range must be start-end", s)
	}
	start, err := strconv.Atoi(rangePart[:dash])
	if err != nil {
		return client.PinRef{}, fmt.Errorf("invalid --pin %q: bad start line", s)
	}
	end, err := strconv.Atoi(rangePart[dash+1:])
	if err != nil {
		return client.PinRef{}, fmt.Errorf("invalid --pin %q: bad end line", s)
	}

	return client.PinRef{
		RepoPath:  expandTilde(repoPath),
		File:      file,
		StartLine: start,
		EndLine:   end,
	}, nil
}

// shortHash trims a hex digest to a readable prefix.
func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

func printAssertion(cmd *cobra.Command, a client.Assertion) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, a.ID)
	fmt.Fprintf(out, "  statement:  %s\n", a.Statement)
	fmt.Fprintf(out, "  subject:    %s\n", a.Subject)
	fmt.Fprintf(out, "  kind:       %s\n", a.Kind)
	status := a.Status
	if a.StaleReason != "" {
		status += " (" + a.StaleReason + ")"
	}
	fmt.Fprintf(out, "  status:     %s\n", status)
	fmt.Fprintf(out, "  confidence: %s\n", a.Confidence)
	prov := fmt.Sprintf("session %s, derived %s",
		a.Provenance.SessionID, a.Provenance.DerivedAt.Local().Format("Mon Jan 2 15:04"))
	if a.Provenance.Author != "" {
		prov = a.Provenance.Author + ", " + prov
	}
	if a.Provenance.CostTokens > 0 {
		prov += fmt.Sprintf(", %d tokens", a.Provenance.CostTokens)
	}
	fmt.Fprintf(out, "  provenance: %s\n", prov)
	fmt.Fprintln(out, "  pins:")
	for _, p := range a.Pins {
		fmt.Fprintf(out, "    %s/%s:%d-%d @ %s\n",
			p.RepoPath, p.File, p.StartLine, p.EndLine, shortHash(p.ContentSHA256))
	}
	if len(a.Links) > 0 {
		fmt.Fprintln(out, "  links:")
		for _, l := range a.Links {
			fmt.Fprintf(out, "    %s\n", l)
		}
	}
	if a.RetractNote != "" {
		fmt.Fprintf(out, "  retracted:  %s\n", a.RetractNote)
	}
}

var (
	assertKind, assertSubject, assertStatement string
	assertConfidence, assertSession            string
	assertCostTokens                           int
	assertLinks, assertPins                    []string

	listSubject, listKind, listStatus string

	retractNote string
)

var assertCmd = &cobra.Command{
	Use:   "assert",
	Short: "Record an assertion pinned to evidence",
	Long: `Record a one-sentence assertion about how a system behaves, pinned to at
least one line range in a repo working tree. serve resolves and hashes each pin
before storing; an assertion it can't ground in evidence is rejected.

--pin takes repo_path:file:start-end and repeats (at least one required);
repo_path is the absolute path to the repo working tree (~ is expanded), e.g.
  --pin ~/code/src/github.com/mad01/thismoon:services/keeper-of-facts/internal/store/store.go:41-60`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if len(assertPins) == 0 {
			return fmt.Errorf("at least one --pin is required (repo_path:file:start-end)")
		}
		pins := make([]client.PinRef, 0, len(assertPins))
		for _, raw := range assertPins {
			p, err := parsePinFlag(raw)
			if err != nil {
				return err
			}
			pins = append(pins, p)
		}
		a, err := apiClient().Assert(client.AssertBody{
			Kind:       assertKind,
			Subject:    assertSubject,
			Statement:  assertStatement,
			Confidence: assertConfidence,
			SessionID:  assertSession,
			CostTokens: assertCostTokens,
			Links:      assertLinks,
			Pins:       pins,
		})
		if err != nil {
			return err
		}
		printAssertion(cmd, a)
		return nil
	},
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List assertions, newest first (--subject/--kind/--status to filter)",
	RunE: func(cmd *cobra.Command, _ []string) error {
		as, err := apiClient().List(listSubject, listKind, listStatus)
		if err != nil {
			return err
		}
		if len(as) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "no assertions")
			return nil
		}
		tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tKIND\tSUBJECT\tSTATUS\tCONFIDENCE\tCHECKED")
		for _, a := range as {
			checked := "-"
			if a.CheckedAt != nil {
				checked = a.CheckedAt.Local().Format("Mon Jan 2 15:04")
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
				a.ID, a.Kind, a.Subject, a.Status, a.Confidence, checked)
		}
		return tw.Flush()
	},
}

var getCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Show one assertion in full",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		a, err := apiClient().Get(args[0])
		if err != nil {
			return err
		}
		printAssertion(cmd, a)
		return nil
	},
}

var checkCmd = &cobra.Command{
	Use:   "check [id]",
	Short: "Re-hash pins and report fresh/stale/flipped (no id = check all)",
	Long: `Re-read each pin's line range from the working tree and compare against the
stored hash. With an id, checks that one assertion; with no id, walks the whole
store (skipping retracted ones). Prints the fresh/stale/flipped counts and, for
each stale assertion, its id and stale reason.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var id string
		if len(args) == 1 {
			id = args[0]
		}
		rep, err := apiClient().Check(id)
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "checked %d, fresh %d, stale %d, flipped %d\n",
			rep.Checked, rep.Fresh, rep.Stale, rep.Flipped)
		for _, a := range rep.Assertions {
			if a.Status == "stale" {
				fmt.Fprintf(out, "  %s  %s\n", a.ID, a.StaleReason)
			}
		}
		return nil
	},
}

var retractCmd = &cobra.Command{
	Use:   "retract <id>",
	Short: "Withdraw an assertion with a counter-evidence note (--note required)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if retractNote == "" {
			return fmt.Errorf("--note is required (why the assertion is withdrawn)")
		}
		a, err := apiClient().Retract(args[0], retractNote)
		if err != nil {
			return err
		}
		printAssertion(cmd, a)
		return nil
	},
}

func init() {
	assertCmd.Flags().StringVar(&assertKind, "kind", "",
		"kind: code-behavior|dead-end|preference|decision|machine-state|open-thread")
	assertCmd.Flags().StringVar(&assertSubject, "subject", "", "namespaced subject key")
	assertCmd.Flags().StringVar(&assertStatement, "statement", "", "the one-sentence assertion")
	assertCmd.Flags().
		StringVar(&assertConfidence, "confidence", "", "confidence: verified|derived|hint")
	assertCmd.Flags().StringVar(&assertSession, "session", "", "originating session id")
	assertCmd.Flags().
		IntVar(&assertCostTokens, "cost-tokens", 0, "tokens spent deriving it (optional)")
	assertCmd.Flags().StringArrayVar(&assertLinks, "link", nil, "related link (repeatable)")
	assertCmd.Flags().StringArrayVar(&assertPins, "pin", nil,
		"evidence pin repo_path:file:start-end (repeatable, at least one required)")

	listCmd.Flags().StringVar(&listSubject, "subject", "", "filter by subject prefix")
	listCmd.Flags().StringVar(&listKind, "kind", "", "filter by kind")
	listCmd.Flags().StringVar(&listStatus, "status", "", "filter: fresh|stale|retracted")

	retractCmd.Flags().StringVar(&retractNote, "note", "", "counter-evidence note (required)")

	rootCmd.AddCommand(assertCmd, listCmd, getCmd, checkCmd, retractCmd)
}
