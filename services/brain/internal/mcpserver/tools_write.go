package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/brain/internal/markdown"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

type writeInput struct {
	Tier     string   `json:"tier"               jsonschema:"target tier: inbox, notes, outputs, or MOCs"`
	Title    string   `json:"title"              jsonschema:"human-readable note title; becomes frontmatter.title and the filename slug"`
	Body     string   `json:"body,omitempty"     jsonschema:"markdown body; can be empty for a stub note"`
	Tags     []string `json:"tags,omitempty"     jsonschema:"frontmatter tags list"`
	Aliases  []string `json:"aliases,omitempty"  jsonschema:"frontmatter aliases — alternate titles that backlinks should resolve to"`
	Type     string   `json:"type,omitempty"     jsonschema:"frontmatter type (note | source | moc | output); defaults based on tier"`
	Status   string   `json:"status,omitempty"   jsonschema:"frontmatter status (raw | validated | archived); defaults based on tier"`
	Source   string   `json:"source,omitempty"   jsonschema:"optional source URL"`
	Evidence string   `json:"evidence,omitempty" jsonschema:"trust tier for empirical notes: A (replicated RCT / meta-analysis), B (single well-designed study), C (preliminary / survey / mechanism-plausible), D (refuted / apocryphal), or mixed. Omit for non-empirical notes."`
	Filename string   `json:"filename,omitempty" jsonschema:"override filename (without .md); defaults to slug(title)"`
}

type editInput struct {
	Path             string         `json:"path"                        jsonschema:"vault-relative note path"`
	Body             string         `json:"body,omitempty"              jsonschema:"replace the body entirely with this string; mutually exclusive with append"`
	Append           string         `json:"append,omitempty"            jsonschema:"append this string to the existing body; mutually exclusive with body"`
	FrontmatterPatch *frontmatterIn `json:"frontmatter_patch,omitempty" jsonschema:"partial frontmatter to merge into the existing one (non-empty fields win)"`
}

