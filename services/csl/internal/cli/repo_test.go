package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/csl/internal/picker"
	"github.com/mad01/thismoon/services/csl/internal/repo/catalogspec"
	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
)

// isolateConfigEnv points every path csl resolves inside home for the rest
// of the test: the config file, the state directory, and anything derived
// from them.
//
// Faking HOME alone is not enough. confdir honors XDG_CONFIG_HOME and
// XDG_STATE_HOME ahead of HOME, and GitHub's Linux runners export
// XDG_CONFIG_HOME while macOS does not — which is why a suite that isolates
// on HOME alone passes on a laptop and reads the runner's real config in
// CI. Both variables are pinned under home rather than emptied, so the
// fixture is found whichever branch confdir takes.
func isolateConfigEnv(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("CSL_CONFIG", "")
	config.SetPath("")
}

// setupTestConfig writes config.yaml into a fake HOME and points csl at it.
func setupTestConfig(t *testing.T, cfgContent string) (home string) {
	t.Helper()
	tmp := t.TempDir()
	cfgDir := filepath.Join(tmp, ".config", "csl")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cfgContent), 0o644); err != nil {
		t.Fatal(err)
	}
	isolateConfigEnv(t, tmp)
	return tmp
}

func TestRepoListFlag(t *testing.T) {
	// Create temp dir with a git repo
	tmp := t.TempDir()
	repoDir := filepath.Join(tmp, "org", "myrepo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitInit(t, repoDir)
	gitSetRemote(t, repoDir, "git@github.com:testorg/myrepo.git")

	// Set up config under fake HOME
	setupTestConfig(t, "dirs:\n  - "+tmp+"\n")

	// Capture output
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"repo", "--list"})

	// Reset flag for test isolation
	repoListFlag = false
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "testorg/myrepo") {
		t.Errorf("expected output to contain testorg/myrepo, got: %s", output)
	}
	if !strings.Contains(output, repoDir) {
		t.Errorf("expected output to contain repo path %s, got: %s", repoDir, output)
	}

	// Reset for other tests
	rootCmd.SetArgs(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
	repoListFlag = false
}

// TestRepoListFlagNoConfig pins the zero-config first run: a machine with no
// config file lists nothing and says which file to create, rather than
// failing. The hint goes to stderr so stdout stays a clean (empty) list.
func TestRepoListFlagNoConfig(t *testing.T) {
	home := t.TempDir()
	isolateConfigEnv(t, home)

	var out, errOut bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&errOut)
	rootCmd.SetArgs([]string{"repo", "--list"})
	repoListFlag = false

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("repo --list with no config = %v, want nil", err)
	}
	if got := out.String(); got != "" {
		t.Errorf("stdout = %q, want an empty list", got)
	}
	wantPath := filepath.Join(home, ".config", "csl", "config.yaml")
	if got := errOut.String(); !strings.Contains(got, wantPath) {
		t.Errorf("stderr = %q, want it to name %q", got, wantPath)
	}

	rootCmd.SetArgs(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
	repoListFlag = false
}

// TestRepoListFlagEmptyDirsErrors is the other half of the contract: a config
// file that exists and still finds nothing is a misconfiguration, so it stays
// an error naming the file.
func TestRepoListFlagEmptyDirsErrors(t *testing.T) {
	tmp := t.TempDir()
	emptyDir := filepath.Join(tmp, "empty")
	if err := os.MkdirAll(emptyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	home := setupTestConfig(t, "dirs:\n  - "+emptyDir+"\n")

	var out, errOut bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&errOut)
	rootCmd.SetArgs([]string{"repo", "--list"})
	repoListFlag = false

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("repo --list with a configured but empty dir = nil, want an error")
	}
	wantPath := filepath.Join(home, ".config", "csl", "config.yaml")
	if !strings.Contains(err.Error(), wantPath) {
		t.Errorf("error %q does not name the config file %q", err, wantPath)
	}

	rootCmd.SetArgs(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
	repoListFlag = false
}

func TestRepoListFlagEmptyDirs(t *testing.T) {
	tmp := t.TempDir()
	emptyDir := filepath.Join(tmp, "empty")
	_ = os.MkdirAll(emptyDir, 0o755)

	setupTestConfig(t, "dirs:\n  - "+emptyDir+"\n")

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"repo", "--list"})
	repoListFlag = false

	err := rootCmd.Execute()
	if err == nil {
		t.Error("expected error when no repos found")
	}

	rootCmd.SetArgs(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
	repoListFlag = false
}

