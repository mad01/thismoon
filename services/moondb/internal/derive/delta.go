package derive

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/pmezard/go-difflib/difflib"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// diffContext is the number of unchanged lines around each change in a
// delta's unified diff, the same as git's default.
const diffContext = 3

// observedMatches compares the derived after-content with the disk read:
// byte for byte when the hook shipped the read, by blob id when it shipped
// only the id because the file was past the chunk cap. compared is false
// when the hook shipped neither.
func observedMatches(
	after, observed content,
	afterBlob, observedBlob string,
) (matches, compared bool) {
	switch {
	case observed.ok:
		return bytes.Equal(after.data, observed.data), true
	case observedBlob != "":
		return afterBlob == observedBlob, true
	default:
		return false, false
	}
}

// delta records how the disk read differs from the derived after-content:
// something besides the tool, such as a formatter hook or a person, wrote
// the file too.
func (d *Deriver) delta(
	ctx context.Context,
	e store.Edit,
	after, observed content,
) (*store.UnattributedDelta, error) {
	key, err := d.putDiff(ctx, e.Path, after.data, observed.data)
	if err != nil {
		return nil, err
	}
	return &store.UnattributedDelta{
		ID:         e.ID + ":observed",
		SessionID:  e.SessionID,
		TurnID:     e.TurnID,
		At:         e.At,
		Location:   e.Location,
		Path:       e.Path,
		FromChunk:  e.AfterChunk,
		ToChunk:    e.ObservedChunk,
		PatchChunk: key,
		Cause:      store.DeltaObservedMismatch,
	}, nil
}

// putDiff stores the unified diff from a to b, two contents of path, and
// returns its chunk key.
func (d *Deriver) putDiff(ctx context.Context, name string, a, b []byte) (string, error) {
	diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        diffLines(a),
		B:        diffLines(b),
		FromFile: path.Join("a", name),
		ToFile:   path.Join("b", name),
		Context:  diffContext,
	})
	if err != nil {
		return "", fmt.Errorf("derive: diff contents of %s: %w", name, err)
	}
	key, err := d.Chunks.Put(ctx, []byte(diff))
	if err != nil {
		return "", fmt.Errorf("derive: store diff of %s: %w", name, err)
	}
	return key, nil
}

// diffLines splits content for difflib, which writes each line as given
// and so needs every line to end in a newline. A last line without one
// gets the marker diff(1) prints for it.
func diffLines(b []byte) []string {
	lines := strings.SplitAfter(string(b), "\n")
	last := len(lines) - 1
	if lines[last] == "" {
		return lines[:last]
	}
	lines[last] += "\n\\ No newline at end of file\n"
	return lines
}
