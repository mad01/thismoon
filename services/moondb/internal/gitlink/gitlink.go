// Package gitlink computes git object identities without running git and
// matches recorded edits to the commits that took their content. The join
// key is the blob id: an Edit's AfterBlob equals the blob a commit's tree
// holds for the path when the commit took that content unchanged.
package gitlink

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// gitlinkMode is the tree mode of a submodule entry, whose object id names
// a commit in another repository rather than a blob.
const gitlinkMode = "160000"

// BlobID returns the git blob id of content, the lowercase hex SHA-1 that
// `git hash-object` prints for a file with these bytes.
func BlobID(content []byte) string {
	h := sha1.New()
	h.Write([]byte("blob " + strconv.Itoa(len(content)) + "\x00"))
	h.Write(content)
	return hex.EncodeToString(h.Sum(nil))
}

// ParseDiffTree parses the raw output of
// `git diff-tree -r --no-commit-id -z <old> <new>`: per file a
// ":srcmode dstmode srcsha dstsha status" header and its path, NUL
// terminated, with renames and copies carrying the old path before the
// new one. Each file comes back with its new path, its blob in the new
// tree ("" for a deletion or a submodule), and the status letter without
// a similarity score.
func ParseDiffTree(out string) ([]spool.ChangedFile, error) {
	fields := strings.Split(strings.TrimSuffix(out, "\x00"), "\x00")
	if len(fields) == 1 && fields[0] == "" {
		return nil, nil
	}
	var files []spool.ChangedFile
	for i := 0; i < len(fields); {
		f, n, err := parseEntry(fields[i:])
		if err != nil {
			return nil, fmt.Errorf("gitlink: diff-tree field %d: %w", i, err)
		}
		files = append(files, f)
		i += n
	}
	return files, nil
}

// parseEntry reads one header and its paths from fields, returning the
// file and how many fields it used.
func parseEntry(fields []string) (spool.ChangedFile, int, error) {
	header, ok := strings.CutPrefix(fields[0], ":")
	if !ok {
		return spool.ChangedFile{}, 0, fmt.Errorf("header %q does not start with ':'", fields[0])
	}
	meta := strings.Fields(header)
	if len(meta) != 5 || meta[4] == "" {
		return spool.ChangedFile{}, 0, fmt.Errorf(
			"header %q has %d fields, want 5",
			fields[0],
			len(meta),
		)
	}
	dstMode, dstSHA, status := meta[1], meta[3], meta[4][:1]
	paths := 1
	if status == "R" || status == "C" {
		paths = 2
	}
	if len(fields) < 1+paths {
		return spool.ChangedFile{}, 0, fmt.Errorf(
			"status %s wants %d paths after %q",
			status,
			paths,
			fields[0],
		)
	}
	f := spool.ChangedFile{Path: fields[paths], Status: status}
	if dstMode != gitlinkMode && strings.Trim(dstSHA, "0") != "" {
		f.Blob = dstSHA
	}
	return f, 1 + paths, nil
}

// Match pairs a committed file with the recorded Edit whose after-content
// it holds.
type Match struct {
	File spool.ChangedFile
	Edit store.Edit
}

// MatchCommit finds, for every changed file with a blob, the newest Edit
// whose AfterBlob is that blob. An edit of the same path wins over a newer
// edit of another path, so identical content written elsewhere (an empty
// file, a copied license) does not steal the link; an edit of another path
// still matches when no same-path edit does, which covers a file moved
// before it was committed. findByBlob does the lookup and should scope it
// to the commit's repository; its results are filtered on AfterBlob again,
// so a broader lookup only costs time. Files without a match are left out.
func MatchCommit(
	files []spool.ChangedFile,
	findByBlob func(blob string) ([]store.Edit, error),
) ([]Match, error) {
	var matches []Match
	for _, f := range files {
		if f.Blob == "" {
			continue
		}
		edits, err := findByBlob(f.Blob)
		if err != nil {
			return nil, fmt.Errorf("gitlink: find edits for blob %s (%s): %w", f.Blob, f.Path, err)
		}
		if e, ok := newest(edits, f); ok {
			matches = append(matches, Match{File: f, Edit: e})
		}
	}
	return matches, nil
}

// newest picks the edit for f: same path before other paths, then the
// latest At, then the earliest in the list.
func newest(edits []store.Edit, f spool.ChangedFile) (store.Edit, bool) {
	var best store.Edit
	found := false
	for _, e := range edits {
		if e.AfterBlob != f.Blob {
			continue
		}
		if !found || better(e, best, f.Path) {
			best, found = e, true
		}
	}
	return best, found
}

func better(e, than store.Edit, path string) bool {
	if (e.Path == path) != (than.Path == path) {
		return e.Path == path
	}
	return e.At.After(than.At)
}
