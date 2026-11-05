package library

import (
	"context"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/services/brain/internal/chunk"
	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
	"github.com/mad01/thismoon/services/brain/internal/markdown"
	"github.com/mad01/thismoon/services/brain/internal/semantic"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// DuplicateSimilarity is the cosine similarity between a new note's first
// chunk and an indexed chunk at or above which the write treats the indexed
// note as a likely duplicate. It is set high on purpose: refusing a note
// that is new costs its writer more than letting a near copy through.
const DuplicateSimilarity = 0.92

// duplicateNeighbours is how many chunks each vault's semantic index is
// asked for when looking for duplicates.
const duplicateNeighbours = 5

// Kinds of duplicate match.
const (
	MatchTitle    = "title"
	MatchAlias    = "alias"
	MatchSemantic = "semantic"
)

// Duplicate is an existing note a new note likely duplicates.
type Duplicate struct {
	Ref   vault.Ref
	Title string
	// Match is MatchTitle, MatchAlias or MatchSemantic.
	Match string
	// Score is the cosine similarity of a semantic match.
	Score float32
}

func (d Duplicate) String() string {
	switch d.Match {
	case MatchSemantic:
		return fmt.Sprintf("%s (similarity %.2f)", d.Ref, d.Score)
	case MatchAlias:
		return fmt.Sprintf("%s (alias of %q)", d.Ref, d.Title)
	}
	return fmt.Sprintf("%s (same title)", d.Ref)
}

// Write creates the note opts describes in v after the duplicate check:
// a note in v or in a shared vault with the same normalized title or alias,
// or, when a semantic index is there to ask, one whose content is at least
// DuplicateSimilarity alike, refuses the write with
// vault.ErrPossibleDuplicate. distinctFrom names notes the writer says the
// new note is distinct from: each must exist, is stored in distinct_from
// (as id:<ulid> when the note has an id) and is no longer a candidate. The
// warnings say when the semantic part of the check could not run.
func (l *Library) Write(
	ctx context.Context,
	v *vault.Vault,
	opts vault.WriteOptions,
	distinctFrom []string,
) (string, []string, error) {
	if v.Shared {
		path, err := v.Write(opts) // refused as read-only, before any lookup
		return path, nil, err
	}
	distinct, err := l.distinctRefs(distinctFrom)
	if err != nil {
		return "", nil, err
	}
	if len(distinct) > 0 {
		opts.Set = append(opts.Set, distinctField(distinct))
	}
	draft, err := v.Draft(opts)
	if err != nil {
		return "", nil, err
	}
	dups, warnings, err := l.Duplicates(ctx, v, draft)
	if err != nil {
		return "", nil, err
	}
	if dups = withoutDistinct(dups, distinct); len(dups) > 0 {
		return "", warnings, duplicateError(draft.Frontmatter.Title, dups)
	}
	path, err := v.Create(draft)
	return path, warnings, err
}

// distinctNote is one resolved distinct_from ref: the note and the ref
// stored for it.
type distinctNote struct {
	ref    vault.Ref
	stored string
}

// distinctRefs resolves each ref across every vault.
func (l *Library) distinctRefs(refs []string) ([]distinctNote, error) {
	out := make([]distinctNote, 0, len(refs))
	for _, r := range refs {
		ref, err := l.set.Locate(strings.TrimSpace(r))
		if err != nil {
			return nil, fmt.Errorf("distinct_from %s: %w", r, err)
		}
		stored := ref.String()
		if fm, _, err := ref.Vault.ReadFrontmatter(ref.Abs()); err == nil && fm.ID != "" {
			stored = "id:" + fm.ID
		}
		out = append(out, distinctNote{ref: ref, stored: stored})
	}
	return out, nil
}

func distinctField(notes []distinctNote) markdown.Field {
	seq := &yaml.Node{Kind: yaml.SequenceNode}
	for _, n := range notes {
		seq.Content = append(seq.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: n.stored})
	}
	return markdown.Field{Key: lifecycle.KeyDistinctFrom, Value: seq}
}

func withoutDistinct(dups []Duplicate, distinct []distinctNote) []Duplicate {
	named := map[string]bool{}
	for _, d := range distinct {
		named[d.ref.String()] = true
	}
	kept := dups[:0]
	for _, d := range dups {
		if !named[d.Ref.String()] {
			kept = append(kept, d)
		}
	}
	return kept
}

func duplicateError(title string, dups []Duplicate) error {
	names := make([]string, 0, len(dups))
	for _, d := range dups {
		names = append(names, d.String())
	}
	return fmt.Errorf(
		"%w: %q looks like %s; nothing was written (%s, or name each in distinct_from, "+
			"--distinct-from on the CLI, to write it anyway)",
		vault.ErrPossibleDuplicate, title, strings.Join(names, ", "), extendHint(dups),
	)
}

