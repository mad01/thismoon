package query

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/mad01/thismoon/services/moondb/internal/patch"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// Content attribution. Why scans at most contentScanLimit edits of the
// path and keeps maxCandidates. A candidate scores 1 when its hunks added
// the line and createWeight when only its created file holds it, times
// rankDecay for every candidate ranked above it.
const (
	contentScanLimit = 500
	maxCandidates    = 5
	rankDecay        = 0.8
	createWeight     = 0.9
)

// whyByContent answers by the line's text: the edits of the path whose
// added lines equal it once normalized, newest first. Repo spans every
// checkout of the repository; without it the worktree narrows the scan.
func (s *Service) whyByContent(ctx context.Context, req WhyRequest, res *WhyResult) error {
	want := normalize(res.Text)
	if trivial(want) {
		return nil
	}
	q := store.EditQuery{Path: req.Path, Limit: contentScanLimit}
	if req.Repo != "" {
		q.Repo = req.Repo
	} else {
		q.Worktree = req.Worktree
	}
	edits, err := s.Store.Edits(ctx, q)
	if err != nil {
		return fmt.Errorf("query: edits of %s: %w", req.Path, err)
	}
	cands, err := s.candidates(ctx, edits, want)
	if err != nil || len(cands) == 0 {
		return err
	}
	res.Method, res.Candidates = MethodContent, cands
	return s.attach(ctx, res, cands[0].Edit, req.Path)
}

// candidates scores the edits that wrote the line, in the order given.
func (s *Service) candidates(
	ctx context.Context,
	edits []store.Edit,
	want string,
) ([]Candidate, error) {
	var out []Candidate
	for _, e := range edits {
		m, err := s.match(ctx, e, want)
		if err != nil {
			return nil, err
		}
		if m.count == 0 {
			continue
		}
		score := m.weight() * math.Pow(rankDecay, float64(len(out)))
		out = append(out, Candidate{Edit: e, Score: score, Reason: m.reason()})
		if len(out) == maxCandidates {
			break
		}
	}
	return out, nil
}

// lineMatch is how an edit wrote a line: count times, through its hunks,
// or as part of a file it created.
type lineMatch struct {
	count   int
	created bool
}

func (m lineMatch) weight() float64 {
	if m.created {
		return createWeight
	}
	return 1
}

func (m lineMatch) reason() string {
	what := "added the line"
	if m.created {
		what = "created the file with the line"
	}
	if m.count > 1 {
		return fmt.Sprintf("%s (%d times)", what, m.count)
	}
	return what
}

// match counts the lines e wrote that equal want: the lines its hunks
// added, or, for a create whose hunks add none of them (a Write reports
// no hunks for a new file), the lines of its after-content. A patch that
// does not parse counts as no hunks.
func (s *Service) match(ctx context.Context, e store.Edit, want string) (lineMatch, error) {
	raw, err := s.chunk(ctx, e.PatchChunk)
	if err != nil {
		return lineMatch{}, err
	}
	if raw != nil {
		if hunks, err := patch.Parse(raw); err == nil {
			if n := countEqual(patch.AddedLines(hunks), want); n > 0 {
				return lineMatch{count: n}, nil
			}
		}
	}
	if e.Kind != store.EditCreate {
		return lineMatch{}, nil
	}
	after, err := s.chunk(ctx, e.AfterChunk)
	if err != nil || after == nil {
		return lineMatch{}, err
	}
	return lineMatch{
		count:   countEqual(strings.Split(string(after), "\n"), want),
		created: true,
	}, nil
}

// chunk loads a chunk; nil when key is "" or the store does not have it.
func (s *Service) chunk(ctx context.Context, key string) ([]byte, error) {
	if key == "" {
		return nil, nil
	}
	data, err := s.Store.Chunks().Get(ctx, key)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query: chunk %s: %w", key, err)
	}
	return data, nil
}

func countEqual(lines []string, want string) int {
	n := 0
	for _, l := range lines {
		if normalize(l) == want {
			n++
		}
	}
	return n
}

// lineAt reads the requested line from the file in the worktree; "" when
// the file or the line is not there.
func lineAt(req WhyRequest) string {
	if req.Line < 1 || !onDisk(req) {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(req.Worktree, filepath.FromSlash(req.Path)))
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	if req.Line > len(lines) {
		return ""
	}
	return strings.TrimSuffix(lines[req.Line-1], "\r")
}

// onDisk reports whether the request names a regular file inside its
// worktree, the only files git and lineAt open.
func onDisk(req WhyRequest) bool {
	rel := filepath.FromSlash(req.Path)
	if req.Worktree == "" || !filepath.IsLocal(rel) {
		return false
	}
	info, err := os.Stat(filepath.Join(req.Worktree, rel))
	return err == nil && info.Mode().IsRegular()
}
