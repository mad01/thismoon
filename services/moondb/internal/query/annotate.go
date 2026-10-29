package query

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mad01/thismoon/kit/gitloc"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// annotateMaxBytes caps the file Annotate reads. A source file past it is
// generated or data, and its per-line answer would be megabytes of JSON.
const annotateMaxBytes = 4 << 20

// AnnotateRequest asks for every line of one file as it is on disk in a
// worktree. Worktree is the absolute checkout root git blame runs in; it
// must hold at least one recorded edit. Repo narrows the content match the
// way WhyRequest.Repo does. Path is worktree-relative.
type AnnotateRequest struct {
	Worktree string `json:"worktree"`
	Repo     string `json:"repo,omitempty"`
	Path     string `json:"path"`
}

// AnnotatedLine is one line of the file and the Edit that explains it,
// found the way Why finds it. Commit is the commit git blame names for the
// line and "" while the line is uncommitted. The ids are "" when Method is
// none; AgentType is "" for the main conversation too.
type AnnotatedLine struct {
	Line      int    `json:"line"`
	Text      string `json:"text"`
	Method    Method `json:"method"`
	EditID    string `json:"edit_id,omitempty"`
	TurnID    string `json:"turn_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	AgentType string `json:"agent_type,omitempty"`
	Commit    string `json:"commit,omitempty"`
}

// AnnotateResult is the file's lines in order, each with its explanation.
type AnnotateResult struct {
	Worktree string          `json:"worktree"`
	Path     string          `json:"path"`
	Lines    []AnnotatedLine `json:"lines"`
}

// explain attributes the line to e.
func (l *AnnotatedLine) explain(m Method, e store.Edit) {
	l.Method, l.EditID, l.TurnID = m, e.ID, e.TurnID
	l.SessionID, l.AgentType = e.SessionID, e.AgentType
}

// Annotate answers Why for every line of a file at once. One git blame
// covers the whole file. Each blamed commit's blob for the file joins the
// Edit whose after-content it is (MethodBlame), with one rev-parse per
// commit. A line blame leaves unexplained, committed or not, is matched by
// its text against one scan of the path's recorded edits (MethodContent).
// Trivial lines and lines nothing matches get MethodNone.
func (s *Service) Annotate(ctx context.Context, req AnnotateRequest) (AnnotateResult, error) {
	if s.NoCheckouts {
		return AnnotateResult{}, fmt.Errorf("%w: annotate reads a checkout, "+
			"which this instance does not hold", ErrNotImplemented)
	}
	if err := s.validateAnnotate(ctx, req); err != nil {
		return AnnotateResult{}, err
	}
	data, err := readWorktreeFile(req.Worktree, req.Path)
	if err != nil {
		return AnnotateResult{}, err
	}
	res := AnnotateResult{Worktree: req.Worktree, Path: req.Path, Lines: []AnnotatedLine{}}
	for i, text := range fileLines(data) {
		res.Lines = append(res.Lines, AnnotatedLine{Line: i + 1, Text: text, Method: MethodNone})
	}
	if err := s.annotateByBlame(ctx, req, res.Lines); err != nil {
		return AnnotateResult{}, err
	}
	if err := s.annotateByContent(ctx, req, res.Lines); err != nil {
		return AnnotateResult{}, err
	}
	return res, nil
}

// validateAnnotate checks the request's shape, then that the worktree is
// one moondb recorded an edit in, so the route cannot read files elsewhere
// on the machine.
func (s *Service) validateAnnotate(ctx context.Context, req AnnotateRequest) error {
	switch {
	case req.Path == "":
		return fmt.Errorf("%w: annotate needs a path", ErrInvalidRequest)
	case !filepath.IsAbs(req.Worktree):
		return fmt.Errorf("%w: annotate needs an absolute worktree, got %q",
			ErrInvalidRequest, req.Worktree)
	case !filepath.IsLocal(filepath.FromSlash(req.Path)):
		return fmt.Errorf("%w: path %q is not inside the worktree", ErrInvalidRequest, req.Path)
	}
	return s.knownWorktree(ctx, req.Worktree)
}

// knownWorktree refuses a worktree moondb never recorded an edit in, so no
// query reads a file from a checkout it has nothing to say about.
func (s *Service) knownWorktree(ctx context.Context, worktree string) error {
	seen, err := s.Store.Edits(ctx, store.EditQuery{Worktree: worktree, Limit: 1})
	if err != nil {
		return fmt.Errorf("query: edits in %s: %w", worktree, err)
	}
	if len(seen) == 0 {
		return fmt.Errorf("%w: no edit recorded in worktree %s", ErrInvalidRequest, worktree)
	}
	return nil
}

// readWorktreeFile reads a regular file of at most annotateMaxBytes.
func readWorktreeFile(worktree, path string) ([]byte, error) {
	full := filepath.Join(worktree, filepath.FromSlash(path))
	info, err := os.Stat(full)
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s is not a file in %s", ErrInvalidRequest, path, worktree)
	}
	if info.Size() > annotateMaxBytes {
		return nil, fmt.Errorf("%w: %s is %d bytes, over the %d annotate reads",
			ErrInvalidRequest, path, info.Size(), annotateMaxBytes)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, fmt.Errorf("query: read %s: %w", full, err)
	}
	return data, nil
}

// fileLines splits content into lines the way git blame counts them: a
// final newline ends the last line rather than starting an empty one, and
// a carriage return before a newline is dropped.
func fileLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	return lines
}

// annotateByBlame explains the committed lines whose commit's blob joins
// an Edit. The blame and every rev-parse share blameBudget. A blame that
// fails, or that counts other lines than the file read (the file changed
// in between), explains nothing and leaves every line to the content
// match.
func (s *Service) annotateByBlame(
	ctx context.Context,
	req AnnotateRequest,
	lines []AnnotatedLine,
) error {
	gitCtx, cancel := context.WithTimeout(ctx, blameBudget)
	defer cancel()
	blamedLines := parseBlameFile(gitloc.Output(gitCtx, req.Worktree,
		"--no-optional-locks", "blame", "--porcelain", "--", req.Path))
	if len(blamedLines) != len(lines) {
		return nil
	}
	j := blobJoin{s: s, repo: req.Repo, blobs: map[string]string{}, edits: map[string]*store.Edit{}}
	for i, b := range blamedLines {
		if b.commit == "" {
			continue
		}
		if b.path == "" {
			b.path = req.Path
		}
		lines[i].Commit = b.commit
		e, err := j.edit(ctx, gitCtx, req.Worktree, b)
		if err != nil {
			return err
		}
		if e != nil {
			lines[i].explain(MethodBlame, *e)
		}
	}
	return nil
}

// blobJoin caches the two lookups behind a blamed line: the blob a commit
// holds at a path, and the Edit whose after-content is that blob. A file
// blamed to a few commits costs a few lookups, not one per line.
type blobJoin struct {
	s     *Service
	repo  string
	blobs map[string]string      // "<commit>:<path>" -> blob id, "" when unknown
	edits map[string]*store.Edit // "<blob>:<path>" -> edit, nil when none
}

// edit returns the Edit behind a committed line, nil when none joins.
// gitCtx carries the git budget; ctx the store reads.
func (j blobJoin) edit(
	ctx, gitCtx context.Context,
	worktree string,
	b blamed,
) (*store.Edit, error) {
	at := b.commit + ":" + b.path
	blob, ok := j.blobs[at]
	if !ok {
		blob = blobAt(gitCtx, worktree, b.commit, b.path)
		j.blobs[at] = blob
	}
	if blob == "" {
		return nil, nil
	}
	key := blob + ":" + b.path
	if e, ok := j.edits[key]; ok {
		return e, nil
	}
	e, found, err := j.s.editByBlob(ctx, j.repo, b.path, blob)
	if err != nil {
		return nil, err
	}
	if found {
		j.edits[key] = &e
	} else {
		j.edits[key] = nil
	}
	return j.edits[key], nil
}

// annotateByContent matches each line still unexplained against what the
// path's recorded edits wrote, newest edit first: the same choice Why's
// top candidate makes. The edits are loaded once, on the first line that
// is not trivial.
func (s *Service) annotateByContent(
	ctx context.Context,
	req AnnotateRequest,
	lines []AnnotatedLine,
) error {
	var written []writtenLines
	loaded := false
	for i := range lines {
		l := &lines[i]
		want := normalize(l.Text)
		if l.Method != MethodNone || trivial(want) {
			continue
		}
		if !loaded {
			var err error
			if written, err = s.writtenByPath(ctx, req); err != nil {
				return err
			}
			loaded = true
		}
		for _, w := range written {
			if w.match(want).count > 0 {
				l.explain(MethodContent, w.edit)
				break
			}
		}
	}
	return nil
}

// writtenByPath loads what every scanned edit of the path wrote.
func (s *Service) writtenByPath(ctx context.Context, req AnnotateRequest) ([]writtenLines, error) {
	edits, err := s.Store.Edits(ctx, contentScan(req.Worktree, req.Repo, req.Path))
	if err != nil {
		return nil, fmt.Errorf("query: edits of %s: %w", req.Path, err)
	}
	out := make([]writtenLines, 0, len(edits))
	for _, e := range edits {
		w, err := s.written(ctx, e)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, nil
}
