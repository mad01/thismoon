package drift

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
)

// defaultHost is the host of a repo written owner/name.
const defaultHost = "github.com"

// gitChecker checks code refs in local checkouts, each distinct ref once.
type gitChecker struct {
	roots   []string
	checked map[string][]Finding
}

func newGitChecker(roots []string) *gitChecker {
	return &gitChecker{roots: roots, checked: map[string][]Finding{}}
}

// check returns the findings for one code ref.
func (g *gitChecker) check(ctx context.Context, ref lifecycle.CodeRef) []Finding {
	key := ref.String()
	if got, ok := g.checked[key]; ok {
		return got
	}
	got := g.checkRef(ctx, ref)
	g.checked[key] = got
	return got
}

func (g *gitChecker) checkRef(ctx context.Context, ref lifecycle.CodeRef) []Finding {
	finding := func(kind, format string, args ...any) Finding {
		return Finding{Kind: kind, Source: ref.String(), Message: fmt.Sprintf(format, args...)}
	}
	dir, err := g.checkout(ref.Repo)
	if err != nil {
		return []Finding{finding(KindUnresolved, "%v", err)}
	}
	repo := gitRepo{ctx: ctx, dir: dir}
	if !repo.ok("cat-file", "-e", ref.Commit+"^{commit}") {
		return []Finding{finding(KindUnresolved,
			"commit %s is not in the checkout at %s (fetch it)", ref.Commit, dir)}
	}
	pinned, ok := repo.blob(ref.Commit, ref.Path)
	if !ok {
		return []Finding{finding(KindUnresolved,
			"%s does not exist at commit %s", ref.Path, ref.Commit)}
	}
	head, ok := repo.blob("HEAD", ref.Path)
	if !ok {
		f := finding(KindMoved, "%s no longer exists at HEAD", ref.Path)
		if to := repo.renamedTo(ref.Commit, ref.Path); to != "" {
			f.MovedTo = to
			f.Message += "; git sees it renamed to " + to
		}
		return []Finding{f}
	}
	var out []Finding
	if pinned != head {
		out = append(out, finding(KindChanged, "%s changed since %s", ref.Path, ref.Commit))
	}
	if ref.Symbol != "" && !repo.hasSymbol(head, ref.Symbol) {
		out = append(out, finding(KindSymbolMissing,
			"symbol %s is no longer in %s at HEAD", ref.Symbol, ref.Path))
	}
	return out
}

// checkout resolves a repo, host/owner/name or owner/name on github.com, to
// the first <root>/<host>/<owner>/<name> that is a git checkout.
func (g *gitChecker) checkout(repo string) (string, error) {
	parts := strings.Split(strings.TrimSuffix(repo, ".git"), "/")
	switch len(parts) {
	case 2:
		parts = append([]string{defaultHost}, parts...)
	case 3:
	default:
		return "", fmt.Errorf("repo %q is neither host/owner/name nor owner/name", repo)
	}
	if len(g.roots) == 0 {
		return "", fmt.Errorf("repo %s: no code_roots in the brain config to find it under", repo)
	}
	for _, root := range g.roots {
		dir := filepath.Join(append([]string{root}, parts...)...)
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}
	}
	return "", fmt.Errorf("repo %s is not checked out under %s",
		strings.Join(parts, "/"), strings.Join(g.roots, ", "))
}

// gitRepo runs git in one checkout.
type gitRepo struct {
	ctx context.Context
	dir string
}

func (r gitRepo) run(args ...string) ([]byte, error) {
	cmd := exec.CommandContext(r.ctx, "git", append([]string{"-C", r.dir}, args...)...)
	return cmd.Output()
}

func (r gitRepo) ok(args ...string) bool {
	_, err := r.run(args...)
	return err == nil
}

// blob returns the object name of path at rev, or false when it is not
// there.
func (r gitRepo) blob(rev, path string) (string, bool) {
	out, err := r.run("rev-parse", "--verify", "--quiet", rev+":"+path)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

// renamedTo returns the path git's rename detection says path moved to
// between from and HEAD, or "".
func (r gitRepo) renamedTo(from, path string) string {
	out, err := r.run("diff", "--name-status", "-M", "--diff-filter=R", from, "HEAD")
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(string(out), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) == 3 && fields[1] == path {
			return fields[2]
		}
	}
	return ""
}

// hasSymbol reports whether symbol appears in the blob as a whole word: a
// plain textual match, not a parse of the language.
func (r gitRepo) hasSymbol(blob, symbol string) bool {
	out, err := r.run("cat-file", "-p", blob)
	if err != nil {
		return false
	}
	word := regexp.MustCompile(`(^|[^\pL\pN_])` + regexp.QuoteMeta(symbol) + `($|[^\pL\pN_])`)
	return word.Match(out)
}
