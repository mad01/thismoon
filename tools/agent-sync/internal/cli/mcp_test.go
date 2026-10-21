package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// mcpCasesDir holds one directory per scenario: case.json (the steps, what
// the case covers, prune), the inputs (servers.json, overlays/, ralph.toml,
// stamp, and claude/, the fake claude's canned responses), and want/. For
// plan and sync steps want/ holds the reference sync_mcp.py's result,
// written by ../mcp/testdata/reference.py; check steps have no counterpart
// in the script and their want files are written by hand.
const mcpCasesDir = "../mcp/testdata/cases"

// mcpFakeClaude logs its argv and working directory and replays the case's
// canned responses.
const mcpFakeClaude = "../mcp/testdata/fake-claude.sh"

// Where a case's files go inside its HOME: the paths sync_mcp.py
// hardcoded, which are also the mcp section's defaults. reference.py uses
// the same layout.
const (
	mcpServersRel = "code/src/github.com/mad01/dotfiles/recipes/claude-mcp/servers.json"
	mcpOverlayRel = ".config/claude-mcp/servers.d"
	mcpRalphRel   = ".config/ralph/config.local.toml"
	mcpStampRel   = ".cache/claude-mcp/sync.hash"
	mcpFakeDirRel = "fake-claude"
)

// mcpStampHashed starts a want stamp recorded as the text it hashes.
const mcpStampHashed = "sha256 of:\n"

// mcpCase is a case.json.
type mcpCase struct {
	Covers   string            `json:"covers"`
	Steps    []string          `json:"steps"`
	Prune    bool              `json:"prune"`
	NoClaude bool              `json:"no_claude"`
	Files    map[string]string `json:"files"`
	// Deviation says why agent-sync deliberately differs from the
	// reference. A want file with a ".deviation" suffix then holds
	// agent-sync's expected value and wins over the reference's.
	Deviation string `json:"deviation"`
}

// TestMCPMatchesReference runs every case through the CLI, step by step in
// one temp HOME, and compares each step's stdout, stderr, exit code, the
// claude calls it made and the stamp it left with the reference's. A case
// has .deviation want files exactly when its case.json says why, and each
// one must override a want file a step reads.
//
// The test's working directory stands in for a project with local scope
// servers: run there, the fake claude answers with the case's project-*
// responses, as the reference's claude does. Every step must run claude
// somewhere else.
func TestMCPMatchesReference(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join(mcpCasesDir, "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no cases under %s: %v", mcpCasesDir, err)
	}
	bin := mcpFakeBin(t)
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	host = strings.ToLower(host)
	project, err := os.Getwd()
	if err == nil {
		project, err = filepath.EvalSymlinks(project)
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) { runMCPCase(t, dir, bin, host, project) })
	}
}

