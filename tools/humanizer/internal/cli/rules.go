package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/humanizer/internal/rules"
)

var rulesCmd = &cobra.Command{
	Use:   "rules",
	Short: "List or explain the bundled humanizer detection rules",
}

var (
	rulesListCategory string
	rulesListJSON     bool
)

var rulesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List every humanizer rule with ID, category, default severity, and summary",
	RunE: func(cmd *cobra.Command, _ []string) error {
		all := rules.All()
		var filtered []rules.Rule
		if rulesListCategory == "" {
			filtered = all
		} else {
			cat := strings.ToLower(rulesListCategory)
			for _, r := range all {
				if strings.ToLower(r.Category) == cat {
					filtered = append(filtered, r)
				}
			}
		}
		if rulesListJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(filtered)
		}
		printRulesList(cmd.OutOrStdout(), filtered)
		return nil
	},
}

var rulesExplainCmd = &cobra.Command{
	Use:   "explain <rule_id>",
	Short: "Print full metadata for a single rule (rationale, before/after example, reference)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		r, ok := rules.Get(args[0])
		if !ok {
			return fmt.Errorf("unknown rule %q", args[0])
		}
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "ID:        %s\n", r.ID)
		fmt.Fprintf(out, "Name:      %s\n", r.Name)
		fmt.Fprintf(out, "Category:  %s\n", r.Category)
		fmt.Fprintf(out, "Severity:  %s\n", r.DefaultSeverity)
		fmt.Fprintf(out, "Summary:   %s\n", r.Summary)
		if r.Rationale != "" {
			fmt.Fprintf(out, "Rationale: %s\n", r.Rationale)
		}
		if r.Before != "" {
			fmt.Fprintf(out, "\nBefore:\n  %s\n", indent(r.Before, "  "))
		}
		if r.After != "" {
			fmt.Fprintf(out, "\nAfter:\n  %s\n", indent(r.After, "  "))
		}
		if r.Reference != "" {
			fmt.Fprintf(out, "\nReference: %s\n", r.Reference)
		}
		if r.File != "" {
			fmt.Fprintf(out, "File:      %s\n", r.File)
		}
		return nil
	},
}

func init() {
	rulesListCmd.Flags().
		StringVar(&rulesListCategory, "category", "", "filter by category (content|language|style|communication)")
	rulesListCmd.Flags().BoolVar(&rulesListJSON, "json", false, "emit rules as JSON")
	rulesCmd.AddCommand(rulesListCmd)
	rulesCmd.AddCommand(rulesExplainCmd)
	rootCmd.AddCommand(rulesCmd)
}

func printRulesList(w io.Writer, rs []rules.Rule) {
	if len(rs) == 0 {
		fmt.Fprintln(w, "no rules")
		return
	}
	for _, r := range rs {
		fmt.Fprintf(w, "%-12s %-10s %s\n  %s\n  %s\n\n",
			r.Category, r.DefaultSeverity, r.ID, r.Name, r.Summary)
	}
}

func indent(s, prefix string) string {
	return strings.ReplaceAll(s, "\n", "\n"+prefix)
}
