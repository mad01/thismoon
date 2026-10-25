package record

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/mad01/thismoon/kit/gitloc"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// maxChangedFiles caps the files one head-moved report lists.
const maxChangedFiles = 500

// gitlinkMode is the tree mode of a submodule, whose id is a commit.
const gitlinkMode = "160000"

// moveHeads reports every worktree the session touched whose HEAD moved
// since the hook last saw it, then remembers the HEADs it saw. A worktree
// git cannot place in time is skipped and keeps its old HEAD, so the next
// close catches the range up.
func (rec *recorder) moveHeads() error {
	worktrees := rec.state.touched()
	if len(worktrees) == 0 {
		return nil
	}
	heads := rec.state.heads()
	for _, wt := range worktrees {
		if rec.spent() {
			break
		}
		loc := rec.locate(wt)
		if loc.Head == "" {
			continue
		}
		old := heads[wt]
		if old != "" && old != loc.Head {
			env := rec.envelope(spool.KindHeadMoved, headMovedID(rec.p.promptID, wt, loc.Head))
			env.HeadMoved = rec.headMoved(loc, old)
			if err := rec.spool.Write(env, nil); err != nil {
				return err
			}
		}
		heads[wt] = loc.Head
	}
	rec.state.saveHeads(heads)
	return nil
}

// headMovedID keys a head-moved report by turn, worktree and new HEAD. The
// worktree goes in as a short hash: two touched worktrees can land on the
// same commit in one turn, and both reports must survive ingest.
func headMovedID(promptID, worktree, head string) string {
	sum := sha256.Sum256([]byte(worktree))
	return promptID + ":head:" + hex.EncodeToString(sum[:6]) + ":" + head
}

// headMoved lists what the range old..loc.Head changed in loc's worktree.
// A diff git cannot produce in time leaves Files empty.
func (rec *recorder) headMoved(loc store.Location, old string) *spool.HeadMoved {
	ctx, cancel := rec.gitCtx()
	defer cancel()
	out := gitloc.Output(
		ctx,
		loc.Worktree,
		"diff-tree",
		"-r",
		"--no-commit-id",
		"-z",
		old,
		loc.Head,
	)
	files, truncated := parseDiffTree(out, maxChangedFiles)
	return &spool.HeadMoved{
		Location: loc, OldHead: old, NewHead: loc.Head, Files: files, Truncated: truncated,
	}
}

// parseDiffTree reads `git diff-tree -r -z` output: per file a
// ":<mode> <mode> <oid> <oid> <status>" record, then the path, or for a
// rename or copy the source and destination paths, each NUL-terminated.
// It lists at most limit files and reports whether more were left out.
// Submodule entries are skipped; output of the wrong shape ends the list.
func parseDiffTree(out string, limit int) ([]spool.ChangedFile, bool) {
	fields := strings.Split(strings.TrimSuffix(out, "\x00"), "\x00")
	var files []spool.ChangedFile
	for i := 0; i < len(fields); {
		meta := strings.Fields(fields[i])
		if len(meta) != 5 || !strings.HasPrefix(meta[0], ":") || meta[4] == "" {
			break
		}
		status := meta[4][:1]
		paths := 1
		if status == "R" || status == "C" {
			paths = 2
		}
		if i+paths >= len(fields) {
			break
		}
		path := fields[i+paths]
		i += paths + 1
		if meta[1] == gitlinkMode {
			continue
		}
		if len(files) == limit {
			return files, true
		}
		blob := meta[3]
		if strings.Trim(blob, "0") == "" {
			blob = "" // the all-zero name of the missing side: a deletion
		}
		files = append(files, spool.ChangedFile{Path: path, Blob: blob, Status: status})
	}
	return files, false
}