func TestRepoListMultipleRepos(t *testing.T) {
	tmp := t.TempDir()

	// Create multiple repos
	for _, name := range []string{"repo-a", "repo-b", "repo-c"} {
		repoDir := filepath.Join(tmp, name)
		if err := os.MkdirAll(repoDir, 0o755); err != nil {
			t.Fatal(err)
		}
		gitInit(t, repoDir)
		gitSetRemote(t, repoDir, "git@github.com:org/"+name+".git")
	}

	setupTestConfig(t, "dirs:\n  - "+tmp+"\n")

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"repo", "--list"})
	repoListFlag = false

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute error: %v", err)
	}

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d: %s", len(lines), output)
	}

	for _, name := range []string{"org/repo-a", "org/repo-b", "org/repo-c"} {
		if !strings.Contains(output, name) {
			t.Errorf("expected output to contain %s", name)
		}
	}

	rootCmd.SetArgs(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
	repoListFlag = false
}

func TestRepoToonFlag(t *testing.T) {
	tmp := t.TempDir()
	repoDir := filepath.Join(tmp, "org", "myrepo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitInit(t, repoDir)
	gitSetRemote(t, repoDir, "git@github.com:testorg/myrepo.git")

	setupTestConfig(t, "dirs:\n  - "+tmp+"\n")

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"repo", "--toon"})
	repoListFlag = false
	repoJSONFlag = false
	repoToonFlag = false

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "repos") {
		t.Errorf("expected 'repos' key in TOON output, got: %s", output)
	}
	if !strings.Contains(output, "testorg/myrepo") {
		t.Errorf("expected testorg/myrepo in TOON output, got: %s", output)
	}
	if !strings.Contains(output, repoDir) {
		t.Errorf("expected path %s in TOON output, got: %s", repoDir, output)
	}

	rootCmd.SetArgs(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
	repoListFlag = false
	repoJSONFlag = false
	repoToonFlag = false
}

func TestRepoJSONFlag(t *testing.T) {
	tmp := t.TempDir()
	repoDir := filepath.Join(tmp, "org", "myrepo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitInit(t, repoDir)
	gitSetRemote(t, repoDir, "git@github.com:testorg/myrepo.git")

	setupTestConfig(t, "dirs:\n  - "+tmp+"\n")

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"repo", "--json"})
	repoListFlag = false
	repoJSONFlag = false

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute error: %v", err)
	}

	var repos []struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal(buf.Bytes(), &repos); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, buf.String())
	}

	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	if repos[0].Name != "testorg/myrepo" {
		t.Errorf("expected name testorg/myrepo, got %s", repos[0].Name)
	}
	if repos[0].Path != repoDir {
		t.Errorf("expected path %s, got %s", repoDir, repos[0].Path)
	}

	rootCmd.SetArgs(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
	repoListFlag = false
	repoJSONFlag = false
}

func TestRepoQuerySingleMatchPrintsPath(t *testing.T) {
	tmp := t.TempDir()
	for _, name := range []string{"dotfiles", "kitty-session"} {
		repoDir := filepath.Join(tmp, name)
		if err := os.MkdirAll(repoDir, 0o755); err != nil {
			t.Fatal(err)
		}
		gitInit(t, repoDir)
		gitSetRemote(t, repoDir, "git@github.com:mad01/"+name+".git")
	}

	setupTestConfig(t, "dirs:\n  - "+tmp+"\n")

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"repo", "DOTFILES"}) // also exercises case-insensitivity
	resetRepoFlags()

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute error: %v\noutput: %s", err, buf.String())
	}

	wantPath := filepath.Join(tmp, "dotfiles")
	got := strings.TrimSpace(buf.String())
	if got != wantPath {
		t.Errorf("expected %q, got %q", wantPath, got)
	}

	rootCmd.SetArgs(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
	resetRepoFlags()
}

