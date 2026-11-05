package mcpserver

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/markdown"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

type listInput struct {
	Tier   string `json:"tier"             jsonschema:"vault tier to list: inbox, notes, outputs, or MOCs"`
	Tag    string `json:"tag,omitempty"    jsonschema:"filter by frontmatter tag"`
	Status string `json:"status,omitempty" jsonschema:"filter by frontmatter status"`
	Type   string `json:"type,omitempty"   jsonschema:"filter by frontmatter type"`
	Limit  int    `json:"limit,omitempty"  jsonschema:"maximum number of notes to return (default 100)"`
}

type listedNote struct {
	Path     string   `json:"path"               jsonschema:"vault-relative note path"`
	Title    string   `json:"title,omitempty"`
	Status   string   `json:"status,omitempty"`
	Type     string   `json:"type,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Aliases  []string `json:"aliases,omitempty"`
	Evidence string   `json:"evidence,omitempty" jsonschema:"trust tier (A/B/C/D/mixed) if set"`
	Updated  string   `json:"updated,omitempty"`
}

type listOutput struct {
	Tier  string       `json:"tier"`
	Total int          `json:"total"`
	Notes []listedNote `json:"notes"`
}

func registerListTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "brain_list_tier",
		Description: "List notes in a single vault tier (inbox, notes, outputs, or MOCs). " +
			"Use to survey what's in a tier — 'what's in my inbox', 'list validated notes tagged kubernetes', 'show MOCs'. " +
			"Optionally filter by tag, status, or type. Returns notes sorted by frontmatter.updated descending, then path. Default limit 100.",
	}, handleListTier)
}

func handleListTier(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in listInput,
) (*mcp.CallToolResult, listOutput, error) {
	if strings.TrimSpace(in.Tier) == "" {
		return nil, listOutput{}, fmt.Errorf("brain_list_tier: tier is required")
	}
	tier, err := vault.ParseTier(in.Tier)
	if err != nil {
		return nil, listOutput{}, fmt.Errorf("brain_list_tier: %w", err)
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 100
	}

	cfg, _, err := loadVault()
	if err != nil {
		return nil, listOutput{}, fmt.Errorf("brain_list_tier: %w", err)
	}
	notes, err := collectTier(cfg, tier, in.Tag, in.Status, in.Type)
	if err != nil {
		return nil, listOutput{}, fmt.Errorf("brain_list_tier: %w", err)
	}
	sortNotes(notes)
	total := len(notes)
	if len(notes) > limit {
		notes = notes[:limit]
	}
	return nil, listOutput{
		Tier:  tier.Dir(),
		Total: total,
		Notes: notes,
	}, nil
}

func collectTier(
	cfg *config.Config,
	tier vault.Tier,
	tag, status, typeFilter string,
) ([]listedNote, error) {
	dir := filepath.Join(cfg.Vault, tier.Dir())
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}

	var out []listedNote
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
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
		if tag != "" && !fm.HasTag(tag) {
			return nil
		}
		if status != "" && !strings.EqualFold(fm.Status, status) {
			return nil
		}
		if typeFilter != "" && !strings.EqualFold(fm.Type, typeFilter) {
			return nil
		}
		rel, err := filepath.Rel(cfg.Vault, path)
		if err != nil {
			return nil
		}
		out = append(out, listedNote{
			Path:     filepath.ToSlash(rel),
			Title:    fm.Title,
			Status:   fm.Status,
			Type:     fm.Type,
			Tags:     fm.Tags,
			Aliases:  fm.Aliases,
			Evidence: markdown.EvidenceOf(fm),
			Updated:  fm.Updated,
		})
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return out, nil
}

// sortNotes sorts by Updated desc, then Path asc.
func sortNotes(notes []listedNote) {
	sort.SliceStable(notes, func(i, j int) bool {
		if notes[i].Updated != notes[j].Updated {
			return notes[i].Updated > notes[j].Updated
		}
		return notes[i].Path < notes[j].Path
	})
}
