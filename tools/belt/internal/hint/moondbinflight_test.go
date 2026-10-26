package hint

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

const (
	inflightRoot = "/work/repo"
	inflightRepo = "github.com/o/repo"
)

// inflightNow is the clock every rendering test reads ages against.
var inflightNow = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

// inflightAnswer is one GET /api/inflight file edited by session at
// inflightNow less ago.
func inflightAnswer(path, session, title string, ago time.Duration) map[string]any {
	at := inflightNow.Add(-ago)
	return map[string]any{
		"path": path, "last_edit_at": at, "linked_commit": nil,
		"sessions": []map[string]any{
			{"id": session, "title": title, "last_edit_at": at, "edits": 1},
		},
	}
}

// moondbServer answers GET /api/inflight with files and records the query
// it got.
func moondbServer(t *testing.T, files ...map[string]any) (*httptest.Server, *url.Values) {
	t.Helper()
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/inflight" {
			http.NotFound(w, r)
			return
		}
		got = r.URL.Query()
		_ = json.NewEncoder(w).Encode(map[string]any{"worktree": inflightRoot, "files": files})
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

// inflightHint is the hint with git replaced: the checkout is inflightRoot
// and git status lists dirty.
func inflightHint(base string, dirty ...string) *MoondbInflight {
	set := map[string]bool{}
	for _, p := range dirty {
		set[p] = true
	}
	return &MoondbInflight{
		cfg:         testConfig(),
		base:        base,
		topLevel:    func(string) string { return inflightRoot },
		resolveRepo: func(string) string { return inflightRepo },
		dirtyPaths:  func(string, []string) (map[string]bool, bool) { return set, true },
		now:         func() time.Time { return inflightNow },
	}
}

func sessionStart() Input {
	return Input{Event: EventSessionStart, Cwd: inflightRoot, SessionID: "me-session"}
}

func TestMoondbInflightListsOtherSessionsDirtyFiles(t *testing.T) {
	srv, got := moondbServer(t,
		inflightAnswer("a.go", "88087760-aaaa", "fix the parser", 12*time.Minute),
		inflightAnswer("b.go", "1234abcd-bbbb", "1234abcd-bbbb", 3*time.Hour),
	)
	a := inflightHint(srv.URL, "a.go", "b.go").Check(sessionStart())
	if a == nil {
		t.Fatal("Check = nil, want advice for two dirty in-flight files")
	}
	want := "2 files in this checkout have uncommitted edits by other sessions:\n" +
		"  a.go: \"fix the parser\" (88087760), 12m ago\n" +
		"  b.go: 1234abcd, 3h ago\n" +
		"Another session may still be working on them: leave them out of commits, " +
		"stashes and resets unless the user asks."
	if a.Hint != "moondb-inflight" || a.Text != want {
		t.Errorf("advice = %s %q\nwant %q", a.Hint, a.Text, want)
	}
	if got.Get("worktree") != inflightRoot || got.Get("exclude_session") != "me-session" ||
		got.Get("limit") != strconv.Itoa(inflightQueryLimit) {
		t.Errorf("query = %v, want the checkout root, the session to exclude and the limit", *got)
	}
}

// Files a commit took without a link stay in moondb's answer ahead of the
// dirty ones; the cap counts only what git status keeps.
func TestMoondbInflightCapsAfterGitStatus(t *testing.T) {
	var files []map[string]any
	for i := range maxInflightPaths + 2 {
		files = append(files, inflightAnswer(
			fmt.Sprintf("committed%d.go", i), "s1", "t", time.Duration(i)*time.Minute))
	}
	files = append(files, inflightAnswer("dirty.go", "s2", "t", time.Hour))
	srv, _ := moondbServer(t, files...)
	a := inflightHint(srv.URL, "dirty.go").Check(sessionStart())
	if a == nil || !strings.Contains(a.Text, "  dirty.go: ") ||
		!strings.HasPrefix(a.Text, "1 file in this checkout") {
		t.Errorf("advice = %+v, want dirty.go listed alone", a)
	}
}

