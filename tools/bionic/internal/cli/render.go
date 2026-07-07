package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/bionic/internal/transform"
)

var renderCmd = &cobra.Command{
	Use:   "render [file]",
	Short: "Convert text to bionic reading format",
	Long: `Read markdown text from stdin or a file and output it in bionic reading
format where the first ~50% of each word is bolded.

Examples:
  echo "The quick brown fox" | bionic render
  bionic render README.md`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRender,
}

func init() {
	rootCmd.AddCommand(renderCmd)
}

func runRender(_ *cobra.Command, args []string) error {
	var r io.Reader = os.Stdin
	if len(args) == 1 && args[0] != "-" {
		f, err := os.Open(args[0])
		if err != nil {
			return fmt.Errorf("opening %s: %w", args[0], err)
		}
		defer f.Close()
		r = f
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}

	fmt.Print(transform.Bionic(string(data)))
	return nil
}
