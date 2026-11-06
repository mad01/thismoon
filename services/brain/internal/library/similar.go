package library

import (
	"context"
	"os"

	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// SimilarSimilarity is the cosine similarity at or above which brain
// similar lists a note. It sits below DuplicateSimilarity on purpose: a
// similar note is a warning for a reviewer and never refuses anything, so
// it can afford to cast wider.
const SimilarSimilarity = 0.85

// similarNeighbours is how many chunks each vault's semantic index is
// asked for. It is wider than the duplicate check's, since the note's own
// chunks come back among them.
const similarNeighbours = 10

// SimilarNote is a note brain similar finds alike to another.
type SimilarNote struct {
	Vault string `json:"vault"`
	Path  string `json:"path"`
	Title string `json:"title,omitempty"`
	// Match is title, alias or semantic.
	Match string `json:"match"`
	// Score is the cosine similarity of a semantic match.
	Score float32 `json:"score,omitempty"`
}

// SimilarReport is the JSON result of brain similar.
type SimilarReport struct {
	Vault   string        `json:"vault"`
	Path    string        `json:"path"`
	Similar []SimilarNote `json:"similar"`
	// Warnings say when the semantic part could not run.
	Warnings []string `json:"warnings,omitempty"`
}

// Similar lists the notes across every configured vault that look like the
// note at ref: the same normalized title or alias as the note's title or
// one of its aliases, then, when a semantic index exists, notes whose
// content is at least SimilarSimilarity alike to the note's first chunk.
// The note itself, and a copy of it with the same id in another vault (a
// promotion in flight), are left out, as are revoked notes.
func (l *Library) Similar(ctx context.Context, ref vault.Ref) (SimilarReport, error) {
	data, err := os.ReadFile(ref.Abs())
	if err != nil {
		return SimilarReport{}, err
	}
	fm, _, err := ref.Vault.ReadFrontmatter(ref.Abs())
	if err != nil {
		return SimilarReport{}, err
	}
	report := SimilarReport{Vault: ref.Vault.Name, Path: ref.Path, Similar: []SimilarNote{}}
	self := func(r vault.Ref) bool { return r.String() == ref.String() || sameID(r, fm.ID) }
	seen := map[string]bool{}
	add := func(d Duplicate) {
		if key := d.Ref.String(); !seen[key] && !self(d.Ref) {
			seen[key] = true
			report.Similar = append(report.Similar, SimilarNote{
				Vault: d.Ref.Vault.Name, Path: d.Ref.Path, Title: d.Title, Match: d.Match, Score: d.Score,
			})
		}
	}
	for _, title := range append([]string{fm.Title}, fm.Aliases...) {
		matches, err := l.set.SameTitle(title)
		if err != nil {
			return SimilarReport{}, err
		}
		for _, m := range matches {
			match := MatchTitle
			if m.Alias != "" {
				match = MatchAlias
			}
			add(Duplicate{Ref: m.Ref, Title: m.Title, Match: match})
		}
	}
	similar, warnings := l.similarNotes(ctx, l.set.Vaults(),
		firstChunk(data, l.chunkOptions()), SimilarSimilarity, similarNeighbours)
	for _, d := range similar {
		add(d)
	}
	report.Warnings = warnings
	return report, nil
}

// sameID reports whether the note at r carries id.
func sameID(r vault.Ref, id string) bool {
	if id == "" {
		return false
	}
	fm, _, err := r.Vault.ReadFrontmatter(r.Abs())
	return err == nil && fm.ID == id
}