func TestRepoQueryNoMatchErrors(t *testing.T) {
	tmp := t.TempDir()
	repoDir := filepath.Join(tmp, "dotfiles")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitInit(t, repoDir)
	gitSetRemote(t, repoDir, "git@github.com:mad01/dotfiles.git")

	setupTestConfig(t, "dirs:\n  - "+tmp+"\n")

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"repo", "nope-not-a-real-repo"})
	resetRepoFlags()

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for no matches, got nil")
	}
	if !strings.Contains(err.Error(), "no repos match query") {
		t.Errorf("expected error message about no matches, got: %v", err)
	}

	rootCmd.SetArgs(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
	resetRepoFlags()
}

func TestRepoQueryMultipleMatchesErrors(t *testing.T) {
	tmp := t.TempDir()
	for _, owner := range []string{"org-a", "org-b"} {
		repoDir := filepath.Join(tmp, owner+"-dotfiles")
		if err := os.MkdirAll(repoDir, 0o755); err != nil {
			t.Fatal(err)
		}
		gitInit(t, repoDir)
		gitSetRemote(t, repoDir, "git@github.com:"+owner+"/dotfiles.git")
	}

	setupTestConfig(t, "dirs:\n  - "+tmp+"\n")

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"repo", "dotfiles"})
	resetRepoFlags()

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for multiple matches, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "multiple repos match") {
		t.Errorf("expected multi-match error, got: %v", err)
	}
	for _, want := range []string{"org-a/dotfiles", "org-b/dotfiles"} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected candidate %q in error, got: %v", want, err)
		}
	}

	rootCmd.SetArgs(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
	resetRepoFlags()
}

func TestRepoListWithQueryFiltersWithoutErroring(t *testing.T) {
	tmp := t.TempDir()
	for _, name := range []string{"dotfiles", "kitty-session", "ralph"} {
		repoDir := filepath.Join(tmp, name)
		if err := os.MkdirAll(repoDir, 0o755); err != nil {
			t.Fatal(err)
		}
		gitInit(t, repoDir)
		gitSetRemote(t, repoDir, "git@github.com:mad01/"+name+".git")
	}

	setupTestConfig(t, "dirs:\n  - "+tmp+"\n")

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"repo", "--list", "kitty"})
	resetRepoFlags()

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute error: %v\noutput: %s", err, buf.String())
	}

	output := buf.String()
	if !strings.Contains(output, "mad01/kitty-session") {
		t.Errorf("expected kitty-session in output, got: %s", output)
	}
	for _, unwanted := range []string{"mad01/dotfiles", "mad01/ralph"} {
		if strings.Contains(output, unwanted) {
			t.Errorf("did not expect %q in filtered list, got: %s", unwanted, output)
		}
	}

	rootCmd.SetArgs(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
	resetRepoFlags()
}

func resetRepoFlags() {
	repoListFlag = false
	repoJSONFlag = false
	repoToonFlag = false
	repoSkippedFlag = false
	repoComponentFlag = ""
	repoOwnerFlag = ""
	repoSystemFlag = ""
}

// TestRepoSkippedFlag pins the answer to "csl cannot see my repo": --skipped
// lists what discovery removed, tab-separated with the reason, and keeps
// working on the machine where the filters took everything — which is exactly
// the machine that has to ask.
func TestRepoSkippedFlag(t *testing.T) {
	tmp := t.TempDir()
	dropped := filepath.Join(tmp, "org", "elsewhere")
	if err := os.MkdirAll(dropped, 0o755); err != nil {
		t.Fatal(err)
	}
	gitInit(t, dropped)
	gitSetRemote(t, dropped, "git@git.example.com:testorg/elsewhere.git")

	setupTestConfig(t, "dirs:\n  - "+tmp+"\nindex:\n  hosts:\n    - github.com\n")

	t.Run("plain", func(t *testing.T) {
		out := runRepoCmd(t, "repo", "--list", "--skipped")
		want := "testorg/elsewhere\t" + dropped + "\thost git.example.com not in index.hosts\n"
		if out != want {
			t.Errorf("stdout = %q, want %q", out, want)
		}
	})

	t.Run("json", func(t *testing.T) {
		out := runRepoCmd(t, "repo", "--json", "--skipped")
		var items []struct {
			Name   string `json:"name"`
			Path   string `json:"path"`
			Remote string `json:"remote"`
			Host   string `json:"host"`
			Reason string `json:"reason"`
		}
		if err := json.Unmarshal([]byte(out), &items); err != nil {
			t.Fatalf("unmarshal %q: %v", out, err)
		}
		if len(items) != 1 {
			t.Fatalf("items = %v, want 1", items)
		}
		got := items[0]
		if got.Name != "testorg/elsewhere" || got.Path != dropped {
			t.Errorf("item = %+v, want the dropped repo", got)
		}
		if got.Host != "git.example.com" || got.Remote == "" {
			t.Errorf("item = %+v, want the remote and host carried through", got)
		}
		if got.Reason != "host git.example.com not in index.hosts" {
			t.Errorf("reason = %q, want the host reason", got.Reason)
		}
	})
}

