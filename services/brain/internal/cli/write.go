package cli

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

var (
	writeTierFlag     string
	writeTitleFlag    string
	writeBodyFlag     string
	writeTagsFlag     []string
	writeAliasesFlag  []string
	writeTypeFlag     string
	writeStatusFlag   string
	writeSourceFlag   string
	writeFilenameFlag string
)

var writeCmd = &cobra.Command{
	Use:   "write",
	Short: "Create a new note in a tier",
	Long: `Create a new note in a tier (inbox, notes, outputs, or MOCs).

If --body is "-" or omitted, the body is read from stdin. Use --tag and
--alias multiple times to add tags/aliases.`,
	RunE: runWriteCmd,
}

func init() {
	writeCmd.Flags().
		StringVar(&writeTierFlag, "tier", "inbox", "target tier (inbox, notes, outputs, MOCs)")
	writeCmd.Flags().StringVar(&writeTitleFlag, "title", "", "note title (required)")
	writeCmd.Flags().StringVar(&writeBodyFlag, "body", "-", `markdown body; "-" reads stdin`)
	writeCmd.Flags().StringSliceVar(&writeTagsFlag, "tag", nil, "frontmatter tag (repeatable)")
	writeCmd.Flags().
		StringSliceVar(&writeAliasesFlag, "alias", nil, "frontmatter alias (repeatable)")
	writeCmd.Flags().
		StringVar(&writeTypeFlag, "type", "", "frontmatter type (note | source | moc | output)")
	writeCmd.Flags().
		StringVar(&writeStatusFlag, "status", "", "frontmatter status (raw | validated | archived)")
	writeCmd.Flags().StringVar(&writeSourceFlag, "source", "", "source URL")
	writeCmd.Flags().
		StringVar(&writeFilenameFlag, "filename", "", "override filename (without .md)")
	_ = writeCmd.MarkFlagRequired("title")
	rootCmd.AddCommand(writeCmd)
}

func runWriteCmd(cmd *cobra.Command, _ []string) error {
	tier, err := vault.ParseTier(writeTierFlag)
	if err != nil {
		return fmt.Errorf("brain write: %w", err)
	}

	body := writeBodyFlag
	if body == "-" {
		data, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return fmt.Errorf("brain write: read stdin: %w", err)
		}
		body = string(data)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("brain write: %w", err)
	}
	v, err := vault.Open(cfg.Vault)
	if err != nil {
		return fmt.Errorf("brain write: %w", err)
	}

	abs, err := v.Write(vault.WriteOptions{
		Tier:     tier,
		Title:    writeTitleFlag,
		Body:     body,
		Tags:     writeTagsFlag,
		Aliases:  writeAliasesFlag,
		Type:     writeTypeFlag,
		Status:   writeStatusFlag,
		Source:   writeSourceFlag,
		Filename: writeFilenameFlag,
	})
	if err != nil {
		return fmt.Errorf("brain write: %w", err)
	}
	rel, err := filepath.Rel(cfg.Vault, abs)
	if err != nil {
		rel = abs
	}
	fmt.Fprintln(cmd.OutOrStdout(), filepath.ToSlash(rel))
	return nil
}
