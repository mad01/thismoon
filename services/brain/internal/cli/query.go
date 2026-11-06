package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/search"
)

func newQueryCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "query <query>",
		Short: "Validate a zoekt query and print the parse tree",
		Long: `Parse a zoekt query string and print its parse tree. An invalid query
fails with exit code 5 and a fixing hint. With -o json, a valid query prints
the brain_query_validate document.`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			info := search.ValidateQuery(args[0])
			if !info.Valid {
				if info.Hint != "" {
					return fmt.Errorf(
						"%w: %s (hint: %s)",
						search.ErrInvalidQuery,
						info.Error,
						info.Hint,
					)
				}
				return fmt.Errorf("%w: %s", search.ErrInvalidQuery, info.Error)
			}
			return a.emit(cmd.OutOrStdout(), info, func(w io.Writer) error {
				fmt.Fprintf(w, "valid\nparsed: %s\n", info.Parsed)
				return nil
			})
		},
	}
}
