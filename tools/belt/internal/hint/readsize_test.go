package hint

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/cslindex"
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
		{
			"exactly the threshold",
			Input{FilePath: file, TotalLines: config.DefaultLargeFileLines},
			true,
		},
		{
			"below the threshold",
			Input{FilePath: file, TotalLines: config.DefaultLargeFileLines - 1},
			false,
		},
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
			got := NewReadSize(config.Config{}).Check(in)
			if (got != nil) != tt.want {
				t.Fatalf("Check(%+v) = %+v, want advice: %v", in, got, tt.want)
			}
		})
	}
}

// TestReadSizeFollowsLargeFileLines: the hint reads its threshold from the
// one config value it shares with the large-read guard.
func TestReadSizeFollowsLargeFileLines(t *testing.T) {
	file, _ := indexedRepoFile(t)
	h := NewReadSize(config.Config{LargeFileLines: 100})
	for i, tt := range []struct {
		lines int
		want  bool
	}{{100, true}, {99, false}} {
		in := Input{
			Event:      EventRead,
			FilePath:   file,
			TotalLines: tt.lines,
			SessionID:  fmt.Sprint("s", i),
		}
		if got := h.Check(in) != nil; got != tt.want {
			t.Errorf(
				"Check at %d lines with large_file_lines 100: advised = %v, want %v",
				tt.lines,
				got,
				tt.want,
			)
		}
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
			got := NewReadSize(config.Config{}).Check(Input{
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
	h := NewReadSize(config.Config{})
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
	marker := readSizeMarker("a1", "acme/widget", "odd.go \t")
	seen.record(path, []string{marker})
	if !seen.load(path)[marker] {
		t.Errorf("marker %q not found after a seen round trip", marker)
	}
}

// TestReadSizeIsSilentInALinkedWorktree backs the documented worktree rule:
// a linked worktree's .git is a file, which RepoRoot finds, but the
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
	if got := cslindex.RepoRoot(file); got != worktree {
		t.Fatalf("RepoRoot(%s) = %q, want the worktree %q", file, got, worktree)
	}
	in := Input{Event: EventRead, FilePath: file, TotalLines: 412, SessionID: "s1"}
	if got := NewReadSize(config.Config{}).Check(in); got != nil {
		t.Errorf("Check in a linked worktree = %+v, want nil", got)
	}
}

// TestReadSizeCheckOnCSLRead covers the csl_read path, with the same
// threshold, range and file-type rules as a Read. The text form's
// org/repo/path is split at a csl-indexed repo name; the json forms' repo
// and path arrive apart, and the repo must still be indexed.
func TestReadSizeCheckOnCSLRead(t *testing.T) {
	_, _ = indexedRepoFile(t)
	index := filepath.Join(os.Getenv("HOME"), ".config", "csl", "search-index")
	writeEmpty(t, filepath.Join(index, "group%2Fsub%2Ftool_v16.00000.zoekt"))
	in := func(name string) Input { return Input{CSLFile: CSLFile{Name: name}, TotalLines: 843} }
	inJSON := func(repo, path string) Input {
		return Input{CSLFile: CSLFile{Repo: repo, Path: path}, TotalLines: 843}
	}
	big := CSLFile{Name: "acme/widget/internal/big.go"}
	tests := []struct {
		name string
		in   Input
		want bool
	}{
		{"large whole read", in("acme/widget/internal/big.go"), true},
		{
			"exactly the threshold",
			Input{CSLFile: big, TotalLines: config.DefaultLargeFileLines},
			true,
		},
		{
			"below the threshold",
			Input{CSLFile: big, TotalLines: config.DefaultLargeFileLines - 1},
			false,
		},
		{
			"ranged read",
			Input{CSLFile: big, TotalLines: 843, Ranged: true},
			false,
		},
		{"unknown line count", Input{CSLFile: big}, false},
		{"repo name in another case", in("Acme/Widget/internal/big.go"), true},
		{"repo name with a subgroup", in("group/sub/tool/cmd/main.go"), true},
		{"repo csl does not index", in("acme/other/big.go"), false},
		{"path escaping the repo", in("acme/widget/../other/big.go"), false},
		{"repo name alone", in("acme/widget.go"), false},
		{"markdown", in("acme/widget/docs/guide.md"), true},
		{"json", in("acme/widget/data.json"), false},
		{"json form", inJSON("acme/widget", "internal/big.go"), true},
		{"json form, repo in another case", inJSON("Acme/Widget", "internal/big.go"), true},
		{"json form, repo csl does not index", inJSON("acme/other", "big.go"), false},
		{"json form, path escaping the repo", inJSON("acme/widget", "../other/big.go"), false},
		{"json form, absolute path", inJSON("acme/widget", "/internal/big.go"), false},
		{"json form, file type without symbols", inJSON("acme/widget", "data.json"), false},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := tt.in
			in.Event = EventRead
			in.SessionID = "c" + string(rune('a'+i)) // a fresh session per case
			got := NewReadSize(config.Config{}).Check(in)
			if (got != nil) != tt.want {
				t.Fatalf("Check(%+v) = %+v, want advice: %v", in, got, tt.want)
			}
		})
	}
}

