package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// modsCasesDir holds one directory per scenario, written by
// ../mods/testdata/generate.py from the reference sync_mods.py: case.json
// (the steps, the environment, the symlinks to create), home/ (the files a
// run starts from, the home path written as {HOME}), one directory per state
// a step switches to, and want/, the reference's stdout, stderr, exit code
// and claude calls for each step. Each case lists the Python tests it ports
// in "ports"; the rest cover branches those tests did not.
const modsCasesDir = "../mods/testdata/cases"

// modsCase is a case.json.
type modsCase struct {
	Ports []string `json:"ports"`
	// Steps are check, plan or sync, or state:<dir> to copy that directory
	// of the case over the home before the next step.
	Steps []string `json:"steps"`
	// Env sets variables for every step, e.g. FAKE_CLAUDE_EXIT.
	Env map[string]string `json:"env"`
	// NoClaude leaves the fake claude off PATH.
	NoClaude bool `json:"no_claude"`
	// Symlinks maps a path under the home to its target.
	Symlinks map[string]string `json:"symlinks"`
	// Deviation says why agent-sync deliberately differs from the
	// reference. A want file with a ".deviation" suffix then holds
	// agent-sync's expected value and wins over the reference's.
	Deviation string `json:"deviation"`
}

// TestModsMatchesReference runs every case through the CLI, step by step in
// one temp HOME with a fake claude first on PATH, and compares each step's
// stdout, stderr, exit code and claude calls with the reference's. No step
// may write under HOME: the sync changes state only through claude.
func TestModsMatchesReference(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join(modsCasesDir, "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no cases under %s: %v", modsCasesDir, err)
	}
	fake := modsFakeClaude(t)
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	host = strings.ToLower(host)
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) { runModsCase(t, dir, fake, host) })
	}
}

// modsFakeClaude installs testdata/fake-claude.sh once for the whole run and
// launches it once, untimed: macOS can take seconds to start a freshly
// written executable the first time. Each case links to this copy.
func modsFakeClaude(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "claude")
	if err := os.WriteFile(path, readFile(t, "../mods/testdata/fake-claude.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	warm := exec.Command(path, "warm-up")
	warm.Env = append(os.Environ(), "FAKE_CLAUDE_LOG="+filepath.Join(dir, "warm-up.log"))
	if out, err := warm.CombinedOutput(); err != nil {
		t.Fatalf("fake claude: %v: %s", err, out)
	}
	return path
}

func runModsCase(t *testing.T, dir, fake, host string) {
	var c modsCase
	if err := json.Unmarshal(readFile(t, filepath.Join(dir, "case.json")), &c); err != nil {
		t.Fatal(err)
	}
	home := modsHome(t, dir, c, fake, host)
	log := filepath.Join(home, "claude.log")
	config := filepath.Join(t.TempDir(), "agent-sync.yaml")
	writeFile(t, config, fmt.Sprintf("mods:\n  file: %s\n  dropins: %s\n  profiles_file: %s\n",
		filepath.Join(home, "mods.json"), filepath.Join(home, "mods.d"),
		filepath.Join(home, "config.local.toml")))

	deviations := 0
	for i, step := range c.Steps {
		name := fmt.Sprintf("%d-%s", i+1, step)
		if label, ok := strings.CutPrefix(step, "state:"); ok {
			modsCopyTree(t, filepath.Join(dir, label), home, host)
			continue
		}
		before := modsTree(t, home)
		var stdout, stderr bytes.Buffer
		code := Main([]string{"--config", config, "mods", step}, &stdout, &stderr)
		calls := ""
		if data, err := os.ReadFile(log); err == nil {
			calls = string(data)
			if err := os.Remove(log); err != nil {
				t.Fatal(err)
			}
		}
		if !maps.Equal(before, modsTree(t, home)) {
			t.Errorf("step %s wrote under HOME", name)
		}
		got := map[string]string{
			"stdout": stdout.String(),
			"stderr": stderr.String(),
			"result": fmt.Sprintf("exit: %d\n", code),
			"calls":  calls,
		}
		for _, field := range []string{"stdout", "stderr", "result", "calls"} {
			want, deviates := modsWant(t, dir, name, field)
			if deviates {
				deviations++
			}
			if got := strings.ReplaceAll(got[field], home, "{HOME}"); got != want {
				t.Errorf("step %s %s:\n got %q\nwant %q", name, field, got, want)
			}
		}
	}
	if (c.Deviation != "") != (deviations > 0) {
		t.Errorf("case.json deviation %q, but %d .deviation want files", c.Deviation, deviations)
	}
}

// modsHome builds the case's home: home/ copied in with its placeholders
// filled, the symlinks, and bin/claude linked to the fake. It points HOME,
// PATH and the fake's variables at it, and clears CLAUDE_CONFIG_DIR and
// XDG_CONFIG_HOME unless the case sets them.
func modsHome(t *testing.T, dir string, c modsCase, fake, host string) string {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	modsCopyTree(t, filepath.Join(dir, "home"), home, host)
	for rel, target := range c.Symlinks {
		link := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(strings.ReplaceAll(target, "{HOME}", home), link); err != nil {
			t.Fatal(err)
		}
	}
	bin := filepath.Join(home, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if !c.NoClaude {
		if err := os.Symlink(fake, filepath.Join(bin, "claude")); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	t.Setenv("FAKE_CLAUDE_LOG", filepath.Join(home, "claude.log"))
	t.Setenv("FAKE_CLAUDE_EXIT", "0")
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	for k, v := range c.Env {
		t.Setenv(k, strings.ReplaceAll(v, "{HOME}", home))
	}
	return home
}

// modsCopyTree copies the files under src into home, replacing {HOME} with
// home and {HOST} and {HOST_UPPER} with the hostname. A missing src copies
// nothing.
func modsCopyTree(t *testing.T, src, home, host string) {
	t.Helper()
	replacer := strings.NewReplacer(
		"{HOME}", home, "{HOST_UPPER}", strings.ToUpper(host), "{HOST}", host,
	)
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if os.IsNotExist(err) && path == src {
			return filepath.SkipDir
		}
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, []byte(replacer.Replace(string(data))), 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// modsTree maps every file and symlink under home, except the fake claude
// and its log, to its content or link target.
func modsTree(t *testing.T, home string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(home, path)
		if err != nil {
			return err
		}
		switch {
		case rel == "bin" && d.IsDir():
			return filepath.SkipDir
		case rel == "claude.log" || d.IsDir():
			return nil
		case d.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			tree[rel] = "link: " + target
			return err
		}
		data, err := os.ReadFile(path)
		tree[rel] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

// modsWant returns want/<step>.<field>, or its .deviation override and
// true. A field with no want file is empty.
func modsWant(t *testing.T, dir, step, field string) (string, bool) {
	t.Helper()
	base := filepath.Join(dir, "want", step+"."+field)
	if data, err := os.ReadFile(base + ".deviation"); err == nil {
		return string(data), true
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	data, err := os.ReadFile(base)
	if os.IsNotExist(err) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data), false
}
