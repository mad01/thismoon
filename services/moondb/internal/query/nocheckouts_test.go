package query

import (
	"context"
	"errors"
	"testing"

	"github.com/mad01/thismoon/services/moondb/internal/gitlink"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// TestNoCheckoutsReadsNoDisk runs the questions a shared instance answers
// against a worktree that is on disk and committed, where blame would
// name the commit: with NoCheckouts, Why matches the given text alone and
// Annotate refuses, so nothing reads the checkout.
func TestNoCheckoutsReadsNoDisk(t *testing.T) {
	const line = "func One() int { return 1 }"
	ctx := context.Background()
	dir, head := gitRepo(t)
	f := newFixture(t)
	f.svc.NoCheckouts = true
	made := f.adding(edit("e1", 1, dir, "a.go"), line)
	made.AfterBlob = gitlink.BlobID([]byte(committed))
	f.seed(made, store.CommitLink{
		ID: head + ":a.go", Repo: repo, Commit: head, Path: "a.go",
		Blob: made.AfterBlob, EditID: "e1", TurnID: "p1", SessionID: "s1", At: at(3),
	})

	_, err := f.svc.Why(ctx, WhyRequest{Worktree: dir, Path: "a.go", Line: 3})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("Why without the text = %v, want ErrInvalidRequest", err)
	}
	res, err := f.svc.Why(ctx, WhyRequest{Worktree: dir, Path: "a.go", Line: 3, Text: line})
	if err != nil || res.Method != MethodContent || res.Edit == nil || res.Edit.ID != "e1" {
		t.Errorf(
			"Why with the text = %s %+v, %v; want a content match on e1",
			res.Method,
			res.Edit,
			err,
		)
	}
	_, err = f.svc.Annotate(ctx, AnnotateRequest{Worktree: dir, Path: "a.go"})
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("Annotate = %v, want ErrNotImplemented", err)
	}
}
