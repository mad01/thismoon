package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/srtspec/internal/verify"
)

func verifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "verify [root]",
		Short: "Fail when a settings file or expansion layer under a repo is not what its source says",
		Long: `Check every sandbox artifact under a repo root (default .) without writing
anything. Every srt manifest must render to an existing settings file with
the same bytes, every recipes/*/*.srt.json must have a manifest, and every
*.local.json beside a recipe must only widen the settings file it sits next
to. One line per check; exit 1 when any fails. generate fixes the renders.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			results, err := verify.Run(root, home)
			if err != nil {
				return err
			}
			if failed := report(cmd.OutOrStdout(), results); failed > 0 {
				return fmt.Errorf(
					"%d of %d sandbox checks under %s failed",
					failed,
					len(results),
					root,
				)
			}
			return nil
		},
	}
}

// report prints one line per result and returns how many failed.
func report(w io.Writer, results []verify.Result) int {
	failed := 0
	for _, r := range results {
		if r.OK() {
			fmt.Fprintf(w, "ok      %s (%s)\n", r.Path, r.Note)
			continue
		}
		failed++
		fmt.Fprintf(w, "FAIL    %s: %s\n", r.Path, r.Reason)
	}
	return failed
}
