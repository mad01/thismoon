package cli

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/markdown"
)

var (
	readStartFlag int
	readEndFlag   int
)

var readCmd = &cobra.Command{
	Use:   "read <path>",
	Short: "Read a note (frontmatter + optional body slice)",
	Long: `Read a note by vault-relative path and print its frontmatter block
followed by the requested body lines. Use --start / --end to slice (1-based
inclusive over the body).`,
	Args: cobra.ExactArgs(1),
	RunE: runReadCmd,
}

func init() {
	readCmd.Flags().IntVar(&readStartFlag, "start", 0, "first body line to print (1-based)")
	readCmd.Flags().IntVar(&readEndFlag, "end", 0, "last body line to print (1-based inclusive)")
	rootCmd.AddCommand(readCmd)
}

func runReadCmd(cmd *cobra.Command, args []string) error {
	rel := args[0]
	if readStartFlag < 0 || readEndFlag < 0 {
		return fmt.Errorf("brain read: --start and --end must be >= 0")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("brain read: %w", err)
	}
	if filepath.IsAbs(rel) || strings.Contains(filepath.Clean(rel), "..") {
		return fmt.Errorf("brain read: path must be vault-relative: %s", rel)
	}
	full := filepath.Join(cfg.Vault, filepath.Clean(rel))

	data, err := os.ReadFile(full)
	if err != nil {
		return fmt.Errorf("brain read: %w", err)
	}
	fm, body, err := markdown.ParseFrontmatter(data)
	if err != nil {
		return fmt.Errorf("brain read: parse %s: %w", rel, err)
	}

	w := cmd.OutOrStdout()
	yml, err := yaml.Marshal(&fm)
	if err != nil {
		return fmt.Errorf("brain read: marshal frontmatter: %w", err)
	}
	fmt.Fprintln(w, "---")
	fmt.Fprint(w, string(yml))
	fmt.Fprintln(w, "---")

	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	n := 0
	for scanner.Scan() {
		n++
		if readStartFlag > 0 && n < readStartFlag {
			continue
		}
		if readEndFlag > 0 && n > readEndFlag {
			break
		}
		fmt.Fprintln(w, scanner.Text())
	}
	return nil
}
