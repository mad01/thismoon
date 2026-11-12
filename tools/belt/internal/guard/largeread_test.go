package guard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// largeReadHome builds a hermetic home holding a csl shard for acme/widget,
// a working tree for it with one file of 400 lines, and an empty session
// transcript. It returns the tree's root, the large file and the transcript.
func largeReadHome(t *testing.T) (root, big, transcript string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	root = filepath.Join(home, "code", "acme", "widget")
	big = filepath.Join(root, "internal", "big.go")
	transcript = filepath.Join(home, "projects", "p", "s1.jsonl")
	writeTestFile(
		t,
		filepath.Join(home, ".config", "csl", "search-index", "acme%2Fwidget_v16.00000.zoekt"),
		"",
	)
	writeTestFile(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	writeTestFile(t, big, textLines(400))
	writeTestFile(t, transcript, "")
	return root, big, transcript
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// textLines is n newline-terminated lines of Go-looking text.
func textLines(n int) string {
	return strings.Repeat("\tx := compute(x) // a line of code\n", n)
}

// editLine is one transcript line in the shape Claude Code records an
// assistant tool call: tool_use blocks inside message.content.
func editLine(tool, path string) string {
	return fmt.Sprintf(
		`{"type":"assistant","message":{"role":"assistant","content":[`+
			`{"type":"tool_use","id":"toolu_1","name":%q,"input":{"replace_all":false,"file_path":%q,"old_string":"a"}}]}}`+"\n",
		tool, path,
	)
}

func checkRead(in Input) *Denial {
	in.Event = EventRead
	return NewLargeRead(config.Config{}).Check(in)
}

// TestLargeReadDeniesAWholeReadOfALargeIndexedFile pins the refusal: it
// names the file, hands over the csl_ask call for it with the path and repo
// escaped and anchored, and says in one line that a ranged read passes.
func TestLargeReadDeniesAWholeReadOfALargeIndexedFile(t *testing.T) {
	_, big, transcript := largeReadHome(t)
	d := checkRead(Input{FilePath: big, TranscriptPath: transcript})
	if d == nil {
		t.Fatal("Check = nil, want a denial")
	}
	wants := []string{
		"belt[large-read]: ", big, "300 lines or more",
		`csl_ask(question: "<what you need from it>", queries: ["f:^internal/big\\.go$"], repo: "^acme/widget$")`,
		"a ranged read passes this guard",
	}
	for _, want := range wants {
		if !strings.Contains(d.Reason, want) {
			t.Errorf("reason %q does not contain %q", d.Reason, want)
		}
	}
	if n := strings.Count(d.Reason, "\n") + 1; n != 3 {
		t.Errorf("reason is %d lines, want 3: %q", n, d.Reason)
	}
}

// TestLargeReadAllows covers every read the guard lets through: anything
// short, ranged, outside the index, or not lines of text.
func TestLargeReadAllows(t *testing.T) {
	root, big, transcript := largeReadHome(t)
	home := os.Getenv("HOME")
	unindexed := filepath.Join(home, "code", "beta", "other")
	writeTestFile(t, filepath.Join(unindexed, ".git", "HEAD"), "")
	writeTestFile(t, filepath.Join(unindexed, "big.go"), textLines(400))
	writeTestFile(t, filepath.Join(home, "loose", "big.go"), textLines(400))
	writeTestFile(t, filepath.Join(root, "small.go"), textLines(299))
	writeTestFile(t, filepath.Join(root, "image.png"), textLines(400))
	writeTestFile(t, filepath.Join(root, "paper.PDF"), textLines(400))
	writeTestFile(t, filepath.Join(root, "notes.ipynb"), textLines(400))
	writeTestFile(t, filepath.Join(root, "blob.bin"), textLines(400)+"\x00")
	unreadable := filepath.Join(root, "secret.go")
	writeTestFile(t, unreadable, textLines(400))
	if err := os.Chmod(unreadable, 0); err != nil {
		t.Fatal(err)
	}
	if f, err := os.Open(unreadable); err == nil {
		_ = f.Close()
		t.Log("running with permission to read a mode-0 file; the unreadable case checks nothing")
	}
	worktree := filepath.Join(home, ".worktrees", "widget", "feature")
	writeTestFile(
		t,
		filepath.Join(worktree, ".git"),
		"gitdir: "+filepath.Join(root, ".git", "worktrees", "feature")+"\n",
	)
	writeTestFile(t, filepath.Join(worktree, "big.go"), textLines(400))

	tests := []struct {
		name string
		in   Input
	}{
		{"299 lines", Input{FilePath: filepath.Join(root, "small.go")}},
		{"ranged read", Input{FilePath: big, Ranged: true}},
		{"relative path", Input{FilePath: "internal/big.go"}},
		{"repo csl does not index", Input{FilePath: filepath.Join(unindexed, "big.go")}},
		{"outside any repo", Input{FilePath: filepath.Join(home, "loose", "big.go")}},
		{"linked worktree, which has no shard", Input{FilePath: filepath.Join(worktree, "big.go")}},
		{"image", Input{FilePath: filepath.Join(root, "image.png")}},
		{"pdf, extension in capitals", Input{FilePath: filepath.Join(root, "paper.PDF")}},
		{"notebook", Input{FilePath: filepath.Join(root, "notes.ipynb")}},
		{"binary", Input{FilePath: filepath.Join(root, "blob.bin")}},
		{"missing file", Input{FilePath: filepath.Join(root, "gone.go")}},
		{"directory", Input{FilePath: filepath.Join(root, "internal")}},
		{"unreadable file", Input{FilePath: unreadable}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := tt.in
			in.TranscriptPath = transcript
			if d := checkRead(in); d != nil {
				t.Errorf("Check(%+v) = %q, want allow", in, d.Reason)
			}
		})
	}
}

// TestLargeReadCountsLines pins the threshold edge: large_file_lines lines
// deny whether or not the last one ends in a newline, one fewer allows, and
// the config value moves the threshold.
func TestLargeReadCountsLines(t *testing.T) {
	root, _, transcript := largeReadHome(t)
	file := filepath.Join(root, "edge.go")
	tests := []struct {
		name     string
		content  string
		minLines int
		deny     bool
	}{
		{"exactly 300 lines", textLines(300), 0, true},
		{"300th line without a newline", textLines(299) + "last", 0, true},
		{"299 lines", textLines(299), 0, false},
		{"configured threshold reached", textLines(50), 50, true},
		{"configured threshold missed", textLines(49), 50, false},
		{"empty file", "", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeTestFile(t, file, tt.content)
			g := NewLargeRead(config.Config{LargeFileLines: tt.minLines})
			d := g.Check(Input{Event: EventRead, FilePath: file, TranscriptPath: transcript})
			if (d != nil) != tt.deny {
				t.Errorf("Check = %v, want deny %v", d, tt.deny)
			}
		})
	}
}