// Without a session id the hint cannot leave the session's own edits out,
// so it does not ask moondb at all.
func TestMoondbInflightNeedsTheSessionID(t *testing.T) {
	srv, got := moondbServer(t, inflightAnswer("a.go", "s1", "t", time.Minute))
	in := sessionStart()
	in.SessionID = ""
	if a := inflightHint(srv.URL, "a.go").Check(in); a != nil {
		t.Errorf("Check without a session id = %q, want nil", a.Text)
	}
	if len(*got) != 0 {
		t.Errorf("moondb was asked %v, want no query", *got)
	}
}

// moondb links only a commit a session saw land, so a file committed by
// hand still reads as in flight there; git status decides.
func TestMoondbInflightDropsFilesGitReportsClean(t *testing.T) {
	srv, _ := moondbServer(t,
		inflightAnswer("a.go", "s1", "one", time.Minute),
		inflightAnswer("committed.go", "s2", "two", time.Hour),
	)
	a := inflightHint(srv.URL, "a.go").Check(sessionStart())
	if a == nil ||
		!strings.HasPrefix(
			a.Text,
			"1 file in this checkout has uncommitted edits by another session:\n",
		) ||
		strings.Contains(a.Text, "committed.go") {
		t.Errorf("advice = %+v, want a.go alone", a)
	}
	if a := inflightHint(srv.URL).Check(sessionStart()); a != nil {
		t.Errorf("advice with every file clean = %q, want nil", a.Text)
	}
}

func TestMoondbInflightCapsThePaths(t *testing.T) {
	var files []map[string]any
	var dirty []string
	for i := range maxInflightPaths + 3 {
		p := filepath.Join("dir", string(rune('a'+i))+".go")
		files = append(files, inflightAnswer(p, "s1", "t", time.Duration(i)*time.Minute))
		dirty = append(dirty, p)
	}
	srv, _ := moondbServer(t, files...)
	a := inflightHint(srv.URL, dirty...).Check(sessionStart())
	if a == nil {
		t.Fatal("Check = nil, want capped advice")
	}
	lines := strings.Split(a.Text, "\n")
	if !strings.HasPrefix(lines[0], "13 files") || len(lines) != 1+maxInflightPaths+2 ||
		lines[maxInflightPaths+1] != "  and 3 more (moondb inflight lists them)" {
		t.Errorf("advice = %q, want 13 counted, %d listed and 3 more", a.Text, maxInflightPaths)
	}
}