func runMCPCase(t *testing.T, dir, bin, host, project string) {
	var c mcpCase
	if err := json.Unmarshal(readFile(t, filepath.Join(dir, "case.json")), &c); err != nil {
		t.Fatal(err)
	}
	home := mcpSetupHome(t, dir, c, host)
	t.Setenv("FAKE_CLAUDE_PROJECT", project)
	wants := mcpWants{dir: dir, read: map[string]bool{}}
	path := bin + ":/usr/bin:/bin"
	if c.NoClaude {
		path = "/usr/bin:/bin"
	}
	t.Setenv("PATH", path)
	cfg := filepath.Join(home, "agent-sync.yaml")
	writeFile(t, cfg, fmt.Sprintf("mcp:\n  servers: %s\n  prune: %t\n",
		filepath.Join(home, mcpServersRel), c.Prune))
	normalize := strings.NewReplacer(filepath.Join(bin, "claude"), "{CLAUDE}", home, "{HOME}")

	seen := 0
	for i, step := range c.Steps {
		words := strings.Fields(step)
		id := mcpStepID(i+1, words)
		stampBefore, _ := os.ReadFile(filepath.Join(home, mcpStampRel))
		var stdout, stderr bytes.Buffer
		code := Main(append([]string{"--config", cfg, "mcp"}, words...), &stdout, &stderr)
		calls := mcpLogLines(t, home, "argv.log")
		got := map[string]string{
			"stdout": normalize.Replace(stdout.String()),
			"stderr": normalize.Replace(stderr.String()),
			"result": fmt.Sprintf("exit: %d\n", code),
			"argv":   normalize.Replace(strings.Join(calls[seen:], "")),
		}
		mcpCheckWorkDir(t, id, mcpLogLines(t, home, "pwd.log")[seen:], project)
		seen = len(calls)
		for _, field := range []string{"stdout", "stderr", "result", "argv"} {
			want, ok := wants.file(t, id, field)
			if !ok && field == "stderr" {
				if _, tb := wants.file(t, id, "traceback"); tb {
					t.Errorf("step %s: the reference crashed; add want/%s.stderr.deviation", id, id)
					continue
				}
			}
			if !ok && words[0] != "check" {
				t.Errorf("step %s: no want/%s.%s; run reference.py", id, id, field)
				continue
			}
			if got[field] != want {
				t.Errorf("step %s %s:\n got %q\nwant %q", id, field, got[field], want)
			}
		}
		if words[0] == "check" {
			mcpCheckStampUnchanged(t, home, id, stampBefore)
			continue
		}
		mcpCheckStamp(t, wants, home, host, id)
	}
	deviations, err := filepath.Glob(filepath.Join(dir, "want", "*.deviation"))
	if err != nil {
		t.Fatal(err)
	}
	if (c.Deviation != "") != (len(deviations) > 0) {
		t.Errorf(
			"case.json deviation %q, but %d .deviation want files",
			c.Deviation,
			len(deviations),
		)
	}
	for _, path := range deviations {
		if !wants.read[filepath.Base(path)] {
			t.Errorf("want/%s overrides no want file a step reads", filepath.Base(path))
		}
	}
}

// mcpCheckWorkDir fails unless a step ran its claude calls, whose working
// directories are dirs, in one directory other than project and removed it
// before returning. claude merges a directory's local and project scope
// servers into list and get.
func mcpCheckWorkDir(t *testing.T, id string, dirs []string, project string) {
	t.Helper()
	if len(dirs) == 0 {
		return
	}
	first := strings.TrimSuffix(dirs[0], "\n")
	for _, d := range dirs {
		switch d = strings.TrimSuffix(d, "\n"); {
		case d == project:
			t.Errorf("step %s ran claude in the test's working directory %s", id, d)
			return
		case d != first:
			t.Errorf("step %s ran claude in %s and %s, want one directory", id, first, d)
			return
		}
	}
	if _, err := os.Stat(first); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("step %s left claude's working directory %s (stat: %v)", id, first, err)
	}
}

// mcpStepID names a step's want files: 3-sync-force for "sync --force".
func mcpStepID(n int, words []string) string {
	parts := []string{fmt.Sprint(n)}
	for _, w := range words {
		parts = append(parts, strings.TrimLeft(w, "-"))
	}
	return strings.Join(parts, "-")
}

// mcpWants reads a case's want files and records the name of each one it
// read, so the case can fail on a .deviation file no step reads.
type mcpWants struct {
	dir  string
	read map[string]bool
}

// file returns want/<id>.<field>, or its .deviation override.
func (w mcpWants) file(t *testing.T, id, field string) (string, bool) {
	t.Helper()
	base := filepath.Join(w.dir, "want", id+"."+field)
	for _, path := range []string{base + ".deviation", base} {
		data, err := os.ReadFile(path)
		if err == nil {
			w.read[filepath.Base(path)] = true
			return string(data), true
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatal(err)
		}
	}
	return "", false
}

