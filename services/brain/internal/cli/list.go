package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/markdown"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

var (
	listTagFlag    string
	listStatusFlag string
	listTypeFlag   string
	listLimitFlag  int
)

var listCmd = &cobra.Command{
	Use:   "list <tier>",
	Short: "List notes in a tier with optional filters",
	Long: `List notes in a single vault tier (inbox, notes, outputs, or MOCs).

Prints a simple table: updated | tier/path | title. Optionally filter by
frontmatter tag, status, or type.`,
	Args: cobra.ExactArgs(1),
	RunE: runListCmd,
}

func init() {
	listCmd.Flags().StringVar(&listTagFlag, "tag", "", "filter by frontmatter tag")
	listCmd.Flags().StringVar(&listStatusFlag, "status", "", "filter by frontmatter status")
	listCmd.Flags().StringVar(&listTypeFlag, "type", "", "filter by frontmatter type")
	listCmd.Flags().IntVar(&listLimitFlag, "limit", 100, "maximum number of notes to print")
	rootCmd.AddCommand(listCmd)
}

type listRow struct {
	Path    string
	Title   string
	Updated string
}

func runListCmd(cmd *cobra.Command, args []string) error {
	tier, err := vault.ParseTier(args[0])
	if err != nil {
		return fmt.Errorf("brain list: %w", err)
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("brain list: %w", err)
	}

	dir := filepath.Join(cfg.Vault, tier.Dir())
	var rows []listRow
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".obsidian" || name == ".trash" || name == "attachments" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		fm, _, err := markdown.ParseFrontmatter(data)
		if err != nil {
			return nil
		}
		if listTagFlag != "" && !fm.HasTag(listTagFlag) {
			return nil
		}
		if listStatusFlag != "" && !strings.EqualFold(fm.Status, listStatusFlag) {
			return nil
		}
		if listTypeFlag != "" && !strings.EqualFold(fm.Type, listTypeFlag) {
			return nil
		}
		rel, err := filepath.Rel(cfg.Vault, path)
		if err != nil {
			return nil
		}
		rows = append(rows, listRow{
			Path:    filepath.ToSlash(rel),
			Title:   fm.Title,
			Updated: fm.Updated,
		})
		return nil
	})
	if walkErr != nil {
		if !os.IsNotExist(walkErr) {
			return fmt.Errorf("brain list: %w", walkErr)
		}
	}

	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Updated != rows[j].Updated {
			return rows[i].Updated > rows[j].Updated
		}
		return rows[i].Path < rows[j].Path
	})
	if len(rows) > listLimitFlag {
		rows = rows[:listLimitFlag]
	}

	w := cmd.OutOrStdout()
	for _, r := range rows {
		upd := r.Updated
		if upd == "" {
			upd = "-"
		}
		fmt.Fprintf(w, "%s  %s  %s\n", upd, r.Path, r.Title)
	}
	return nil
}
