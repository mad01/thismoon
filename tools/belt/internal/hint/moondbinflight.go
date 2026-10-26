package hint

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/mad01/thismoon/kit/gitloc"
	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// maxInflightPaths caps the paths the session-start block lists. Past a
// handful the block stops being read (docs/adr/0008); the rest are counted.
const maxInflightPaths = 10

// moondbTimeout bounds the call to moondb serve, the budget the kof hints
// use: the session is opening, and a slow or dead moondb costs this and
// then stays quiet.
const moondbTimeout = 400 * time.Millisecond

// inflightTitleRunes caps a session title in the block.
const inflightTitleRunes = 50

// inflightQueryLimit is how many files the hint asks moondb for. It is far
// past maxInflightPaths because moondb cannot see every commit: files a
// commit took without a link (a delete, an excluded path, a commit made
// outside a session) stay in its answer, and git status drops them only
// after the query. A small limit would let them crowd out the dirty files.
const inflightQueryLimit = 200

// inflightGitTimeout bounds each git exec the hint makes. The session is
// opening, so a git stuck on a lock or a slow filesystem costs this and
// then the hint stays quiet.
const inflightGitTimeout = time.Second

// inflightGitWaitDelay bounds the wait for git's output pipes after the
// timeout kills it, in case a child process still holds them.
const inflightGitWaitDelay = 100 * time.Millisecond

// MoondbInflight tells a session, when it opens, which files of its
// checkout other sessions edited without a commit taking the change yet.
// A session that commits with `git add -A`, stashes or resets would
// otherwise sweep up or throw away work it never saw start.
//
// moondb only links a commit a session saw land, so a file a person
// committed after the session ended still reads as in flight there. The
// hint therefore keeps only the paths git status reports dirty now.
type MoondbInflight struct {
	cfg  config.Config
	base string // moondb serve base URL; overridable for tests
	// topLevel resolves a directory to its checkout root; overridable for
	// tests.
	topLevel func(dir string) string
	// resolveRepo resolves a directory to its canonical host/owner/repo
	// identity for exclude_repos matching; overridable for tests.
	resolveRepo func(dir string) string
	// dirtyPaths reports which of paths git status lists in the checkout
	// at root, and false when it cannot tell; overridable for tests.
	dirtyPaths func(root string, paths []string) (map[string]bool, bool)
	now        func() time.Time
}

// NewMoondbInflight returns the hint wired to moondb serve on MOONDB_PORT
// and to git.
func NewMoondbInflight(cfg config.Config) *MoondbInflight {
	return &MoondbInflight{
		cfg:         cfg,
		base:        moondbBaseURL(),
		topLevel:    boundedTopLevel,
		resolveRepo: boundedCanonicalRepo,
		dirtyPaths:  gitDirtyPaths,
		now:         time.Now,
	}
}

func (h *MoondbInflight) ID() string    { return "moondb-inflight" }
func (h *MoondbInflight) Event() string { return EventSessionStart }

// Check asks moondb about the checkout the session opened in. Every step
// that cannot answer (no session id, no checkout, moondb down or too old,
// an answer that does not decode, git status failing) ends in silence.
// Without a session id the hint cannot leave the session's own edits out,
// so it would name them as another session's. The repo identity and git
// status cost an exec each, so they run only once moondb named a file.
func (h *MoondbInflight) Check(in Input) *Advice {
	if in.SessionID == "" {
		return nil
	}
	root := h.topLevel(nearestDir(in.Cwd))
	if root == "" {
		return nil
	}
	files := queryInflight(h.base, root, in.SessionID)
	if len(files) == 0 || h.cfg.HintRepoExcluded(h.ID(), h.resolveRepo(root)) {
		return nil
	}
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
	}
	dirty, ok := h.dirtyPaths(root, paths)
	if !ok {
		return nil
	}
	var kept []inflightFile
	for _, f := range files {
		if dirty[f.Path] {
			kept = append(kept, f)
		}
	}
	if len(kept) == 0 {
		return nil
	}
	return &Advice{Hint: h.ID(), Text: renderInflight(kept, h.now())}
}

// moondbBaseURL points at moondb serve on loopback. MOONDB_PORT mirrors
// what the moondb CLI and MCP read, so all of them find the same instance.
func moondbBaseURL() string {
	port := os.Getenv("MOONDB_PORT")
	if port == "" {
		port = "7433"
	}
	return "http://127.0.0.1:" + port
}

// inflightFile is the subset of moondb's GET /api/inflight file this hint
// renders.
type inflightFile struct {
	Path       string            `json:"path"`
	LastEditAt time.Time         `json:"last_edit_at"`
	Sessions   []inflightSession `json:"sessions"`
}