// mcpCheckStamp compares the stamp with want/<id>.stamp: "absent", "raw:"
// and the bytes, or "sha256 of:" and the text whose hash it must hold.
func mcpCheckStamp(t *testing.T, wants mcpWants, home, host, id string) {
	t.Helper()
	want, ok := wants.file(t, id, "stamp")
	if !ok {
		t.Errorf("step %s: no want/%s.stamp; run reference.py", id, id)
		return
	}
	data, err := os.ReadFile(filepath.Join(home, mcpStampRel))
	switch {
	case want == "absent\n":
		if err == nil {
			t.Errorf("step %s stamp = %q, want none", id, data)
		}
		return
	case err != nil:
		t.Errorf("step %s stamp: %v, want %q", id, err, want)
		return
	}
	if raw, ok := strings.CutPrefix(want, "raw:\n"); ok {
		if string(data) != raw {
			t.Errorf("step %s stamp = %q, want %q", id, data, raw)
		}
		return
	}
	text, ok := strings.CutPrefix(want, mcpStampHashed)
	body, hasHost := strings.CutSuffix(text, "\n{HOST}\n")
	if !ok || !hasHost {
		t.Fatalf("step %s: malformed want stamp %q", id, want)
	}
	sum := sha256.Sum256([]byte(strings.ReplaceAll(body, "{HOME}", home) + "\n" + host))
	if wantHash := hex.EncodeToString(sum[:]) + "\n"; string(data) != wantHash {
		t.Errorf("step %s stamp = %q, want %q, the hash of\n%s", id, data, wantHash, body)
	}
}

// mcpCheckStampUnchanged fails when a check step changed the stamp.
func mcpCheckStampUnchanged(t *testing.T, home, id string, before []byte) {
	t.Helper()
	after, _ := os.ReadFile(filepath.Join(home, mcpStampRel))
	if !bytes.Equal(before, after) {
		t.Errorf("step %s changed the stamp from %q to %q", id, before, after)
	}
}

// mcpSetupHome lays a case out in a fresh HOME, as reference.py does.
func mcpSetupHome(t *testing.T, dir string, c mcpCase, host string) string {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("FAKE_CLAUDE_DIR", filepath.Join(home, mcpFakeDirRel))
	subst := strings.NewReplacer(
		"{HOME}", home, "{HOST_UPPER}", strings.ToUpper(host), "{HOST}", host,
	)
	place := func(src, rel string) {
		data, err := os.ReadFile(src)
		if errors.Is(err, fs.ErrNotExist) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		mcpWriteFile(t, filepath.Join(home, rel), subst.Replace(string(data)))
	}
	place(filepath.Join(dir, "servers.json"), mcpServersRel)
	place(filepath.Join(dir, "ralph.toml"), mcpRalphRel)
	place(filepath.Join(dir, "stamp"), mcpStampRel)
	for sub, rel := range map[string]string{"overlays": mcpOverlayRel, "claude": mcpFakeDirRel} {
		entries, err := os.ReadDir(filepath.Join(dir, sub))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Fatal(err)
		}
		for _, e := range entries {
			place(filepath.Join(dir, sub, e.Name()), filepath.Join(rel, e.Name()))
		}
	}
	if err := os.MkdirAll(filepath.Join(home, mcpFakeDirRel), 0o755); err != nil {
		t.Fatal(err)
	}
	for rel, content := range c.Files {
		mcpWriteFile(t, filepath.Join(home, rel), content)
	}
	return home
}

// mcpFakeBin installs the fake claude in a directory shared by every case
// and runs it once, untimed: a freshly written executable can take seconds
// to launch the first time on macOS.
func mcpFakeBin(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	claude := filepath.Join(bin, "claude")
	if err := os.WriteFile(claude, readFile(t, mcpFakeClaude), 0o755); err != nil {
		t.Fatal(err)
	}
	warm := exec.Command(claude, "mcp", "list")
	warm.Env = append(os.Environ(), "FAKE_CLAUDE_DIR="+t.TempDir())
	if out, err := warm.CombinedOutput(); err != nil {
		t.Fatalf("fake claude: %v: %s", err, out)
	}
	return bin
}

// mcpLogLines returns one of the fake claude's logs, one line per call.
func mcpLogLines(t *testing.T, home, name string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, mcpFakeDirRel, name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.SplitAfter(string(data), "\n")
	return lines[:len(lines)-1] // the empty string after the last newline
}

func mcpWriteFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, data)
}