// extendHint says how to extend the first candidate a direct edit can
// reach, or that a shared candidate changes through a pull request.
func extendHint(dups []Duplicate) string {
	for _, d := range dups {
		if !d.Ref.Vault.Shared {
			return "extend it with: brain edit " + d.Ref.String()
		}
	}
	return fmt.Sprintf("%s is in a shared vault; extend it through a pull request on its repo",
		dups[0].Ref)
}

// Duplicates returns the notes in v and in the shared vaults that the draft
// likely duplicates: the same normalized title or alias first, then
// semantic matches, each note once.
func (l *Library) Duplicates(
	ctx context.Context,
	v *vault.Vault,
	draft vault.Draft,
) ([]Duplicate, []string, error) {
	scope := l.duplicateScope(v)
	titles, err := vault.NewSet(scope...).SameTitle(draft.Frontmatter.Title)
	if err != nil {
		return nil, nil, err
	}
	var out []Duplicate
	seen := map[string]bool{}
	for _, m := range titles {
		match := MatchTitle
		if m.Alias != "" {
			match = MatchAlias
		}
		out = append(out, Duplicate{Ref: m.Ref, Title: m.Title, Match: match})
		seen[m.Ref.String()] = true
	}
	similar, warnings := l.similarNotes(ctx, scope, draft)
	for _, d := range similar {
		if !seen[d.Ref.String()] {
			out = append(out, d)
			seen[d.Ref.String()] = true
		}
	}
	return out, warnings, nil
}

// duplicateScope is the vaults a new note in v is checked against: v, then
// every shared vault.
func (l *Library) duplicateScope(v *vault.Vault) []*vault.Vault {
	scope := []*vault.Vault{v}
	for _, o := range l.set.Vaults() {
		if o.Shared && o != v {
			scope = append(scope, o)
		}
	}
	return scope
}

// similarNotes asks each vault's semantic index, when the layer is on and
// the vault has one, for chunks like the draft's first chunk. The index is
// used as it is, never refreshed: a write does not wait for embeddings. A
// check that cannot run (the embedder is down) is a warning, not a refusal.
func (l *Library) similarNotes(
	ctx context.Context,
	scope []*vault.Vault,
	draft vault.Draft,
) ([]Duplicate, []string) {
	if !l.cfg.Semantic.Enabled {
		return nil, nil
	}
	text := firstChunk(draft, l.chunkOptions())
	if text == "" {
		return nil, nil
	}
	var (
		out    []Duplicate
		probed bool
	)
	for _, v := range scope {
		idx, ok := l.existingSemantic(v)
		if !ok {
			continue
		}
		if !probed {
			if err := l.Health(ctx); err != nil {
				return nil, []string{"the semantic duplicate check did not run: " + err.Error()}
			}
			probed = true
		}
		hits, err := idx.Query(ctx, text, semantic.QueryOptions{K: duplicateNeighbours})
		if err != nil {
			return out, []string{"the semantic duplicate check did not run: " + err.Error()}
		}
		out = append(out, similarHits(v, hits)...)
	}
	return out, nil
}

// existingSemantic opens the vault's semantic index when it exists and
// holds chunks.
func (l *Library) existingSemantic(v *vault.Vault) (*semantic.Index, bool) {
	dir, err := l.cfg.Semantic.ResolvedIndexDir(v.Name)
	if err != nil {
		return nil, false
	}
	if _, err := os.Stat(dir); err != nil {
		return nil, false
	}
	idx, err := l.openSemantic(v)
	if err != nil || idx.Count() == 0 {
		return nil, false
	}
	return idx, true
}

// similarHits keeps the hits at or above DuplicateSimilarity whose note
// still exists and is not revoked, one per note.
func similarHits(v *vault.Vault, hits []semantic.Hit) []Duplicate {
	var out []Duplicate
	seen := map[string]bool{}
	for _, h := range hits {
		if h.Score < DuplicateSimilarity || seen[h.Path] {
			continue
		}
		seen[h.Path] = true
		ref := vault.Ref{Vault: v, Path: h.Path}
		fm, _, err := v.ReadFrontmatter(ref.Abs())
		if err != nil || lifecycle.Revoked(fm.Status) {
			continue
		}
		out = append(
			out,
			Duplicate{Ref: ref, Title: fm.Title, Match: MatchSemantic, Score: h.Score},
		)
	}
	return out
}

// firstChunk returns the text of the draft's first chunk, the way the
// semantic index would embed it, or "" for a note without a body.
func firstChunk(draft vault.Draft, opts chunk.Options) string {
	chunks, err := chunk.Split("draft.md", "", draft.Content(), opts)
	if err != nil || len(chunks) == 0 {
		return ""
	}
	return chunks[0].Body
}
