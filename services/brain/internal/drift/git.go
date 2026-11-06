package drift

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
)

// defaultHost is the host of a repo written owner/name.
const defaultHost = "github.com"

// GitTimeout bounds one git call in a checkout.
const GitTimeout = 30 * time.Second

// MaxBlobBytes caps the file drift reads to look for a symbol; a larger one
// is a too-large finding.
const MaxBlobBytes = 8 << 20

// gitWaitDelay is how long a timed-out git's output pipes may stay open
// before drift stops waiting on them.
const gitWaitDelay = time.Second

// gitEnv keeps every git call local and non-interactive: no credential
// prompt, and no fetch of a missing object from a partial clone's remote.
var gitEnv = []string{"GIT_TERMINAL_PROMPT=0", "GIT_NO_LAZY_FETCH=1"}

// gitChecker checks code refs in local checkouts, each distinct ref once.
type gitChecker struct {
	roots   []string
	timeout time.Duration
	maxBlob int64
	checked map[string][]Finding
}

func newGitChecker(roots []string) *gitChecker {
	return &gitChecker{
		roots:   roots,
		timeout: GitTimeout,
		maxBlob: MaxBlobBytes,
		checked: map[string][]Finding{},
	}
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
	unresolved := func(err error) []Finding { return []Finding{finding(KindUnresolved, "%v", err)} }
	dir, err := g.checkout(ref.Repo)
	if err != nil {
		return unresolved(err)
	}
	repo := gitRepo{ctx: ctx, dir: dir, timeout: g.timeout}
	if _, err := repo.run("cat-file", "-e", ref.Commit+"^{commit}"); err != nil {
		if answeredNo(err) {
			return []Finding{finding(KindUnresolved,
				"commit %s is not in the checkout at %s (fetch it)", ref.Commit, dir)}
		}
		return unresolved(err)
	}
	pinned, found, err := repo.blob(ref.Commit, ref.Path)
	switch {
	case err != nil:
		return unresolved(err)
	case !found:
		return []Finding{finding(KindUnresolved,
			"%s does not exist at commit %s", ref.Path, ref.Commit)}
	}
	head, found, err := repo.blob("HEAD", ref.Path)
	switch {
	case err != nil:
		return unresolved(err)
	case !found:
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
	if ref.Symbol != "" {
		out = append(out, g.checkSymbol(repo, ref, head, finding)...)
	}
	return out
}

// checkSymbol looks for ref's symbol in the HEAD blob of its path. A blob
// over the cap is left unread, and one the checkout does not hold (a
// partial clone) is unresolved, never fetched.
func (g *gitChecker) checkSymbol(
	repo gitRepo,
	ref lifecycle.CodeRef,
	blob string,
	finding func(kind, format string, args ...any) Finding,
) []Finding {
	size, err := repo.blobSize(blob)
	switch {
	case answeredNo(err):
		return []Finding{finding(KindUnresolved,
			"the content of %s at HEAD is not in the checkout at %s "+
				"(a partial clone; drift never fetches it)", ref.Path, repo.dir)}
	case err != nil:
		return []Finding{finding(KindUnresolved, "%v", err)}
	case size > g.maxBlob:
		return []Finding{finding(KindTooLarge,
			"%s at HEAD is %d bytes, over the %d drift reads; symbol %s not checked",
			ref.Path, size, g.maxBlob, ref.Symbol)}
	}
	found, err := repo.hasSymbol(blob, ref.Symbol)
	if err != nil {
		return []Finding{finding(KindUnresolved, "%v", err)}
	}
	if !found {
		return []Finding{finding(KindSymbolMissing,
			"symbol %s is no longer in %s at HEAD", ref.Symbol, ref.Path)}
	}
	return nil
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

// gitRepo runs git in one checkout, each call bounded by timeout.
type gitRepo struct {
	ctx     context.Context
	dir     string
	timeout time.Duration
}

// errTimeout marks a git call cut off at its timeout.
var errTimeout = errors.New("timed out")

func (r gitRepo) run(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(r.ctx, r.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", r.dir}, args...)...)
	cmd.Env = append(os.Environ(), gitEnv...)
	cmd.WaitDelay = gitWaitDelay
	out, err := cmd.Output()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, fmt.Errorf("git %s in %s %w after %s", args[0], r.dir, errTimeout, r.timeout)
	}
	if err != nil {
		return nil, fmt.Errorf("git %s in %s: %w", args[0], r.dir, err)
	}
	return out, nil
}

// answeredNo reports whether err is git exiting non-zero on its own: the
// object or path asked about is not there. A timeout or a git that would not
// start is not an answer.
func answeredNo(err error) bool {
	var exit *exec.ExitError
	return err != nil && !errors.Is(err, errTimeout) && errors.As(err, &exit)
}

// blob returns the object name of path at rev; found is false when it is
// not there.
func (r gitRepo) blob(rev, path string) (name string, found bool, err error) {
	out, err := r.run("rev-parse", "--verify", "--quiet", rev+":"+path)
	switch {
	case answeredNo(err):
		return "", false, nil
	case err != nil:
		return "", false, err
	}
	return strings.TrimSpace(string(out)), true, nil
}

// blobSize returns the size in bytes of a blob the checkout holds.
func (r gitRepo) blobSize(blob string) (int64, error) {
	out, err := r.run("cat-file", "-s", blob)
	if err != nil {
		return 0, err
	}
	size, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("git cat-file -s %s in %s: %w", blob, r.dir, err)
	}
	return size, nil
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
func (r gitRepo) hasSymbol(blob, symbol string) (bool, error) {
	out, err := r.run("cat-file", "-p", blob)
	if err != nil {
		return false, err
	}
	word := regexp.MustCompile(`(^|[^\pL\pN_])` + regexp.QuoteMeta(symbol) + `($|[^\pL\pN_])`)
	return word.Match(out), nil
}
