package discover

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/deps/internal/store"
)

// miseFixture copies testdata/mise (a trimmed real mise.lock and install tree)
// into a temp dir, adds the alias links mise makes beside each version, and
// returns a Mise over the copy. Each test may break its own copy.
func miseFixture(t *testing.T) Mise {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("testdata", "mise"))); err != nil {
		t.Fatal(err)
	}
	installs := filepath.Join(dir, "installs")
	for link, target := range map[string]string{
		"npm-yarn/latest": "1.22.22",
		"npm-yarn/1":      "1.22.22",
		"pipx-httpie/3.2": "3.2.4",
	} {
		if err := os.Symlink(target, filepath.Join(installs, link)); err != nil {
			t.Fatal(err)
		}
	}
	return Mise{LockPath: filepath.Join(dir, "mise.lock"), InstallRoot: installs}
}

// summary renders each dependency as "label name@version", with " direct"
// when it is the tool itself, sorted.
func summary(deps []store.Dependency) []string {
	out := make([]string, 0, len(deps))
	for _, d := range deps {
		s := fmt.Sprintf("%s %s@%s", d.Repo, d.Name, d.Version)
		if d.Direct {
			s += " direct"
		}
		out = append(out, s)
	}
	slices.Sort(out)
	return out
}

func TestMiseDiscoverReadsTheLockedTrees(t *testing.T) {
	m := miseFixture(t)
	deps, errs := m.Discover()
	if len(errs) != 0 {
		t.Fatalf("discovery errors = %+v, want none", errs)
	}
	// pipx:tox is pinned but not installed here, and gemini-cli is installed
	// but no longer pinned: neither shows up.
	want := []string{
		"mise:npm:@anthropic-ai/sandbox-runtime @anthropic-ai/sandbox-runtime@0.0.81 direct",
		"mise:npm:@anthropic-ai/sandbox-runtime @pondwader/socks5-server@1.0.10",
		"mise:npm:@anthropic-ai/sandbox-runtime commander@12.1.0",
		"mise:npm:@anthropic-ai/sandbox-runtime node-forge@1.4.0",
		"mise:npm:@anthropic-ai/sandbox-runtime zod@3.25.76",
		"mise:npm:yarn yarn@1.22.22 direct",
		"mise:pipx:httpie httpie@3.2.4 direct",
		"mise:pipx:httpie markdown-it-py@4.2.0",
		"mise:pipx:httpie pysocks@1.7.1",
		"mise:pipx:httpie requests@2.34.2",
	}
	if got := summary(deps); !slices.Equal(got, want) {
		t.Errorf("discovered:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for _, d := range deps {
		wantEco := "npm"
		if strings.HasPrefix(d.Repo, "mise:pipx:") {
			wantEco = "PyPI"
		}
		if d.Ecosystem != wantEco || !d.Imported || d.ManifestPath == "" {
			t.Errorf(
				"%s = %+v, want ecosystem %s, imported, with a manifest path",
				d.Name,
				d,
				wantEco,
			)
		}
	}
}

// TestMiseDiscoverReadsThePipxLayout: without uv, mise's pipx backend runs
// pipx with PIPX_HOME in the version's directory, so the venv sits under
// venvs/ and pip under the shared/ venv every tool venv imports.
func TestMiseDiscoverReadsThePipxLayout(t *testing.T) {
	m := miseFixture(t)
	version := filepath.Join(m.InstallRoot, "pipx-httpie", "3.2.4")
	if err := os.MkdirAll(filepath.Join(version, "venvs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(
		filepath.Join(version, "httpie"),
		filepath.Join(version, "venvs", "httpie"),
	); err != nil {
		t.Fatal(err)
	}
	shared := filepath.Join(version, "shared", "lib", "python3.14", "site-packages")
	if err := os.MkdirAll(filepath.Join(shared, "pip-25.2.dist-info"), 0o755); err != nil {
		t.Fatal(err)
	}
	deps, errs := m.Discover()
	if len(errs) != 0 {
		t.Fatalf("discovery errors = %+v, want none", errs)
	}
	var got []string
	for _, s := range summary(deps) {
		if strings.HasPrefix(s, "mise:pipx:httpie ") {
			got = append(got, s)
		}
	}
	want := []string{
		"mise:pipx:httpie httpie@3.2.4 direct",
		"mise:pipx:httpie markdown-it-py@4.2.0",
		"mise:pipx:httpie pip@25.2",
		"mise:pipx:httpie pysocks@1.7.1",
		"mise:pipx:httpie requests@2.34.2",
	}
	if !slices.Equal(got, want) {
		t.Errorf("discovered:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestMiseDiscoverWithoutMise: a machine without mise has no lock or no
// installs directory, and that is no tools, not a discovery error.
func TestMiseDiscoverWithoutMise(t *testing.T) {
	fixture := miseFixture(t)
	absent := filepath.Join(t.TempDir(), "absent")
	for name, m := range map[string]Mise{
		"no lock configured": {InstallRoot: fixture.InstallRoot},
		"lock missing":       {LockPath: absent, InstallRoot: fixture.InstallRoot},
		"installs missing":   {LockPath: fixture.LockPath, InstallRoot: absent},
	} {
		t.Run(name, func(t *testing.T) {
			deps, errs := m.Discover()
			if len(deps) != 0 || len(errs) != 0 {
				t.Errorf("Discover() = %d deps, errors %+v; want none of either", len(deps), errs)
			}
		})
	}
}

// TestMiseDiscoverReportsUnreadableTrees is the silent-failure guard: every way
// a pinned tool's tree can fail to read becomes a DiscoveryError under the
// tool's label, and the other tools are still discovered.
func TestMiseDiscoverReportsUnreadableTrees(t *testing.T) {
	tests := []struct {
		name   string
		label  string
		eco    string
		want   string // substring of the error
		damage func(t *testing.T, m Mise)
	}{
		{
			name: "install lags the lock", label: "mise:npm:yarn", eco: "npm",
			want: "mise.lock pins 1.22.23, but it is not installed",
			damage: func(t *testing.T, m Mise) {
				rewrite(t, m.LockPath, `version = "1.22.22"`, `version = "1.22.23"`)
			},
		},
		{
			name: "older lockfile format", label: "mise:npm:yarn", eco: "npm",
			want: `lockfileVersion "6.0", want 9`,
			damage: func(t *testing.T, m Mise) {
				lock := filepath.Join(m.InstallRoot, "npm-yarn", "1.22.22", "aube-lock.yaml")
				rewrite(t, lock, "lockfileVersion: '9.0'", "lockfileVersion: '6.0'")
			},
		},
		{
			name: "aube-lock.yaml missing", label: "mise:npm:yarn", eco: "npm",
			want: "aube-lock.yaml",
			damage: func(t *testing.T, m Mise) {
				remove(t, filepath.Join(m.InstallRoot, "npm-yarn", "1.22.22", "aube-lock.yaml"))
			},
		},
		{
			name: "no site-packages", label: "mise:pipx:httpie", eco: "PyPI",
			want: "no site-packages under",
			damage: func(t *testing.T, m Mise) {
				remove(t, filepath.Join(m.InstallRoot, "pipx-httpie", "3.2.4", "httpie", "lib"))
			},
		},
		{
			// A sandbox denial looks the same: the directory stats, but no
			// read inside it succeeds.
			name: "install directory unreadable", label: "mise:pipx:httpie", eco: "PyPI",
			want: "permission denied",
			damage: func(t *testing.T, m Mise) {
				lockOut(t, filepath.Join(m.InstallRoot, "pipx-httpie", "3.2.4"))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := miseFixture(t)
			tt.damage(t, m)
			deps, errs := m.Discover()
			if len(errs) != 1 || errs[0].Repo != tt.label || errs[0].Ecosystem != tt.eco ||
				!strings.Contains(errs[0].Error, tt.want) {
				t.Fatalf("errors = %+v, want one %s error for %s containing %q",
					errs, tt.eco, tt.label, tt.want)
			}
			for _, d := range deps {
				if d.Repo == tt.label {
					t.Errorf("broken tool %s still reported %s@%s", tt.label, d.Name, d.Version)
				}
			}
			if len(deps) == 0 {
				t.Error("the other tools were lost along with the broken one")
			}
		})
	}
}

// TestMiseDiscoverReportsAnUnparsableLock: a lock that exists but does not
// parse hides every tool, so it is one error under the lock's own path.
func TestMiseDiscoverReportsAnUnparsableLock(t *testing.T) {
	m := miseFixture(t)
	if err := os.WriteFile(m.LockPath, []byte("[[tools.act]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deps, errs := m.Discover()
	if len(deps) != 0 {
		t.Errorf("deps = %d, want none from an unparsable lock", len(deps))
	}
	want := store.DiscoveryError{Repo: m.LockPath, Ecosystem: MiseEcosystem}
	if len(errs) != 1 || errs[0].Repo != want.Repo || errs[0].Ecosystem != want.Ecosystem ||
		!strings.Contains(errs[0].Error, "parse "+m.LockPath) {
		t.Fatalf("errors = %+v, want one %s error under %s", errs, MiseEcosystem, m.LockPath)
	}
}

// TestMiseDirName pins the install directory names mise 2026.9.1 gives a tool,
// each checked against `mise where` on a scratch data directory.
func TestMiseDirName(t *testing.T) {
	tests := map[string]string{
		"npm:@googleworkspace/cli":          "npm-googleworkspace-cli",
		"npm:@anthropic-ai/sandbox-runtime": "npm-anthropic-ai-sandbox-runtime",
		"npm:yarn":                          "npm-yarn",
		"pipx:httpie":                       "pipx-httpie",
		"pipx:Foo_Bar.Baz":                  "pipx-foo-bar-baz",
		"pipx:fooBar":                       "pipx-foo-bar",
		"pipx:ABC":                          "pipx-abc",
		"pipx:aB1Cd":                        "pipx-a-b1-cd",
		"npm:a..b_C":                        "npm-a-b-c",
		"npm:@Scope/HTTPServer2go":          "npm-scope-http-server2go",
	}
	for tool, want := range tests {
		if got := miseDirName(tool); got != want {
			t.Errorf("miseDirName(%q) = %q, want %q", tool, got, want)
		}
	}
}

func TestSplitPackageKey(t *testing.T) {
	tests := []struct {
		key, name, version string
		ok                 bool
	}{
		{"left-pad@1.3.0", "left-pad", "1.3.0", true},
		{"@scope/name@1.0.0", "@scope/name", "1.0.0", true},
		{"react-dom@18.2.0(react@18.2.0)", "react-dom", "18.2.0", true},
		{"foo@https://example.com/foo-1.0.0.tgz", "", "", false},
		{"foo@file:../foo", "", "", false},
		{"@scope/name", "", "", false},
		{"name@", "", "", false},
	}
	for _, tt := range tests {
		name, version, ok := splitPackageKey(tt.key)
		if name != tt.name || version != tt.version || ok != tt.ok {
			t.Errorf("splitPackageKey(%q) = %q, %q, %v; want %q, %q, %v",
				tt.key, name, version, ok, tt.name, tt.version, tt.ok)
		}
	}
}

// rewrite replaces the one occurrence of old in the file at path.
func rewrite(t *testing.T, path, old, replacement string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(raw), old) != 1 {
		t.Fatalf("%s holds %q %d times, want once", path, old, strings.Count(string(raw), old))
	}
	body := strings.Replace(string(raw), old, replacement, 1)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// lockOut removes every permission from the directory at path until the test
// ends. Root reads through that, so the test is skipped as root.
func lockOut(t *testing.T, path string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(path, 0o755); err != nil {
			t.Error(err)
		}
	})
}

// remove deletes path and everything under it.
func remove(t *testing.T, path string) {
	t.Helper()
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
}
