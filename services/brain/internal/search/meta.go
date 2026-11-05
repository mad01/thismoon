package search

import (
	"os"
	"path/filepath"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
	"github.com/mad01/thismoon/services/brain/internal/markdown"
)

// NoteMeta is what a hit says about its note's standing: the evidence tier,
// the stored status, review_by, and whether review_by has passed.
type NoteMeta struct {
	Evidence string
	Status   string
	ReviewBy string
	Stale    bool
}

// MetaReader reads hit metadata from the notes on disk, so a hit reports a
// note's status as it is now, not as an index saw it. Each note is read at
// most once. Metadata is best effort: a note that cannot be read or parsed
// has none.
type MetaReader struct {
	roots map[string]string
	now   time.Time
	cache map[string]NoteMeta
}

// NewMetaReader reads notes under roots (vault name to root) and judges
// staleness against now.
func NewMetaReader(roots map[string]string, now time.Time) *MetaReader {
	return &MetaReader{roots: roots, now: now, cache: map[string]NoteMeta{}}
}

// Meta returns the metadata of the note at the vault-relative path rel.
func (r *MetaReader) Meta(vaultName, rel string) NoteMeta {
	key := vaultName + ":" + rel
	if m, ok := r.cache[key]; ok {
		return m
	}
	m := readMeta(r.roots[vaultName], rel, r.now)
	r.cache[key] = m
	return m
}

func readMeta(root, rel string, now time.Time) NoteMeta {
	if root == "" {
		return NoteMeta{}
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return NoteMeta{}
	}
	fm, _, err := markdown.ParseFrontmatter(data)
	if err != nil {
		return NoteMeta{}
	}
	f := lifecycle.Read(fm)
	return NoteMeta{
		Evidence: markdown.EvidenceOf(fm),
		Status:   fm.Status,
		ReviewBy: f.ReviewBy,
		Stale:    lifecycle.PastReview(f, now),
	}
}

// Hidden reports whether default search leaves out a note with this
// status: a revoked one, unless includeRevoked is set or the caller asked
// for that status by name.
func Hidden(status string, includeRevoked bool, wantStatus string) bool {
	if includeRevoked || !lifecycle.Revoked(status) {
		return false
	}
	return lifecycle.Normalize(wantStatus) != lifecycle.Normalize(status)
}