// runRepoCmd executes the repo command with args and returns stdout, with the
// package-level flags reset around the call so tests do not leak state. It is
// runCLI plus the repo flag reset; keep the cobra plumbing in one place.
func runRepoCmd(t *testing.T, args ...string) string {
	t.Helper()
	resetRepoFlags()
	t.Cleanup(resetRepoFlags)
	out, err := runCLI(t, args...)
	if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return out
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init in %s: %v\n%s", dir, err, out)
	}
}

func gitSetRemote(t *testing.T, dir, url string) {
	t.Helper()
	cmd := exec.Command("git", "remote", "add", "origin", url)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git remote add in %s: %v\n%s", dir, err, out)
	}
}

// writeDescriptor gives a repo a root catalog descriptor.
func writeDescriptor(t *testing.T, dir, name, owner, system string) {
	t.Helper()
	content := "apiVersion: catalog.mad01/v1alpha1\nkind: Component\nmetadata:\n  name: " + name +
		"\nspec:\n  owner: " + owner + "\n  system: " + system + "\n"
	if err := os.WriteFile(filepath.Join(dir, "service-info.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// setupCatalogRepos lays out three repos, two with descriptors, and points
// csl at them. It returns the temp root.
func setupCatalogRepos(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	for _, name := range []string{"code-search-local", "ralph", "dotfiles"} {
		repoDir := filepath.Join(tmp, name)
		if err := os.MkdirAll(repoDir, 0o755); err != nil {
			t.Fatal(err)
		}
		gitInit(t, repoDir)
		gitSetRemote(t, repoDir, "git@github.com:mad01/"+name+".git")
	}
	writeDescriptor(t, filepath.Join(tmp, "code-search-local"), "csl", "platform", "thismoon")
	writeDescriptor(t, filepath.Join(tmp, "ralph"), "ralph", "mad01", "thismoon")
	setupTestConfig(t, "dirs:\n  - "+tmp+"\n")
	return tmp
}

// TestRepoCatalogFilters pins the catalog flags on the non-interactive
// paths: they narrow --list, resolve to one path on their own when one repo
// is left, error like a query when none is, and compose with a query.
func TestRepoCatalogFilters(t *testing.T) {
	tmp := setupCatalogRepos(t)

	t.Run("list by system", func(t *testing.T) {
		out := runRepoCmd(t, "repo", "--list", "--system", "thismoon")
		for _, want := range []string{"mad01/code-search-local", "mad01/ralph"} {
			if !strings.Contains(out, want) {
				t.Errorf("output %q lacks %s", out, want)
			}
		}
		if strings.Contains(out, "mad01/dotfiles") {
			t.Errorf("output %q lists a repo with no descriptor", out)
		}
	})

	t.Run("owner alone resolves the single match", func(t *testing.T) {
		out := runRepoCmd(t, "repo", "--owner", "PLATFORM")
		if got := strings.TrimSpace(out); got != filepath.Join(tmp, "code-search-local") {
			t.Errorf("stdout = %q, want the platform repo's path", got)
		}
	})

	t.Run("component alone resolves the single match", func(t *testing.T) {
		out := runRepoCmd(t, "repo", "--component", "csl")
		if got := strings.TrimSpace(out); got != filepath.Join(tmp, "code-search-local") {
			t.Errorf("stdout = %q, want csl's checkout", got)
		}
	})

	t.Run("query composes with a filter", func(t *testing.T) {
		out := runRepoCmd(t, "repo", "ralph", "--system", "thismoon")
		if got := strings.TrimSpace(out); got != filepath.Join(tmp, "ralph") {
			t.Errorf("stdout = %q, want ralph's path", got)
		}
	})

	t.Run("no match names the filters", func(t *testing.T) {
		resetRepoFlags()
		t.Cleanup(resetRepoFlags)
		_, err := runCLI(t, "repo", "ralph", "--owner", "nobody")
		if err == nil {
			t.Fatal("expected an error")
		}
		if !strings.Contains(err.Error(), "no repos match query") ||
			!strings.Contains(err.Error(), "owner=nobody") {
			t.Errorf("error %q should say no repos match and name owner=nobody", err)
		}
	})

	t.Run("json carries the descriptor fields", func(t *testing.T) {
		out := runRepoCmd(t, "repo", "--json", "code-search")
		var rows []struct {
			Name      string `json:"name"`
			Component string `json:"component"`
			Owner     string `json:"owner"`
			System    string `json:"system"`
		}
		if err := json.Unmarshal([]byte(out), &rows); err != nil {
			t.Fatalf("unmarshal %q: %v", out, err)
		}
		if len(rows) != 1 {
			t.Fatalf("rows = %+v, want one", rows)
		}
		if r := rows[0]; r.Component != "csl" || r.Owner != "platform" || r.System != "thismoon" {
			t.Errorf("row = %+v, want csl/platform/thismoon", r)
		}
	})

	t.Run("plain list stays name and path", func(t *testing.T) {
		out := runRepoCmd(t, "repo", "--list", "code-search")
		want := "mad01/code-search-local\t" + filepath.Join(tmp, "code-search-local") + "\n"
		if out != want {
			t.Errorf("stdout = %q, want %q", out, want)
		}
	})
}

// TestPickerItemSegments pins the picker line: the component name appears
// only when it differs from the repo name, owner and system carry dim
// labels and their own colors, and a repo without a descriptor is just name
// and path.
func TestPickerItemSegments(t *testing.T) {
	plain := pickerItem(finder.Repo{Name: "mad01/dotfiles", Path: "/r/dotfiles"})
	if got := plain.String(); got != "mad01/dotfiles @ /r/dotfiles" {
		t.Errorf("plain line = %q", got)
	}

	full := pickerItem(finder.Repo{
		Name: "mad01/code-search-local", Path: "/r/csl",
		Catalog: &catalogspec.Component{Name: "csl", Owner: "platform", System: "thismoon"},
	})
	if got := full.String(); got != "mad01/code-search-local  csl  owner:platform  system:thismoon @ /r/csl" {
		t.Errorf("full line = %q", got)
	}
	colors := map[string]picker.Color{}
	for _, seg := range full {
		colors[seg.Text] = seg.Color
	}
	if colors["csl"] != picker.Cyan || colors["platform"] != picker.Magenta ||
		colors["thismoon"] != picker.Blue || colors["  owner:"] != picker.Dim {
		t.Errorf("segment colors = %v", colors)
	}

	same := pickerItem(finder.Repo{
		Name: "mad01/ralph@feature", Path: "/r/ralph",
		Catalog: &catalogspec.Component{Name: "Ralph", Owner: "mad01"},
	})
	if got := same.String(); got != "mad01/ralph@feature  owner:mad01 @ /r/ralph" {
		t.Errorf("worktree line = %q, want the component name folded into the repo name", got)
	}
}

// TestRepoCatalogFilterAloneNoMatchErrors pins the fix for an empty picker:
// catalog flags with no positional query and nothing left after filtering
// must error like a query does, not open a picker over nothing.
func TestRepoCatalogFilterAloneNoMatchErrors(t *testing.T) {
	setupCatalogRepos(t)
	resetRepoFlags()
	t.Cleanup(resetRepoFlags)
	_, err := runCLI(t, "repo", "--owner", "nobody")
	if err == nil {
		t.Fatal("expected an error, got nil (an empty picker would have opened)")
	}
	if !strings.Contains(err.Error(), `no repos match query "owner=nobody"`) {
		t.Errorf("error = %q, want the no-match text naming owner=nobody", err)
	}
}

// TestRepoQueryNoMatchKeepsBareName: the pre-catalog error text for a plain
// query is unchanged, since scripts may match on it.
func TestRepoQueryNoMatchKeepsBareName(t *testing.T) {
	setupCatalogRepos(t)
	resetRepoFlags()
	t.Cleanup(resetRepoFlags)
	_, err := runCLI(t, "repo", "nosuchrepo")
	if err == nil || !strings.Contains(err.Error(), `no repos match query "nosuchrepo"`) {
		t.Errorf("error = %v, want the bare-name no-match text", err)
	}
}
