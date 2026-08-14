package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/humanizer/internal/scrub"
)

var (
	fixNFKC         bool
	fixAggressive   bool
	fixNoNormSpaces bool
	fixStripEmoji   bool
	fixOutput       string
	fixInPlace      bool
	fixJSON         bool
)

var fixCmd = &cobra.Command{
	Use:   "fix [file]",
	Short: "Strip invisible Unicode and normalize space homoglyphs (Layer A scrub)",
	Long: `Apply the deterministic "Layer A" scrub: remove zero-width/format controls,
bidi overrides, tag characters, and variation selectors, and rewrite exotic
space homoglyphs to a plain ASCII space. This is non-intrusive — it does not
change visible characters.

The cleaned text goes to stdout by default, to a path with -o, or overwrites the
input file with --in-place (a .bak backup is written first). A stats summary
goes to stderr.

Risky, visibly-altering transforms are opt-in:
  --aggressive-homoglyphs  map Cyrillic/fullwidth Latin lookalikes to ASCII
  --nfkc                   apply Unicode NFKC normalization
  --strip-emoji-glue       also strip load-bearing invisibles (paranoid)`,
	Args: cobra.MaximumNArgs(1),
	RunE: runFix,
}

func init() {
	fixCmd.Flags().BoolVar(&fixNFKC, "nfkc", false, "apply Unicode NFKC after the scrub (risky: alters visible characters)")
	fixCmd.Flags().BoolVar(&fixAggressive, "aggressive-homoglyphs", false, "map Cyrillic/fullwidth Latin confusables to ASCII (risky)")
	fixCmd.Flags().BoolVar(&fixNoNormSpaces, "no-normalize-spaces", false, "do not rewrite exotic spaces to U+0020")
	fixCmd.Flags().BoolVar(&fixStripEmoji, "strip-emoji-glue", false, "paranoid: strip load-bearing invisibles too (risky)")
	fixCmd.Flags().StringVarP(&fixOutput, "output", "o", "", "write cleaned text here (default: stdout)")
	fixCmd.Flags().BoolVar(&fixInPlace, "in-place", false, "overwrite the input file (writes a .bak backup first)")
	fixCmd.Flags().BoolVar(&fixJSON, "json", false, "emit the stats summary as JSON on stderr")
	rootCmd.AddCommand(fixCmd)
}

func runFix(cmd *cobra.Command, args []string) error {
	text, err := readInput(args)
	if err != nil {
		return err
	}
	cleaned, stats := scrub.Clean(text, scrub.Options{
		NormalizeSpaces:      !fixNoNormSpaces,
		NFKC:                 fixNFKC,
		AggressiveHomoglyphs: fixAggressive,
		StripEmojiGlue:       fixStripEmoji,
	})

	if err := writeFixOutput(cmd, args, cleaned); err != nil {
		return err
	}

	errw := cmd.ErrOrStderr()
	if fixJSON {
		enc := json.NewEncoder(errw)
		enc.SetIndent("", "  ")
		return enc.Encode(stats)
	}
	fmt.Fprintf(errw, "removed=%d replaced=%d len %d->%d\n",
		stats.RemovedCount, stats.ReplacedCount, stats.InputLength, stats.OutputLength)
	return nil
}

func writeFixOutput(cmd *cobra.Command, args []string, cleaned string) error {
	if fixInPlace {
		if len(args) == 0 || args[0] == "-" {
			return fmt.Errorf("--in-place requires a file path")
		}
		src := args[0]
		orig, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("read %s: %w", src, err)
		}
		if err := os.WriteFile(src+".bak", orig, 0o644); err != nil {
			return fmt.Errorf("write backup %s.bak: %w", src, err)
		}
		if err := os.WriteFile(src, []byte(cleaned), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", src, err)
		}
		return nil
	}
	if fixOutput != "" {
		if err := os.WriteFile(fixOutput, []byte(cleaned), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", fixOutput, err)
		}
		return nil
	}
	fmt.Fprint(cmd.OutOrStdout(), cleaned)
	return nil
}
