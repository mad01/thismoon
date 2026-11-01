package hint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// indexedRepoFile builds a hermetic home holding a csl shard for acme/widget
// and a working tree for it, and returns a file inside that tree and the
// tree's root. The seen file lands under the same home.
func indexedRepoFile(t *testing.T) (file, root string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	index := filepath.Join(home, ".config", "csl", "search-index")
	root = filepath.Join(home, "code", "acme", "widget")
	for _, dir := range []string{index, filepath.Join(root, ".git"), filepath.Join(root, "internal")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	file = filepath.Join(root, "internal", "big.go")
	writeEmpty(t, filepath.Join(index, "acme%2Fwidget_v16.00000.zoekt"), file)
	return file, root
}

func writeEmpty(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReadSizeCheck(t *testing.T) {
	file, root := indexedRepoFile(t)
	unindexed := filepath.Join(filepath.Dir(root), "other")
	if err := os.MkdirAll(filepath.Join(unindexed, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	in := func(path string) Input { return Input{FilePath: path, TotalLines: 412} }
	tests := []struct {
		name string
		in   Input
		want bool
	}{
		{"large whole read", in(file), true},
		{"exactly the threshold", Input{FilePath: file, TotalLines: readSizeMinLines}, true},
		{"below the threshold", Input{FilePath: file, TotalLines: readSizeMinLines - 1}, false},
		{"ranged read", Input{FilePath: file, TotalLines: 412, Ranged: true}, false},
		{"unknown line count", Input{FilePath: file}, false},
		{"relative path", in("internal/big.go"), false},
		{"newline in path", in(file + "\nx"), false},
		{"trailing space in path", in(file + " "), false},
		{"outside any repo", in(filepath.Join(filepath.Dir(root), "loose.go")), false},
		{"repo csl does not index", in(filepath.Join(unindexed, "big.go")), false},
		{"go test file", in(filepath.Join(root, "internal", "big_test.go")), true},
		{"python", in(filepath.Join(root, "tool.py")), true},
		{"markdown", in(filepath.Join(root, "docs", "guide.md")), true},
		{"upper-case extension", in(filepath.Join(root, "README.MD")), true},
		{"json", in(filepath.Join(root, "data.json")), false},
		{"yaml", in(filepath.Join(root, "config.yaml")), false},
		{"toml", in(filepath.Join(root, "config.toml")), false},
		{"plain text", in(filepath.Join(root, "notes.txt")), false},
		{"csv", in(filepath.Join(root, "rows.csv")), false},
		{"lock file", in(filepath.Join(root, "package-lock.lock")), false},
		{"javascript, which csl has no symbols for", in(filepath.Join(root, "app.js")), false},
		{"no extension", in(filepath.Join(root, "Dockerfile")), false},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := tt.in
			in.Event = EventRead
			in.SessionID = "s" + string(rune('a'+i)) // a fresh session per case
			got := NewReadSize().Check(in)
			if (got != nil) != tt.want {
				t.Fatalf("Check(%+v) = %+v, want advice: %v", in, got, tt.want)
			}
		})
	}
}

// TestReadSizeAdviceNamesTheCalls pins the advice: csl_ask first, its path
// and repo escaped and anchored as the regexes csl reads them as, then the
// outline call, then a ranged read. The %q rendering doubles each backslash, as a JSON string
// would, so `big\\.go` in the text is the regex big\.go.
func TestReadSizeAdviceNamesTheCalls(t *testing.T) {
	file, root := indexedRepoFile(t)
	tests := []struct {
		name, path, wantAsk, wantOutline string
	}{
		{
			"code", file,
			`queries: ["f:^internal/big\\.go$"], repo: "^acme/widget$")`,
			`csl_outline(repo: "acme/widget", path: "internal/big.go", include_tests: true)`,
		},
		{
			"markdown asks for headings", filepath.Join(root, "docs", "guide.md"),
			`queries: ["f:^docs/guide\\.md$"], repo: "^acme/widget$")`,
			`csl_outline(repo: "acme/widget", path: "docs/guide.md", include_tests: true, kinds: ["section"])`,
		},
		{
			"a space quotes the path as one zoekt term", filepath.Join(root, "my dir", "big.go"),
			`queries: ["f:\"^my dir/big\\\\.go$\""], repo: "^acme/widget$")`,
			`csl_outline(repo: "acme/widget", path: "my dir/big.go", include_tests: true)`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewReadSize().Check(Input{
				Event: EventRead, FilePath: tt.path, TotalLines: 412, SessionID: tt.name,
			})
			if got == nil {
				t.Fatal("Check = nil, want advice")
			}
			wants := []string{
				tt.path, "412 lines", `csl_ask(question: "<what you need from it>", `,
				tt.wantAsk, tt.wantOutline, "csl_read", "offset and limit",
			}
			for _, want := range wants {
				if !strings.Contains(got.Text, want) {
					t.Errorf("advice %q does not mention %q", got.Text, want)
				}
			}
			ask := strings.Index(got.Text, "csl_ask(")
			if outline := strings.Index(got.Text, "csl_outline("); ask > outline {
				t.Errorf("advice names csl_outline before csl_ask: %q", got.Text)
			}
			if n := strings.Count(got.Text, "\n") + 1; n > 3 {
				t.Errorf("advice is %d lines, want at most 3: %q", n, got.Text)
			}
		})
	}
}

// TestReadSizeSpeaksOncePerFilePerAgent pins the dedupe: a second whole read
// of the same file by the same agent is silent; another file, another
// session, or another agent in the same session is not. A subagent's calls
// carry the parent's session id, so the agent id is what keeps one
// subagent's read from spending the nudge for the main thread. A read the
// hint stayed silent on does not spend the file's nudge either.
func TestReadSizeSpeaksOncePerFilePerAgent(t *testing.T) {
	file, root := indexedRepoFile(t)
	other := filepath.Join(root, "other.go")
	h := NewReadSize()
	steps := []struct {
		name, session, agent, path string
		ranged, want               bool
	}{
		{"ranged read first", "s1", "", file, true, false},
		{"subagent reads it whole first", "s1", "a-explore", file, false, true},
		{"main thread still gets its nudge", "s1", "", file, false, true},
		{"main thread again", "s1", "", file, false, false},
		{"same subagent again", "s1", "a-explore", file, false, false},
		{"another subagent", "s1", "a-plan", file, false, true},
		{
			"same file, unclean path",
			"s1",
			"",
			filepath.Join(root, "internal", "..", "internal", "big.go"),
			false,
			false,
		},
		{"another file", "s1", "", other, false, true},
		{"same file, another session", "s2", "", file, false, true},
	}
	for _, s := range steps {
		in := Input{
			Event: EventRead, FilePath: s.path, TotalLines: 412, Ranged: s.ranged,
			SessionID: s.session, AgentID: s.agent,
		}
		if got := h.Check(in) != nil; got != s.want {
			t.Errorf("%s: advised = %v, want %v", s.name, got, s.want)
		}
	}
}

// TestReadSizeMarkerSurvivesTheSeenStore: the seen store trims each line on
// load, so a marker that kept trailing whitespace would never match its own
// record.
func TestReadSizeMarkerSurvivesTheSeenStore(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := seen.path("s1")
	marker := readSizeMarker("a1", "/repo/odd.go \t")
	seen.record(path, []string{marker})
	if !seen.load(path)[marker] {
		t.Errorf("marker %q not found after a seen round trip", marker)
	}
}

// TestReadSizeIsSilentInALinkedWorktree backs the documented worktree rule:
// a linked worktree's .git is a file, which repoRoot finds, but the
// worktree is named after its own directory, so no shard matches.
func TestReadSizeIsSilentInALinkedWorktree(t *testing.T) {
	_, root := indexedRepoFile(t)
	worktree := filepath.Join(filepath.Dir(filepath.Dir(root)), ".worktrees", "widget", "feature")
	gitFile := filepath.Join(worktree, ".git")
	writeEmpty(t, gitFile)
	pointer := "gitdir: " + filepath.Join(root, ".git", "worktrees", "feature") + "\n"
	if err := os.WriteFile(gitFile, []byte(pointer), 0o644); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(worktree, "internal", "big.go")
	writeEmpty(t, file)
	if got := repoRoot(file); got != worktree {
		t.Fatalf("repoRoot(%s) = %q, want the worktree %q", file, got, worktree)
	}
	in := Input{Event: EventRead, FilePath: file, TotalLines: 412, SessionID: "s1"}
	if got := NewReadSize().Check(in); got != nil {
		t.Errorf("Check in a linked worktree = %+v, want nil", got)
	}
}
