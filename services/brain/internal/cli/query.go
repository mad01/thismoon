package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/search"
)

var queryCmd = &cobra.Command{
	Use:   "query <query>",
	Short: "Validate a zoekt query and print the parse tree",
	Long: `Parse a zoekt query string and print either its parse tree (if valid) or
the parse error with a fixing hint (if not).`,
	Args: cobra.ExactArgs(1),
	RunE: runQueryCmd,
}

func init() {
	rootCmd.AddCommand(queryCmd)
}

func runQueryCmd(cmd *cobra.Command, args []string) error {
	info := search.ValidateQuery(args[0])
	w := cmd.OutOrStdout()
	if info.Valid {
		fmt.Fprintf(w, "valid\nparsed: %s\n", info.Parsed)
		return nil
	}
	fmt.Fprintf(w, "invalid: %s\n", info.Error)
	if info.Hint != "" {
		fmt.Fprintf(w, "hint:    %s\n", info.Hint)
	}
	return fmt.Errorf("invalid query")
}
