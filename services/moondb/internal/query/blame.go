package query

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mad01/thismoon/kit/gitloc"
	"github.com/mad01/thismoon/services/moondb/internal/gitlink"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// blameBudget bounds the git calls one Why makes in the worktree.
const blameBudget = 2 * time.Second

// blobMatchLimit bounds the edits one blob lookup returns. A blob many
// recorded files share (an empty file, a stock license) only needs enough
// of them for one of the same path to win.
const blobMatchLimit = 100

// blamed is what git blame says about one line.
type blamed struct {
	// commit last changed the line; "" while the change is uncommitted.
	commit string
	// path is the file's path in commit, which differs from the request's
	// after a rename.
	path string
	text string
}

// whyByBlame answers by git blame when the file is on disk in the
// worktree, and reports whether an Edit explained the line. Either way it
// fills res.Text from the blamed line when the request had none.
func (s *Service) whyByBlame(ctx context.Context, req WhyRequest, res *WhyResult) (bool, error) {
	if req.Line < 1 || !onDisk(req) {
		return false, nil
	}
	b, blob := blame(ctx, req)
	if res.Text == "" {
		res.Text = b.text
	}
	if blob == "" {
		return false, nil
	}
	e, found, err := s.editByBlob(ctx, req.Repo, b.path, blob)
	if err != nil || !found {
		return false, err
	}
	res.Method, res.Commit = MethodBlame, b.commit
	return true, s.attach(ctx, res, e, b.path)
}

// blame runs git blame on the requested line and, when the line is
// committed, git rev-parse for the blob that commit holds for the file.
// Both calls share blameBudget; one git cannot answer in time leaves the
// blob "".
func blame(ctx context.Context, req WhyRequest) (blamed, string) {
	ctx, cancel := context.WithTimeout(ctx, blameBudget)
	defer cancel()
	lines := fmt.Sprintf("%d,%d", req.Line, req.Line)
	b := parseBlame(gitloc.Output(ctx, req.Worktree,
		"--no-optional-locks", "blame", "-L", lines, "--porcelain", "--", req.Path))
	if b.commit == "" {
		return b, ""
	}
	if b.path == "" {
		b.path = req.Path
	}
	return b, blobAt(ctx, req.Worktree, b.commit, b.path)
}

// blobAt returns the blob commit holds at path, "" when git cannot say.
func blobAt(ctx context.Context, worktree, commit, path string) string {
	blob := gitloc.Output(ctx, worktree,
		"--no-optional-locks", "rev-parse", "--verify", "--quiet", commit+":"+path)
	if !isObjectID(blob) {
		return ""
	}
	return blob
}

// parseBlame reads `git blame --porcelain` output for one line; output of
// any other shape yields the zero value.
func parseBlame(out string) blamed {
	lines := parseBlameFile(out)
	if len(lines) == 0 {
		return blamed{}
	}
	return lines[0]
}

// parseBlameFile reads `git blame --porcelain` output, one blamed per line
// in file order. Each line has a header "<sha> <orig-line> <final-line>
// [<count>]", then the commit's key-value lines the first time the commit
// shows up (among them "filename <path>"), then the line itself after a
// tab. An all-zero sha is a line not committed yet. Output of any other
// shape yields nil. A last line with no text is kept: the caller's
// whitespace trim may have cut its tab.
func parseBlameFile(out string) []blamed {
	if out == "" {
		return nil
	}
	var (
		res   []blamed
		sha   string // the header being read; "" between lines
		paths = map[string]string{}
	)
	finish := func(text string) {
		b := blamed{path: paths[sha], text: text}
		if strings.Trim(sha, "0") != "" {
			b.commit = sha
		}
		res = append(res, b)
		sha = ""
	}
	for _, l := range strings.Split(out, "\n") {
		if text, ok := strings.CutPrefix(l, "\t"); ok && sha != "" {
			finish(text)
			continue
		}
		if sha != "" {
			if p, ok := strings.CutPrefix(l, "filename "); ok {
				paths[sha] = p
			}
			continue
		}
		head := strings.Fields(l)
		if len(head) < 3 || !isObjectID(head[0]) {
			return nil
		}
		sha = head[0]
	}
	if sha != "" {
		finish("")
	}
	return res
}

// isObjectID reports whether s is a full git object name, SHA-1 or
// SHA-256.
func isObjectID(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	return strings.Trim(s, "0123456789abcdef") == ""
}

// editByBlob finds the Edit whose after-content is blob, preferring one
// of path the way commit links are matched. Repo narrows the lookup when
// the request names one.
func (s *Service) editByBlob(
	ctx context.Context,
	repo, path, blob string,
) (store.Edit, bool, error) {
	matches, err := gitlink.MatchCommit(
		[]spool.ChangedFile{{Path: path, Blob: blob}},
		func(blob string) ([]store.Edit, error) {
			return s.Store.Edits(ctx, store.EditQuery{
				AfterBlob: blob, Repo: repo, Limit: blobMatchLimit,
			})
		},
	)
	if err != nil {
		return store.Edit{}, false, fmt.Errorf("query: %w", err)
	}
	if len(matches) == 0 {
		return store.Edit{}, false, nil
	}
	return matches[0].Edit, true, nil
}
