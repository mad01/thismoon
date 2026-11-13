package cslindex

import (
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mad01/thismoon/kit/cslignore"
)

func TestZoektTerm(t *testing.T) {
	tests := []struct {
		re   string
		want string
	}{
		{`foo`, `foo`},
		{`foo\(`, `foo\(`},
		// A space would make two AND terms; quoted, it is one phrase.
		{`func Foo`, `"func Foo"`},
		// zoekt unescapes one level of backslashes inside quotes.
		{`func Foo\(`, `"func Foo\\("`},
		// A leading - would negate the term.
		{`-x`, `"-x"`},
		{`say "hi"`, `"say \"hi\""`},
	}
	for _, tc := range tests {
		t.Run(tc.re, func(t *testing.T) {
			if got := ZoektTerm(tc.re); got != tc.want {
				t.Errorf("ZoektTerm(%q) = %q, want %q", tc.re, got, tc.want)
			}
		})
	}
}

// TestLocate pins the index lookup against a hermetic home: a file in a
// repo with a shard resolves to the shard's repo name and a slash path, and
// a file in a repo without one, outside any repo, or in a linked worktree
// of the indexed repo does not resolve.
func TestLocate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	index := filepath.Join(home, ".config", "csl", "search-index")
	root := filepath.Join(home, "code", "Acme", "widget")
	other := filepath.Join(home, "code", "beta", "other")
	worktree := filepath.Join(home, ".worktrees", "widget", "feature")
	for _, d := range []string{index, filepath.Join(root, ".git"), filepath.Join(other, ".git"), worktree} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{
		filepath.Join(index, "acme%2Fwidget_v16.00000.zoekt"),
		filepath.Join(worktree, ".git"),
	} {
		if err := os.WriteFile(f, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name, file, repo, rel string
		ok                    bool
	}{
		{
			"indexed, repo matched regardless of case",
			filepath.Join(root, "internal", "big.go"), "Acme/widget", "internal/big.go", true,
		},
		{"repo csl does not index", filepath.Join(other, "big.go"), "", "", false},
		{"outside any repo", filepath.Join(home, "loose.go"), "", "", false},
		{"linked worktree", filepath.Join(worktree, "big.go"), "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, rel, ok := Locate(tt.file)
			if repo != tt.repo || rel != tt.rel || ok != tt.ok {
				t.Errorf("Locate(%s) = %q, %q, %v, want %q, %q, %v",
					tt.file, repo, rel, ok, tt.repo, tt.rel, tt.ok)
			}
		})
	}
}

// writeFile writes content at path, making its directories.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// indexedHome builds a hermetic home holding a csl shard for acme/widget
// and a main checkout of it, and returns the checkout's root.
func indexedHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeFile(
		t,
		filepath.Join(home, ".config", "csl", "search-index", "acme%2Fwidget_v16.00000.zoekt"),
		"",
	)
	root := filepath.Join(home, "code", "acme", "widget")
	writeFile(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	return root
}

// addWorktree lays out a linked worktree of the checkout at main by files
// alone, the shape git worktree add writes: <main>/.git/worktrees/<name>
// holds commondir, and <home>/.worktrees/<repo>/<name>/.git is a pointer
// file naming that git dir. It returns the worktree's root.
func addWorktree(t *testing.T, main, name string) string {
	t.Helper()
	gitDir := filepath.Join(main, ".git", "worktrees", name)
	writeFile(t, filepath.Join(gitDir, "commondir"), "../..\n")
	wt := filepath.Join(os.Getenv("HOME"), ".worktrees", filepath.Base(main), name)
	writeFile(t, filepath.Join(wt, ".git"), "gitdir: "+gitDir+"\n")
	return wt
}