// TestLargeReadPassesAFileTheAgentEdited pins the edit exemption and its
// scope. The main thread's calls are in the session transcript; a
// subagent's are in agent-<id>.jsonl under <session>/subagents/, and only
// its own count. A transcript that cannot be found or read allows.
func TestLargeReadPassesAFileTheAgentEdited(t *testing.T) {
	root, big, transcript := largeReadHome(t)
	subagent := filepath.Join(
		strings.TrimSuffix(transcript, ".jsonl"),
		"subagents",
		"agent-a1.jsonl",
	)
	unclean := filepath.Join(root, "internal", "..", "internal", "big.go")
	tests := []struct {
		name, session, own, agentID string
		pathOverride                string // when set, the payload's transcript_path
		deny                        bool
	}{
		{name: "no edit in the session", deny: true},
		{name: "edited earlier", session: editLine("Edit", big)},
		{name: "written earlier", session: editLine("Write", big)},
		{name: "multi-edited earlier", session: editLine("MultiEdit", big)},
		{name: "edited under an unclean spelling", session: editLine("Edit", unclean)},
		{
			name:    "another file edited",
			session: editLine("Edit", filepath.Join(root, "other", "big.go")),
			deny:    true,
		},
		{name: "only read earlier", session: editLine("Read", big), deny: true},
		{name: "subagent edited it", own: editLine("Edit", big), agentID: "a1"},
		{
			name:    "subagent, main thread edited it",
			session: editLine("Edit", big),
			agentID: "a1",
			deny:    true,
		},
		{
			name: "subagent, payload names its own transcript", own: editLine("Edit", big), agentID: "a1",
			pathOverride: subagent,
		},
		{name: "subagent transcript missing", agentID: "a2"},
		{name: "agent id with a path in it", agentID: "../a1"},
		{name: "no transcript path", pathOverride: "-"},
		{
			name:         "session transcript missing",
			pathOverride: filepath.Join(filepath.Dir(transcript), "gone.jsonl"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeTestFile(
				t,
				transcript,
				`{"type":"user","message":{"role":"user","content":"hi"}}`+"\n"+tt.session,
			)
			writeTestFile(t, subagent, tt.own)
			in := Input{FilePath: big, TranscriptPath: transcript, AgentID: tt.agentID}
			switch tt.pathOverride {
			case "":
			case "-":
				in.TranscriptPath = ""
			default:
				in.TranscriptPath = tt.pathOverride
			}
			if d := checkRead(in); (d != nil) != tt.deny {
				t.Errorf("Check(%+v) = %v, want deny %v", in, d, tt.deny)
			}
		})
	}
}
