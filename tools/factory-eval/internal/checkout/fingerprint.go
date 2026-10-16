package checkout

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// maxChanges caps the lines Changes returns, so one run that rewrote every
// ref does not bury the results file.
const maxChanges = 20

// Print is the state of a checkout as far as a run could change it: HEAD,
// the working tree status, the worktree count and every ref.
type Print struct {
	Head       string
	StatusHash string
	Worktrees  int
	Refs       map[string]string
}

// Fingerprint reads the state of repo. It skips git's optional locks, so
// it never refreshes the index of a checkout another session works in.
func Fingerprint(ctx context.Context, repo string) (Print, error) {
	queries := [][]string{
		{"rev-parse", "HEAD"},
		{"status", "--porcelain"},
		{"worktree", "list", "--porcelain"},
		{"for-each-ref", "--format=%(refname) %(objectname)"},
	}
	outs := make([]string, len(queries))
	for i, q := range queries {
		out, err := git(ctx, repo, append([]string{"--no-optional-locks"}, q...)...)
		if err != nil {
			return Print{}, err
		}
		outs[i] = string(out)
	}
	sum := sha256.Sum256([]byte(outs[1]))
	p := Print{
		Head:       strings.TrimSpace(outs[0]),
		StatusHash: hex.EncodeToString(sum[:]),
		Worktrees:  strings.Count("\n"+outs[2], "\nworktree "),
		Refs:       map[string]string{},
	}
	for _, line := range strings.Split(strings.TrimSpace(outs[3]), "\n") {
		if ref, sha, ok := strings.Cut(line, " "); ok {
			p.Refs[ref] = sha
		}
	}
	return p, nil
}

// Changes lists how after differs from p, one line per change, the ref
// lines sorted, at most maxChanges lines. None means nothing changed.
func (p Print) Changes(after Print) []string {
	var out []string
	if p.Head != after.Head {
		out = append(out, fmt.Sprintf("HEAD moved from %s to %s", p.Head, after.Head))
	}
	if p.StatusHash != after.StatusHash {
		out = append(out, "git status --porcelain changed")
	}
	if p.Worktrees != after.Worktrees {
		out = append(out, fmt.Sprintf("worktrees were %d, now %d", p.Worktrees, after.Worktrees))
	}
	out = append(out, refChanges(p.Refs, after.Refs)...)
	if len(out) > maxChanges {
		out = append(out[:maxChanges], fmt.Sprintf("and %d more", len(out)-maxChanges))
	}
	return out
}

func refChanges(before, after map[string]string) []string {
	var out []string
	for ref, sha := range after {
		old, ok := before[ref]
		switch {
		case !ok:
			out = append(out, "ref added: "+ref)
		case old != sha:
			out = append(out, "ref moved: "+ref)
		}
	}
	for ref := range before {
		if _, ok := after[ref]; !ok {
			out = append(out, "ref deleted: "+ref)
		}
	}
	sort.Strings(out)
	return out
}