// TestLinkedWorktree: a .git pointer whose git dir holds commondir is a
// linked worktree, and commondir leads to its main checkout. A main clone,
// a submodule-style pointer and a path outside any checkout are not.
func TestLinkedWorktree(t *testing.T) {
	main := indexedHome(t)
	wt := addWorktree(t, main, "feature")
	sub := filepath.Join(main, "third_party", "lib")
	writeFile(t, filepath.Join(sub, ".git"), "gitdir: ../../.git/modules/lib\n")
	tests := []struct {
		name, path, root, main string
		ok                     bool
	}{
		{"file in a worktree", filepath.Join(wt, "internal", "big.go"), wt, main, true},
		{"worktree root", wt, wt, main, true},
		{"main clone", filepath.Join(main, "big.go"), "", "", false},
		{"submodule pointer", filepath.Join(sub, "big.go"), "", "", false},
		{"outside any checkout", filepath.Join(os.Getenv("HOME"), "loose.go"), "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, gotMain, ok := LinkedWorktree(tt.path)
			if root != tt.root || gotMain != tt.main || ok != tt.ok {
				t.Errorf("LinkedWorktree(%s) = %q, %q, %v, want %q, %q, %v",
					tt.path, root, gotMain, ok, tt.root, tt.main, tt.ok)
			}
		})
	}
}

// TestLinkedWorktreeReadsOnlySmallRegularFiles: the lookup runs on every
// hook call, so it reads .git and commondir only as small regular files. A
// .git that is a FIFO is "not a worktree" at once, where opening it would
// block until a writer came. A commondir that is a symlink is refused even
// when it leads to a valid file, since one leading to a device would be
// read without end.
func TestLinkedWorktreeReadsOnlySmallRegularFiles(t *testing.T) {
	main := indexedHome(t)
	worktrees := filepath.Join(os.Getenv("HOME"), ".worktrees", "widget")

	fifo := filepath.Join(worktrees, "fifo")
	if err := os.MkdirAll(fifo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(fifo, ".git"), 0o644); err != nil {
		t.Fatal(err)
	}
	done := make(chan bool, 1)
	go func() {
		_, _, ok := LinkedWorktree(filepath.Join(fifo, "big.go"))
		done <- ok
	}()
	select {
	case ok := <-done:
		if ok {
			t.Error("LinkedWorktree(FIFO .git) = ok, want not a worktree")
		}
	case <-time.After(2 * time.Second):
		t.Error("LinkedWorktree(FIFO .git) blocked, want not a worktree at once")
	}

	gitDir := filepath.Join(main, ".git", "worktrees", "linked")
	target := filepath.Join(t.TempDir(), "commondir")
	writeFile(t, target, "../..\n")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(gitDir, "commondir")); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(worktrees, "linked")
	writeFile(t, filepath.Join(linked, ".git"), "gitdir: "+gitDir+"\n")
	if _, _, ok := LinkedWorktree(filepath.Join(linked, "big.go")); ok {
		t.Error("LinkedWorktree(symlinked commondir) = ok, want refused")
	}
}

// TestResolve: a file in an indexed main checkout resolves to its repo, and
// a file in a linked worktree of one resolves to the worktree root, whose
// csl_ask call names the worktree and no repo. A worktree of a repo csl does
// not index does not resolve.
func TestResolve(t *testing.T) {
	main := indexedHome(t)
	wt := addWorktree(t, main, "feature")
	other := filepath.Join(os.Getenv("HOME"), "code", "beta", "other")
	writeFile(t, filepath.Join(other, ".git", "HEAD"), "")
	otherWT := addWorktree(t, other, "feature")

	loc, ok := Resolve(filepath.Join(main, "internal", "big.go"))
	want := Located{Root: main, Rel: "internal/big.go", Repo: "acme/widget"}
	if !ok || loc != want {
		t.Errorf("Resolve(main checkout) = %+v, %v, want %+v, true", loc, ok, want)
	}

	loc, ok = Resolve(filepath.Join(wt, "internal", "big.go"))
	want = Located{Root: wt, Rel: "internal/big.go", Repo: "acme/widget", Worktree: true}
	if !ok || loc != want {
		t.Errorf("Resolve(worktree) = %+v, %v, want %+v, true", loc, ok, want)
	}
	call := loc.AskCall()
	wantCall := `csl_ask(question: "<what you need from it>", ` +
		`queries: ["f:^internal/big\\.go$"], worktree: "` + wt + `")`
	if call != wantCall {
		t.Errorf("worktree AskCall = %s, want %s", call, wantCall)
	}

	if loc, ok := Resolve(filepath.Join(otherWT, "big.go")); ok {
		t.Errorf("Resolve(worktree of an unindexed repo) = %+v, want not resolved", loc)
	}
}