func TestMoondbInflightSilence(t *testing.T) {
	ok, _ := moondbServer(t, inflightAnswer("a.go", "s1", "t", time.Minute))
	empty, _ := moondbServer(t)
	older := httptest.NewServer(http.NotFoundHandler()) // a moondb before /api/inflight
	t.Cleanup(older.Close)
	garbled := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"files": "not a list"}`))
	}))
	t.Cleanup(garbled.Close)

	tests := []struct {
		name string
		hint *MoondbInflight
	}{
		{"moondb down", inflightHint("http://127.0.0.1:1", "a.go")},
		{"older moondb answers 404", inflightHint(older.URL, "a.go")},
		{"empty answer", inflightHint(empty.URL, "a.go")},
		{"answer that does not decode", inflightHint(garbled.URL, "a.go")},
		{"outside a checkout", func() *MoondbInflight {
			h := inflightHint(ok.URL, "a.go")
			h.topLevel = func(string) string { return "" }
			return h
		}()},
		{"git status fails", func() *MoondbInflight {
			h := inflightHint(ok.URL, "a.go")
			h.dirtyPaths = func(string, []string) (map[string]bool, bool) { return nil, false }
			return h
		}()},
		{"repo opted out", func() *MoondbInflight {
			h := inflightHint(ok.URL, "a.go")
			h.cfg = config.Config{Hints: map[string]config.Toggle{
				"moondb-inflight": {ExcludeRepos: []string{inflightRepo}},
			}}
			return h
		}()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if a := tc.hint.Check(sessionStart()); a != nil {
				t.Errorf("Check = %q, want nil", a.Text)
			}
		})
	}
}

// A moondb that never answers costs the budget and nothing more.
func TestMoondbInflightGivesUpAtTheBudget(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		<-release
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) }) // runs first, so Close is not left waiting
	start := time.Now()
	if a := inflightHint(srv.URL, "a.go").Check(sessionStart()); a != nil {
		t.Errorf("Check = %q, want nil", a.Text)
	}
	if took := time.Since(start); took > moondbTimeout+time.Second {
		t.Errorf("Check took %s, want about the %s budget", took, moondbTimeout)
	}
}

func TestAge(t *testing.T) {
	for d, want := range map[time.Duration]string{
		30 * time.Second: "just now",
		12 * time.Minute: "12m ago",
		3 * time.Hour:    "3h ago",
		47 * time.Hour:   "47h ago",
		72 * time.Hour:   "3d ago",
	} {
		if got := age(d); got != want {
			t.Errorf("age(%s) = %q, want %q", d, got, want)
		}
	}
}

func TestParsePorcelainZ(t *testing.T) {
	out := " M a.go\x00?? new dir/b.go\x00R  renamed.go\x00old.go\x00 D gone.go\x00"
	got := parsePorcelainZ([]byte(out))
	for _, p := range []string{"a.go", "new dir/b.go", "renamed.go", "old.go", "gone.go"} {
		if !got[p] {
			t.Errorf("%q missing from %v", p, got)
		}
	}
	if len(got) != 5 {
		t.Errorf("parsed %d paths, want 5: %v", len(got), got)
	}
}

// TestGitDirtyPathsReadsTheRealStatus runs git: a modified, an untracked
// and a clean file, and a path a glob would widen to the dirty ones, so the
// pathspecs must be literal.
func TestGitDirtyPathsReadsTheRealStatus(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		if out, err := gitCommand(dir, args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q")
	write("clean.go", "a\n")
	write("changed.go", "a\n")
	run("add", ".")
	run("-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", "init")
	write("changed.go", "b\n")
	write("sub/new.go", "c\n")
	got, ok := gitDirtyPaths(dir, []string{"clean.go", "changed.go", "sub/new.go", "*.go"})
	if !ok {
		t.Fatal("gitDirtyPaths failed")
	}
	if !got["changed.go"] || !got["sub/new.go"] || got["clean.go"] || len(got) != 2 {
		t.Errorf("dirty = %v, want changed.go and sub/new.go only", got)
	}
	if globbed, _ := gitDirtyPaths(dir, []string{"clean.go", "*.go"}); len(globbed) != 0 {
		t.Errorf("dirty = %v for a clean file and a literal *.go, want none", globbed)
	}
	if _, ok := gitDirtyPaths(t.TempDir(), []string{"a.go"}); ok {
		t.Error("gitDirtyPaths outside a checkout reported ok")
	}
}

// A git that hangs costs the git budget, after which the hint reads it as
// no checkout.
func TestGitWithinGivesUpAtTheBudget(t *testing.T) {
	bin := t.TempDir()
	stub := "#!/bin/sh\nexec sleep 10\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	start := time.Now()
	if root := boundedTopLevel(t.TempDir()); root != "" {
		t.Errorf("boundedTopLevel = %q, want empty", root)
	}
	if took := time.Since(start); took > inflightGitTimeout+time.Second {
		t.Errorf("boundedTopLevel took %s, want about the %s budget", took, inflightGitTimeout)
	}
}

func TestMoondbProbeReadsTheStatus(t *testing.T) {
	status := func(code int) string {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			if r.URL.Path != "/api/inflight" || q.Get("worktree") != "/" || q.Get("limit") != "1" {
				http.Error(w, "unexpected probe "+r.URL.String(), http.StatusTeapot)
				return
			}
			w.WriteHeader(code)
		}))
		t.Cleanup(srv.Close)
		return srv.URL
	}
	tests := []struct {
		name string
		base string
		want func(error) bool
	}{
		{"route answers", status(http.StatusOK), func(err error) bool { return err == nil }},
		{"older moondb", status(http.StatusNotFound), func(err error) bool {
			return errors.Is(err, ErrMoondbNoInflight)
		}},
		{"route fails", status(http.StatusInternalServerError), func(err error) bool {
			return err != nil && !errors.Is(err, ErrMoondbNoInflight)
		}},
		{"moondb down", "http://127.0.0.1:1", func(err error) bool {
			return err != nil && !errors.Is(err, ErrMoondbNoInflight)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := MoondbProbe(tc.base); !tc.want(err) {
				t.Errorf("MoondbProbe = %v", err)
			}
		})
	}
}
