package vault

import "github.com/mad01/thismoon/services/brain/internal/markdown"

// The types in this file are the JSON shapes the CLI (-o json) and the MCP
// tools share, so the two surfaces return identical documents.

// FrontmatterView is the JSON view of a note's frontmatter returned by read,
// peek and their MCP tools.
//
// Evidence is the computed trust tier (A/B/C/D/mixed/empty), read from the
// explicit frontmatter.evidence field or parsed from an "evidence/*" tag
// when the field is absent. Callers route trust decisions on this value.
type FrontmatterView struct {
	Title    string   `json:"title,omitempty"`
	Aliases  []string `json:"aliases,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Type     string   `json:"type,omitempty"`
	Status   string   `json:"status,omitempty"`
	Source   string   `json:"source,omitempty"`
	Evidence string   `json:"evidence,omitempty" jsonschema:"trust tier: A (replicated RCT / meta-analysis), B (single well-designed study), C (preliminary / survey / mechanism-plausible), D (refuted / apocryphal), mixed, or empty for non-empirical notes"`
	Created  string   `json:"created,omitempty"`
	Updated  string   `json:"updated,omitempty"`
	ID       string   `json:"id,omitempty"`
	// Extra carries the frontmatter keys brain does not model.
	Extra map[string]any `json:"extra,omitempty"    jsonschema:"frontmatter keys brain does not model (owner, sources, ...), with their values"`
}

func viewOf(fm markdown.Frontmatter) (FrontmatterView, error) {
	extra, err := fm.ExtraValues()
	if err != nil {
		return FrontmatterView{}, err
	}
	return FrontmatterView{
		Title:    fm.Title,
		Aliases:  fm.Aliases,
		Tags:     fm.Tags,
		Type:     fm.Type,
		Status:   fm.Status,
		Source:   fm.Source,
		Evidence: markdown.EvidenceOf(fm),
		Created:  fm.Created,
		Updated:  fm.Updated,
		ID:       fm.ID,
		Extra:    extra,
	}, nil
}

// PathResult is the JSON result of write and edit.
type PathResult struct {
	Vault string `json:"vault"              jsonschema:"name of the vault holding the note"`
	Path  string `json:"path"               jsonschema:"vault-relative path of the note"`
	SHA   string `json:"sha,omitempty"      jsonschema:"edit only: sha256 of the bytes written, the if_match for a next edit"`
	// Warnings name the links in a written body that cite a note that is
	// not validated.
	Warnings []string `json:"warnings,omitempty" jsonschema:"write and edit: each link in the body to a note that is not validated, or is past its review_by"`
}

// PromoteReport is the JSON view of a PromoteResult, with vault-relative paths.
type PromoteReport struct {
	Vault            string   `json:"vault"`
	OldPath          string   `json:"old_path"`
	NewPath          string   `json:"new_path"`
	BacklinkRewrites []string `json:"backlink_rewrites"`
	MOCAppended      string   `json:"moc_appended,omitempty"`
}

// Report converts a PromoteResult to its vault-relative JSON view.
func (v *Vault) Report(res PromoteResult) PromoteReport {
	rewrites := make([]string, 0, len(res.BacklinkRewrites))
	for _, r := range res.BacklinkRewrites {
		rewrites = append(rewrites, v.Rel(r))
	}
	moc := ""
	if res.MOCAppended != "" {
		moc = v.Rel(res.MOCAppended)
	}
	return PromoteReport{
		Vault:            v.Name,
		OldPath:          v.Rel(res.OldPath),
		NewPath:          v.Rel(res.NewPath),
		BacklinkRewrites: rewrites,
		MOCAppended:      moc,
	}
}
