package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/markdown"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

var (
	editBodyFlag    string
	editAppendFlag  string
	editStatusFlag  string
	editTypeFlag    string
	editTitleFlag   string
	editSourceFlag  string
	editTagsFlag    []string
	editAliasesFlag []string
)

var editCmd = &cobra.Command{
	Use:   "edit <path>",
	Short: "Edit body or frontmatter of an existing note",
	Long: `Edit an existing note by vault-relative path.

Body handling (mutually exclusive):
  --body "..."      Replace the body entirely.
  --append "..."    Append to the body.
  --body -          Replace body from stdin.
  --append -        Append body from stdin.

Frontmatter patch flags merge into the existing frontmatter (non-empty wins):
  --status, --type, --title, --source, --tag (repeatable), --alias (repeatable)

At least one of body/append/frontmatter-patch must be provided.`,
	Args: cobra.ExactArgs(1),
	RunE: runEditCmd,
}

func init() {
	editCmd.Flags().StringVar(&editBodyFlag, "body", "", `replace body ("-" for stdin)`)
	editCmd.Flags().StringVar(&editAppendFlag, "append", "", `append to body ("-" for stdin)`)
	editCmd.Flags().StringVar(&editStatusFlag, "status", "", "set frontmatter status")
	editCmd.Flags().StringVar(&editTypeFlag, "type", "", "set frontmatter type")
	editCmd.Flags().StringVar(&editTitleFlag, "title", "", "set frontmatter title")
	editCmd.Flags().StringVar(&editSourceFlag, "source", "", "set frontmatter source")
	editCmd.Flags().
		StringSliceVar(&editTagsFlag, "tag", nil, "set frontmatter tags (repeatable, replaces all)")
	editCmd.Flags().
		StringSliceVar(&editAliasesFlag, "alias", nil, "set frontmatter aliases (repeatable, replaces all)")
	rootCmd.AddCommand(editCmd)
}

func runEditCmd(cmd *cobra.Command, args []string) error {
	rel := args[0]
	if editBodyFlag != "" && editAppendFlag != "" {
		return fmt.Errorf("brain edit: --body and --append are mutually exclusive")
	}

	body, err := readMaybeStdin(cmd.InOrStdin(), editBodyFlag)
	if err != nil {
		return fmt.Errorf("brain edit: read body: %w", err)
	}
	appendBody, err := readMaybeStdin(cmd.InOrStdin(), editAppendFlag)
	if err != nil {
		return fmt.Errorf("brain edit: read append: %w", err)
	}

	patch := &markdown.Frontmatter{
		Title:   editTitleFlag,
		Aliases: editAliasesFlag,
		Tags:    editTagsFlag,
		Type:    editTypeFlag,
		Status:  editStatusFlag,
		Source:  editSourceFlag,
	}
	hasPatch := patch.Title != "" || patch.Type != "" || patch.Status != "" || patch.Source != "" ||
		len(patch.Tags) > 0 ||
		len(patch.Aliases) > 0
	if body == "" && appendBody == "" && !hasPatch {
		return fmt.Errorf("brain edit: nothing to do — set --body, --append, or a frontmatter flag")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("brain edit: %w", err)
	}
	v, err := vault.Open(cfg.Vault)
	if err != nil {
		return fmt.Errorf("brain edit: %w", err)
	}
	if filepath.IsAbs(rel) || strings.Contains(filepath.Clean(rel), "..") {
		return fmt.Errorf("brain edit: path must be vault-relative: %s", rel)
	}
	full := filepath.Join(cfg.Vault, filepath.Clean(rel))

	opts := vault.EditOptions{
		Body:   body,
		Append: appendBody,
	}
	if hasPatch {
		opts.FrontmatterPatch = patch
	}
	if err := v.Edit(full, opts); err != nil {
		return fmt.Errorf("brain edit: %w", err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), rel)
	return nil
}

// readMaybeStdin returns the flag value, or stdin-contents if the value is "-".
func readMaybeStdin(stdin io.Reader, val string) (string, error) {
	if val != "-" {
		return val, nil
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