// TestSkipped: every file csl's walk leaves out, and every file csl_ask
// refuses as binary, counts as skipped, so the large-read guard passes it.
// An ordinary text file, a hidden file, a gitignored one and a text file
// with too many trigrams for zoekt, which zoekt keeps by name and csl_ask
// reads from disk, do not.
func TestSkipped(t *testing.T) {
	root := t.TempDir()
	text := strings.Repeat("\tx := compute(x) // a line of code\n", 400)
	files := map[string]string{
		"internal/big.go":           text,
		".golangci.yml":             text,
		".gitignore":                "out.log\n",
		"out.log":                   text,
		"node_modules/pkg/index.js": text,
		"vendor/lib/lib.go":         text,
		"web/dist/app.js":           text,
		".cache/big.go":             text,
		".github/workflows/ci.yml":  text,
		".cslignore":                "generated/\n",
		"generated/big.go":          text,
		"huge.txt":                  text + strings.Repeat("x", cslignore.SizeMax),
		"late-nul.go":               strings.Repeat(text, 5) + "\x00",
		"nul-first.go":              "\x00" + text,
		"random.txt":                randomText(400, 150),
	}
	for rel, content := range files {
		writeFile(t, filepath.Join(root, filepath.FromSlash(rel)), content)
	}
	if err := os.Symlink(filepath.Join(root, "internal", "big.go"), filepath.Join(root, "link.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "internal"), filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		rel  string
		want bool
	}{
		{"internal/big.go", false},
		{".golangci.yml", false},
		{"out.log", false},
		{"node_modules/pkg/index.js", true},
		{"vendor/lib/lib.go", true},
		{"web/dist/app.js", true},
		{".cache/big.go", true},
		{".github/workflows/ci.yml", true},
		{"generated/big.go", true},
		{"huge.txt", true},
		{"late-nul.go", true},
		{"nul-first.go", true},
		{"random.txt", false},
		{"link.go", true},
		{"linked/big.go", true},
		{"gone.go", true},
	}
	for _, tt := range tests {
		if got := Skipped(root, tt.rel); got != tt.want {
			t.Errorf("Skipped(%s) = %v, want %v", tt.rel, got, tt.want)
		}
	}
}

// randomText is lines of letters and digits from a fixed seed: text with
// far more distinct trigrams than zoekt keeps, the shape of an encoded blob.
func randomText(lines, width int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	r := rand.New(rand.NewPCG(1, 2))
	var b strings.Builder
	for range lines {
		for range width {
			b.WriteByte(alphabet[r.IntN(len(alphabet))])
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// TestAskCallAnchorsBoth: the path and the repo are regexes to csl, so both
// are escaped and anchored. The %q rendering doubles each backslash.
func TestAskCallAnchorsBoth(t *testing.T) {
	got := AskCall("acme/widget", "internal/big.go")
	want := `csl_ask(question: "<what you need from it>", ` +
		`queries: ["f:^internal/big\\.go$"], repo: "^acme/widget$")`
	if got != want {
		t.Errorf("AskCall = %s, want %s", got, want)
	}
	if spaced := AskCall("acme/widget", "my dir/big.go"); !strings.Contains(
		spaced, `queries: ["f:\"^my dir/big\\\\.go$\""]`,
	) {
		t.Errorf("AskCall with a space = %s, want the path quoted as one term", spaced)
	}
}