type frontmatterIn struct {
	Title    string   `json:"title,omitempty"`
	Aliases  []string `json:"aliases,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Type     string   `json:"type,omitempty"`
	Status   string   `json:"status,omitempty"`
	Source   string   `json:"source,omitempty"`
	Evidence string   `json:"evidence,omitempty" jsonschema:"trust tier (A/B/C/D/mixed); set to empty string to clear"`
	ID       string   `json:"id,omitempty"`
}

type promoteInput struct {
	Path       string `json:"path"                  jsonschema:"vault-relative inbox note path"`
	NewTitle   string `json:"new_title,omitempty"   jsonschema:"optional new title (and re-slug) for the promoted note"`
	TargetTier string `json:"target_tier,omitempty" jsonschema:"destination tier; defaults to notes"`
	MOC        string `json:"moc,omitempty"         jsonschema:"optional topical MOC path (vault-relative or absolute) to append a bullet to. Must be a semantic MOC; batch-provenance indexes (research-index-* / kickoff-* filenames, or tags including meta/index) are rejected."`
}

func registerWriteTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "brain_write",
		Description: "Create a new note in a tier (inbox, notes, outputs, or MOCs) with frontmatter (title, tags, aliases, type, status, source). " +
			"Use to capture new material — 'save this as an inbox note', 'write a validated note on X', 'create an output'. " +
			"Filename defaults to slug(title); outputs are prefixed with YYYY-MM-DD. Errors if a note with the same filename already exists.",
	}, handleWrite)

	mcp.AddTool(s, &mcp.Tool{
		Name: "brain_edit",
		Description: "Modify an existing note — replace body, append to body, and/or merge a frontmatter patch. " +
			"Exactly one of body/append is allowed; frontmatter_patch is optional. Updates frontmatter.updated. " +
			"Use to extend a note, update status (e.g. raw → validated), add tags, or rewrite the body.",
	}, handleEdit)

	mcp.AddTool(s, &mcp.Tool{
		Name: "brain_promote",
		Description: "Promote an inbox note into notes/ (or another tier): atomic move, status → validated, backlinks across the vault rewritten to the new filename, optional bullet appended to a MOC. " +
			"Use when an inbox capture is ready to become a first-class note. " +
			"The moc target must be a topical MOC (semantic grouping); batch-provenance indexes (filenames matching research-index-* / kickoff-*, or tagged meta/index) are rejected to keep the graph centered on real semantic groupings.",
	}, handlePromote)
}

func handleWrite(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in writeInput,
) (*mcp.CallToolResult, vault.PathResult, error) {
	if strings.TrimSpace(in.Tier) == "" {
		return nil, vault.PathResult{}, fmt.Errorf("brain_write: tier is required")
	}
	if strings.TrimSpace(in.Title) == "" {
		return nil, vault.PathResult{}, fmt.Errorf("brain_write: title is required")
	}
	tier, err := vault.ParseTier(in.Tier)
	if err != nil {
		return nil, vault.PathResult{}, fmt.Errorf("brain_write: %w", err)
	}

	_, v, err := loadVault()
	if err != nil {
		return nil, vault.PathResult{}, fmt.Errorf("brain_write: %w", err)
	}

	abs, err := v.Write(vault.WriteOptions{
		Tier:     tier,
		Title:    in.Title,
		Body:     in.Body,
		Tags:     in.Tags,
		Aliases:  in.Aliases,
		Type:     in.Type,
		Status:   in.Status,
		Source:   in.Source,
		Evidence: in.Evidence,
		Filename: in.Filename,
	})
	if err != nil {
		return nil, vault.PathResult{}, fmt.Errorf("brain_write: %w", err)
	}
	return nil, vault.PathResult{Path: v.Rel(abs)}, nil
}

func handleEdit(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in editInput,
) (*mcp.CallToolResult, vault.PathResult, error) {
	if strings.TrimSpace(in.Path) == "" {
		return nil, vault.PathResult{}, fmt.Errorf("brain_edit: path is required")
	}
	if in.Body != "" && in.Append != "" {
		return nil, vault.PathResult{}, fmt.Errorf(
			"brain_edit: body and append are mutually exclusive",
		)
	}
	if in.Body == "" && in.Append == "" && in.FrontmatterPatch == nil {
		return nil, vault.PathResult{}, fmt.Errorf(
			"brain_edit: one of body, append, or frontmatter_patch must be set",
		)
	}

	_, v, err := loadVault()
	if err != nil {
		return nil, vault.PathResult{}, fmt.Errorf("brain_edit: %w", err)
	}
	abs, err := v.Resolve(in.Path)
	if err != nil {
		return nil, vault.PathResult{}, fmt.Errorf("brain_edit: %w", err)
	}

	opts := vault.EditOptions{
		Body:   in.Body,
		Append: in.Append,
	}
	if in.FrontmatterPatch != nil {
		patch := markdown.Frontmatter{
			Title:    in.FrontmatterPatch.Title,
			Aliases:  in.FrontmatterPatch.Aliases,
			Tags:     in.FrontmatterPatch.Tags,
			Type:     in.FrontmatterPatch.Type,
			Status:   in.FrontmatterPatch.Status,
			Source:   in.FrontmatterPatch.Source,
			Evidence: in.FrontmatterPatch.Evidence,
			ID:       in.FrontmatterPatch.ID,
		}
		opts.FrontmatterPatch = &patch
	}

	if err := v.Edit(abs, opts); err != nil {
		return nil, vault.PathResult{}, fmt.Errorf("brain_edit: %w", err)
	}
	return nil, vault.PathResult{Path: in.Path}, nil
}

func handlePromote(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in promoteInput,
) (*mcp.CallToolResult, vault.PromoteReport, error) {
	if strings.TrimSpace(in.Path) == "" {
		return nil, vault.PromoteReport{}, fmt.Errorf("brain_promote: path is required")
	}

	_, v, err := loadVault()
	if err != nil {
		return nil, vault.PromoteReport{}, fmt.Errorf("brain_promote: %w", err)
	}
	abs, err := v.Resolve(in.Path)
	if err != nil {
		return nil, vault.PromoteReport{}, fmt.Errorf("brain_promote: %w", err)
	}

	opts := vault.PromoteOptions{
		NewTitle: in.NewTitle,
		MOC:      in.MOC,
	}
	if in.TargetTier != "" {
		t, err := vault.ParseTier(in.TargetTier)
		if err != nil {
			return nil, vault.PromoteReport{}, fmt.Errorf("brain_promote: %w", err)
		}
		opts.TargetTier = t
	}

	res, err := v.Promote(abs, opts)
	if err != nil {
		return nil, vault.PromoteReport{}, fmt.Errorf("brain_promote: %w", err)
	}

	return nil, v.Report(res), nil
}
