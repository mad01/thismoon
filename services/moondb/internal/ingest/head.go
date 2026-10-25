package ingest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/gitlink"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// blobMatchLimit bounds the edits one blob lookup returns. A blob shared
// by many recorded files (an empty file, a stock license) only needs
// enough of them for gitlink to prefer one of the same path.
const blobMatchLimit = 100

// headMoved links the files a commit range changed to the recorded edits
// whose after-content they hold, by blob id. The link names the range's
// tip, NewHead, and its parent is OldHead: a file committed in the middle
// of a multi-commit range is linked to the tip, not to the commit that
// first took it.
func (in *Ingester) headMoved(ctx context.Context, env spool.Envelope) error {
	h := env.HeadMoved
	if h == nil {
		return errNoPayload(env)
	}
	if h.NewHead == "" {
		return fmt.Errorf("ingest: head-moved %s has no new head", env.ID)
	}
	matches, err := gitlink.MatchCommit(h.Files, in.blobFinder(ctx, h.Location))
	if err != nil {
		return fmt.Errorf("ingest: head-moved %s: %w", env.ID, err)
	}
	var errs []error
	for _, m := range matches {
		l := commitLink(env.At, h, m)
		if err := in.Store.AppendCommitLink(ctx, l); err != nil {
			errs = append(errs, fmt.Errorf("ingest: append commit link %s: %w", l.ID, err))
		}
	}
	return errors.Join(errs...)
}

// blobFinder looks edits up by after-blob within the commit's repository,
// or its worktree when the repository has no origin to name it by. An
// Edit's AfterBlob identifies content, so an edit made in another worktree
// of the same repository is a correct match.
func (in *Ingester) blobFinder(
	ctx context.Context,
	loc store.Location,
) func(string) ([]store.Edit, error) {
	return func(blob string) ([]store.Edit, error) {
		q := store.EditQuery{AfterBlob: blob, Repo: loc.Repo, Limit: blobMatchLimit}
		if loc.Repo == "" {
			q.Worktree = loc.Worktree
		}
		return in.Store.Edits(ctx, q)
	}
}

func commitLink(at time.Time, h *spool.HeadMoved, m gitlink.Match) store.CommitLink {
	return store.CommitLink{
		ID:        h.NewHead + ":" + m.File.Path,
		Repo:      h.Location.Repo,
		Worktree:  h.Location.Worktree,
		Branch:    h.Location.Branch,
		Commit:    h.NewHead,
		Parent:    h.OldHead,
		Path:      m.File.Path,
		Blob:      m.File.Blob,
		EditID:    m.Edit.ID,
		TurnID:    m.Edit.TurnID,
		SessionID: m.Edit.SessionID,
		At:        at,
	}
}