type inflightSession struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// queryInflight asks moondb serve for up to inflightQueryLimit of the
// checkout's in-flight files that sessions other than session made. Any
// failure returns nothing: moondb down, a moondb older than the route
// (404), a non-200 answer, or a body that does not decode. A file without
// a path or a session is dropped.
func queryInflight(base, root, session string) []inflightFile {
	q := url.Values{
		"worktree":        {root},
		"exclude_session": {session},
		"limit":           {strconv.Itoa(inflightQueryLimit)},
	}
	client := &http.Client{Timeout: moondbTimeout}
	resp, err := client.Get(base + "/api/inflight?" + q.Encode())
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var body struct {
		Files []inflightFile `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil
	}
	var out []inflightFile
	for _, f := range body.Files {
		if f.Path != "" && len(f.Sessions) > 0 {
			out = append(out, f)
		}
	}
	return out
}

// renderInflight writes the block: a count, one line per path with the
// sessions behind it and the age of its last edit, then what to do.
func renderInflight(files []inflightFile, now time.Time) string {
	var b strings.Builder
	if len(files) == 1 {
		b.WriteString("1 file in this checkout has uncommitted edits by another session:\n")
	} else {
		fmt.Fprintf(&b, "%d files in this checkout have uncommitted edits by other sessions:\n",
			len(files))
	}
	for i, f := range files {
		if i == maxInflightPaths {
			fmt.Fprintf(&b, "  and %d more (moondb inflight lists them)\n", len(files)-i)
			break
		}
		names := make([]string, len(f.Sessions))
		for j, s := range f.Sessions {
			names[j] = sessionLabel(s)
		}
		fmt.Fprintf(
			&b,
			"  %s: %s, %s\n",
			f.Path,
			strings.Join(names, ", "),
			age(now.Sub(f.LastEditAt)),
		)
	}
	b.WriteString("Another session may still be working on them: leave them out of commits, " +
		"stashes and resets unless the user asks.")
	return b.String()
}

// sessionLabel names a session by its title and short id, or the id alone
// when the title is the id.
func sessionLabel(s inflightSession) string {
	id := s.ID
	if len(id) > 8 {
		id = id[:8]
	}
	title := strings.TrimSpace(s.Title)
	if title == "" || title == s.ID {
		return id
	}
	if r := []rune(title); len(r) > inflightTitleRunes {
		title = string(r[:inflightTitleRunes]) + "..."
	}
	return fmt.Sprintf("%q (%s)", title, id)
}

// age renders how long ago a file was edited: "just now", "12m ago",
// "3h ago", "2d ago".
func age(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// gitDirtyPaths runs one git status over paths in the checkout at root and
// returns the ones it lists: modified, staged, deleted, untracked, or either
// side of a rename. The pathspecs are literal, and no optional lock is
// taken, since other sessions work in the same checkout. false means git
// could not answer.
func gitDirtyPaths(root string, paths []string) (map[string]bool, bool) {
	args := []string{
		"--no-optional-locks", "--literal-pathspecs", "status",
		"--porcelain=v1", "-z", "--untracked-files=all", "--",
	}
	out, err := gitWithin(root, append(args, paths...)...)
	if err != nil {
		return nil, false
	}
	return parsePorcelainZ(out), true
}

// boundedTopLevel is the checkout root of dir, "" outside one or when git
// does not answer within inflightGitTimeout.
func boundedTopLevel(dir string) string {
	out, err := gitWithin(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// boundedCanonicalRepo is guard.CanonicalRepoAt under inflightGitTimeout:
// the host/owner/repo identity of dir's origin remote, "" when there is
// none or git does not answer in time.
func boundedCanonicalRepo(dir string) string {
	out, err := gitWithin(dir, "remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	return gitloc.CanonicalRepo(strings.TrimSpace(string(out)))
}

// gitWithin runs one git command in dir and returns its stdout, killing it
// past inflightGitTimeout.
func gitWithin(dir string, args ...string) ([]byte, error) {
	if dir == "" {
		return nil, errors.New("hint: git needs a directory")
	}
	ctx, cancel := context.WithTimeout(context.Background(), inflightGitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.WaitDelay = inflightGitWaitDelay
	return cmd.Output()
}

// parsePorcelainZ reads `git status --porcelain=v1 -z` output: "XY path"
// entries separated by NUL, where a rename or copy entry is followed by its
// source path as one more field.
func parsePorcelainZ(out []byte) map[string]bool {
	dirty := map[string]bool{}
	fields := bytes.Split(out, []byte{0})
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if len(f) < 4 {
			continue
		}
		dirty[string(f[3:])] = true
		if renamed(f[0]) || renamed(f[1]) {
			if i+1 < len(fields) {
				i++
				dirty[string(fields[i])] = true
			}
		}
	}
	return dirty
}

// renamed reports whether a porcelain status letter names a rename or copy,
// whose entry carries the source path as the next field.
func renamed(status byte) bool { return status == 'R' || status == 'C' }