// TestReadSizeAdviceOnCSLRead pins the csl_read wording: the file named as
// csl names it, with the read path cleaned, the tool named, and the same
// calls as after a Read.
func TestReadSizeAdviceOnCSLRead(t *testing.T) {
	_, _ = indexedRepoFile(t)
	got := NewReadSize(config.Config{}).Check(Input{
		Event:      EventRead,
		CSLFile:    CSLFile{Name: "acme/widget/./internal/big.go"},
		TotalLines: 843,
		SessionID:  "s1",
	})
	if got == nil {
		t.Fatal("Check = nil, want advice")
	}
	wants := []string{
		"acme/widget/internal/big.go is 843 lines and was read without a range through csl_read.\n",
		`queries: ["f:^internal/big\\.go$"], repo: "^acme/widget$")`,
		`csl_outline(repo: "acme/widget", path: "internal/big.go", include_tests: true)`,
		"csl_read or Read with offset and limit",
	}
	for _, want := range wants {
		if !strings.Contains(got.Text, want) {
			t.Errorf("advice %q does not mention %q", got.Text, want)
		}
	}
	if n := strings.Count(got.Text, "\n") + 1; n != 3 {
		t.Errorf("advice is %d lines, want 3: %q", n, got.Text)
	}
}

// TestReadSizeSharesTheNudgeAcrossReadAndCSLRead pins the shared marker: a
// csl_read and a Read of one file spend one nudge per agent, whichever
// comes first, and a differently cased repo name is the same file.
func TestReadSizeSharesTheNudgeAcrossReadAndCSLRead(t *testing.T) {
	file, _ := indexedRepoFile(t)
	text := CSLFile{Name: "acme/widget/internal/big.go"}
	structured := CSLFile{Repo: "acme/widget", Path: "internal/big.go"}
	h := NewReadSize(config.Config{})
	steps := []struct {
		name, agent string
		cslFile     CSLFile
		path        string
		ranged      bool
		want        bool
	}{
		{"ranged csl_read first", "", text, "", true, false},
		{"whole csl_read", "", text, "", false, true},
		{"csl_read again", "", text, "", false, false},
		{"json-form csl_read again", "", structured, "", false, false},
		{"Read of the same file", "", CSLFile{}, file, false, false},
		{
			"csl_read with the repo in another case",
			"",
			CSLFile{Name: "ACME/widget/internal/big.go"},
			"",
			false,
			false,
		},
		{"subagent Read first", "a-explore", CSLFile{}, file, false, true},
		{"that subagent's csl_read", "a-explore", text, "", false, false},
		{"another subagent's json-form csl_read", "a-plan", structured, "", false, true},
	}
	for _, s := range steps {
		in := Input{
			Event: EventRead, CSLFile: s.cslFile, FilePath: s.path, TotalLines: 412,
			Ranged: s.ranged, SessionID: "s1", AgentID: s.agent,
		}
		if got := h.Check(in) != nil; got != s.want {
			t.Errorf("%s: advised = %v, want %v", s.name, got, s.want)
		}
	}
}

// TestReadSizeKeepsTheJSONRepoBoundary: with shards for both group/sub and
// group/sub/tool, the json forms' repo "group/sub" and path
// "tool/cmd/main.go" stay as csl stated them. Only the text header's joined
// name goes through splitIndexed, which picks the longest indexed name.
func TestReadSizeKeepsTheJSONRepoBoundary(t *testing.T) {
	_, _ = indexedRepoFile(t)
	index := filepath.Join(os.Getenv("HOME"), ".config", "csl", "search-index")
	writeEmpty(
		t,
		filepath.Join(index, "group%2Fsub_v16.00000.zoekt"),
		filepath.Join(index, "group%2Fsub%2Ftool_v16.00000.zoekt"),
	)
	tests := []struct {
		name string
		file CSLFile
		want []string
	}{
		{
			"json form",
			CSLFile{Repo: "group/sub", Path: "tool/cmd/main.go"},
			[]string{
				"group/sub/tool/cmd/main.go is 412 lines",
				`queries: ["f:^tool/cmd/main\\.go$"], repo: "^group/sub$")`,
				`csl_outline(repo: "group/sub", path: "tool/cmd/main.go", include_tests: true)`,
			},
		},
		{
			"text form",
			CSLFile{Name: "group/sub/tool/cmd/main.go"},
			[]string{
				`csl_outline(repo: "group/sub/tool", path: "cmd/main.go", include_tests: true)`,
			},
		},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewReadSize(config.Config{}).Check(Input{
				Event:      EventRead,
				CSLFile:    tt.file,
				TotalLines: 412,
				SessionID:  "b" + string(rune('a'+i)),
			})
			if got == nil {
				t.Fatal("Check = nil, want advice")
			}
			for _, want := range tt.want {
				if !strings.Contains(got.Text, want) {
					t.Errorf("advice %q does not mention %q", got.Text, want)
				}
			}
		})
	}
}

// TestSplitIndexed pins the split of a csl file name: at the longest
// indexed repo name, matched regardless of case, never at a bare org.
func TestSplitIndexed(t *testing.T) {
	repos := map[string]bool{"acme/widget": true, "group/sub": true, "group/sub/tool": true}
	tests := []struct {
		name, repo, rel string
		ok              bool
	}{
		{"acme/widget/internal/big.go", "acme/widget", "internal/big.go", true},
		{"Acme/Widget/big.go", "Acme/Widget", "big.go", true},
		{"group/sub/tool/main.go", "group/sub/tool", "main.go", true},
		{"group/sub/other/main.go", "group/sub", "other/main.go", true},
		{"my org/my repo/a b.go", "", "", false},
		{"acme/big.go", "", "", false},
		{"big.go", "", "", false},
	}
	for _, tt := range tests {
		repo, rel, ok := splitIndexed(tt.name, repos)
		if repo != tt.repo || rel != tt.rel || ok != tt.ok {
			t.Errorf(
				"splitIndexed(%q) = %q, %q, %v; want %q, %q, %v",
				tt.name, repo, rel, ok, tt.repo, tt.rel, tt.ok,
			)
		}
	}
}
