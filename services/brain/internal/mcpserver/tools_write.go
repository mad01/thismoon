package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/brain/internal/markdown"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

type writeInput struct {
	Tier     string   `json:"tier"                    jsonschema:"target tier: inbox, notes, outputs, or MOCs"`
	Title    string   `json:"title"                   jsonschema:"human-readable note title; becomes frontmatter.title and the filename slug"`
	Body     string   `json:"body,omitempty"          jsonschema:"markdown body; can be empty for a stub note"`
	Tags     []string `json:"tags,omitempty"          jsonschema:"frontmatter tags list"`
	Aliases  []string `json:"aliases,omitempty"       jsonschema:"frontmatter aliases: alternate titles that backlinks should resolve to"`
	Type     string   `json:"type,omitempty"          jsonschema:"frontmatter type (note | source | moc | output); defaults based on tier"`
	Status   string   `json:"status,omitempty"        jsonschema:"frontmatter status (raw | validated | archived); defaults based on tier"`
	Source   string   `json:"source,omitempty"        jsonschema:"optional source URL"`
	Evidence string   `json:"evidence,omitempty"      jsonschema:"trust tier for empirical notes: A (replicated RCT / meta-analysis), B (single well-designed study), C (preliminary / survey / mechanism-plausible), D (refuted / apocryphal), or mixed. Omit for non-empirical notes."`
	Filename string   `json:"filename,omitempty"      jsonschema:"override filename (without .md); defaults to slug(title)"`
	Vault    string   `json:"vault,omitempty"         jsonschema:"configured vault to write to; defaults to the default vault. A shared vault refuses writes."`
	// DistinctFrom names notes the new one is distinct from, though alike.
	DistinctFrom []string `json:"distinct_from,omitempty" jsonschema:"note refs the new note is distinct from although alike; each is stored in distinct_from and no longer refuses the write as a possible duplicate"`
}

type editInput struct {
	Path             string         `json:"path"                        jsonschema:"note ref: vault:path, or a vault-relative path found in exactly one vault"`
	Vault            string         `json:"vault,omitempty"             jsonschema:"look the path up in this configured vault only"`
	Body             string         `json:"body,omitempty"              jsonschema:"replace the body entirely with this string; mutually exclusive with append"`
	Append           string         `json:"append,omitempty"            jsonschema:"append this string to the existing body; mutually exclusive with body"`
	FrontmatterPatch *frontmatterIn `json:"frontmatter_patch,omitempty" jsonschema:"partial frontmatter to merge into the existing one (non-empty fields win)"`
	IfMatch          string         `json:"if_match,omitempty"          jsonschema:"sha from brain_read; the edit fails when the note changed since that read"`
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
	Path       string `json:"path"                  jsonschema:"inbox note ref: vault:path, or a vault-relative path found in exactly one vault"`
	NewTitle   string `json:"new_title,omitempty"   jsonschema:"optional new title (and re-slug) for the promoted note"`
	TargetTier string `json:"target_tier,omitempty" jsonschema:"destination tier; defaults to notes"`
	MOC        string `json:"moc,omitempty"         jsonschema:"optional topical MOC path (vault-relative or absolute) to append a bullet to. Must be a semantic MOC; batch-provenance indexes (research-index-* / kickoff-* filenames, or tags including meta/index) are rejected."`
}

func registerWriteTools(s *mcp.Server, h *handlers) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "brain_write",
		Description: "Create a new note in a tier (inbox, notes, outputs, or MOCs) with frontmatter (title, tags, aliases, type, status, source). " +
			"Use to capture new material: 'save this as an inbox note', 'write a validated note on X', 'create an output'. " +
			"Filename defaults to slug(title); outputs are prefixed with YYYY-MM-DD. Errors if a note with the same filename already exists. " +
			"Refuses a possible duplicate, naming the candidates: a note in the target or a shared vault with the same title or alias, or semantically near-identical content. " +
			"Extend a candidate with brain_edit instead, or list it in distinct_from when the new note is distinct. " +
			"Writes go to the default vault unless vault names another; a shared vault is read-only (its notes change through a pull request).",
	}, h.handleWrite)

	mcp.AddTool(s, &mcp.Tool{
		Name: "brain_edit",
		Description: "Modify an existing note — replace body, append to body, and/or merge a frontmatter patch. " +
			"Exactly one of body/append is allowed; frontmatter_patch is optional. Updates frontmatter.updated. " +
			"Use to extend a note, update status (e.g. raw → validated), add tags, or rewrite the body. " +
			"Pass if_match (the sha brain_read returned) to refuse the edit when the note changed since that read; the result carries the new sha.",
	}, h.handleEdit)

	mcp.AddTool(s, &mcp.Tool{
		Name: "brain_promote",
		Description: "Promote an inbox note into notes/ (or another tier): atomic move, status → validated, backlinks across the vault rewritten to the new filename, optional bullet appended to a MOC. " +
			"Every wikilink that resolves to the note is pointed at its new stem or path (links in code are left alone). " +
			"Use when an inbox capture is ready to become a first-class note. " +
			"The moc target must be a topical MOC (semantic grouping); batch-provenance indexes (filenames matching research-index-* / kickoff-*, or tagged meta/index) are rejected to keep the graph centered on real semantic groupings.",
	}, h.handlePromote)
}

func (h *handlers) handleWrite(
	ctx context.Context,
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

	lib, err := h.open()
	if err != nil {
		return nil, vault.PathResult{}, fmt.Errorf("brain_write: %w", err)
	}
	v, err := lib.WriteTarget(in.Vault)
	if err != nil {
		return nil, vault.PathResult{}, fmt.Errorf("brain_write: %w", err)
	}
	abs, warnings, err := lib.Write(ctx, v, vault.WriteOptions{
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
	}, in.DistinctFrom)
	if err != nil {
		return nil, vault.PathResult{}, fmt.Errorf("brain_write: %w", err)
	}
	res := vault.PathResult{Vault: v.Name, Path: v.Rel(abs)}
	res.Warnings = append(warnings,
		lib.CitationWarnings(vault.Ref{Vault: v, Path: res.Path}, time.Now())...)
	return nil, res, nil
}

func (h *handlers) handleEdit(
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

	lib, ref, err := h.locate(in.Vault, in.Path)
	if err != nil {
		return nil, vault.PathResult{}, fmt.Errorf("brain_edit: %w", err)
	}

	opts := vault.EditOptions{
		Body:    in.Body,
		Append:  in.Append,
		IfMatch: in.IfMatch,
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

	sha, err := ref.Vault.Edit(ref.Abs(), opts)
	if err != nil {
		return nil, vault.PathResult{}, fmt.Errorf("brain_edit: %w", err)
	}
	res := vault.PathResult{Vault: ref.Vault.Name, Path: ref.Path, SHA: sha}
	if in.Body != "" || in.Append != "" {
		res.Warnings = lib.CitationWarnings(ref, time.Now())
	}
	return nil, res, nil
}

func (h *handlers) handlePromote(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in promoteInput,
) (*mcp.CallToolResult, vault.PromoteReport, error) {
	if strings.TrimSpace(in.Path) == "" {
		return nil, vault.PromoteReport{}, fmt.Errorf("brain_promote: path is required")
	}

	_, ref, err := h.locate("", in.Path)
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

	res, err := ref.Vault.Promote(ref.Abs(), opts)
	if err != nil {
		return nil, vault.PromoteReport{}, fmt.Errorf("brain_promote: %w", err)
	}
	return nil, ref.Vault.Report(res), nil
}
