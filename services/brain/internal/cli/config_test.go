package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/brain"
	"github.com/mad01/thismoon/services/brain/internal/testhome"
)

// writeConfig writes a config naming vaultRoot at path.
func writeConfig(t *testing.T, path, vaultRoot, extra string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("vault: "+vaultRoot+"\n"+extra), 0o644); err != nil {
		t.Fatal(err)
	}
}

// markedVault makes a vault holding one note, notes/<marker>.md.
func markedVault(t *testing.T, marker string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	note := "---\ntitle: " + marker + "\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(root, "notes", marker+".md"), []byte(note), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestConfigPrecedence(t *testing.T) {
	tests := []struct {
		name string
		// setup writes the configs and returns extra args plus the vault
		// marker brain must read from.
		setup func(t *testing.T, home string) ([]string, string)
	}{
		{
			name: "default path",
			setup: func(t *testing.T, home string) ([]string, string) {
				writeConfig(
					t,
					filepath.Join(home, ".config", "brain", "config.yaml"),
					markedVault(t, "dflt"),
					"",
				)
				return nil, "dflt"
			},
		},
		{
			name: "XDG_CONFIG_HOME moves the default",
			setup: func(t *testing.T, home string) ([]string, string) {
				writeConfig(
					t,
					filepath.Join(home, ".config", "brain", "config.yaml"),
					markedVault(t, "dflt"),
					"",
				)
				xdg := filepath.Join(home, "xdg")
				t.Setenv("XDG_CONFIG_HOME", xdg)
				writeConfig(
					t,
					filepath.Join(xdg, "brain", "config.yaml"),
					markedVault(t, "xdg"),
					"",
				)
				return nil, "xdg"
			},
		},
		{
			name: "env beats default, with ~ expanded",
			setup: func(t *testing.T, home string) ([]string, string) {
				writeConfig(
					t,
					filepath.Join(home, ".config", "brain", "config.yaml"),
					markedVault(t, "dflt"),
					"",
				)
				writeConfig(t, filepath.Join(home, "env.yaml"), markedVault(t, "env"), "")
				t.Setenv(brain.ConfigEnv, "~/env.yaml")
				return nil, "env"
			},
		},
		{
			name: "flag beats env",
			setup: func(t *testing.T, home string) ([]string, string) {
				writeConfig(t, filepath.Join(home, "env.yaml"), markedVault(t, "env"), "")
				t.Setenv(brain.ConfigEnv, filepath.Join(home, "env.yaml"))
				flag := filepath.Join(home, "flag.yaml")
				writeConfig(t, flag, markedVault(t, "flag"), "")
				return []string{"--config", flag}, "flag"
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			home := testhome.Isolate(t)
			args, marker := tc.setup(t, home)
			got := runBrain(t, "", append(args, "read", "notes/"+marker+".md")...)
			if got.code != exitOK {
				t.Fatalf("read from the %s vault: exit %d, stderr %q", marker, got.code, got.stderr)
			}
		})
	}
}

func TestInvalidConfigExitsUsage(t *testing.T) {
	tests := []struct{ name, content string }{
		{"unparseable", "vault: [unclosed\n"},
		{"no vault", "semantic:\n  enabled: true\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			home := testhome.Isolate(t)
			path := filepath.Join(home, "bad.yaml")
			if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			got := runBrain(t, "", "--config", path, "list", "--tier", "notes", "-o", "json")
			if got.code != exitUsage {
				t.Fatalf("exit %d, want %d; stderr %q", got.code, exitUsage, got.stderr)
			}
			if !strings.Contains(got.stderr, path) {
				t.Errorf("stderr %q does not name the config file", got.stderr)
			}
			if env := decodeStrict[errorEnvelope](t, got.stdout); env.Error.Code != "invalid_config" {
				t.Errorf("code = %q, want invalid_config", env.Error.Code)
			}
		})
	}
}

// TestIsolatedByConfigAndStateHome pins the eval isolation contract: with
// BRAIN_CONFIG and XDG_STATE_HOME set, brain writes only its state under
// XDG_STATE_HOME and notes in the vault the config names. HOME carries a
// legacy index, as a real machine does, and must come out untouched.
func TestIsolatedByConfigAndStateHome(t *testing.T) {
	ollama := fakeOllama(t)
	home := testhome.Isolate(t)
	legacyProbe := filepath.Join(home, ".config", "brain", brain.LegacyStateProbe)
	if err := os.MkdirAll(filepath.Dir(legacyProbe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyProbe, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	homeBefore := snapshot(t, home)

	tmp := t.TempDir()
	vaultRoot := filepath.Join(tmp, "vault")
	seedVault(t, vaultRoot)
	cfgPath := filepath.Join(tmp, "config.yaml")
	writeConfig(t, cfgPath, vaultRoot, "semantic:\n  enabled: true\n  ollama_url: "+ollama.URL+"\n")
	stateHome := filepath.Join(tmp, "state")
	tmpdir := filepath.Join(tmp, "tmpdir")
	if err := os.Mkdir(tmpdir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(brain.ConfigEnv, cfgPath)
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("TMPDIR", tmpdir)

	// The first doctor runs before any index exists and must leave none.
	if got := runBrain(t, "", "doctor"); got.code != exitError {
		t.Fatalf("doctor before index: exit %d, want %d", got.code, exitError)
	}
	if _, err := os.Stat(stateHome); !os.IsNotExist(err) {
		t.Fatalf("doctor created %s: %v", stateHome, err)
	}
	steps := [][]string{
		{"index"},
		{"search", "kubernetes"},
		{"count", "kubernetes"},
		{"semantic-search", "kubernetes"},
		{"hybrid-search", "kubernetes"},
		{"list", "--tier", "notes"},
		{"read", "notes/alpha.md"},
		{"peek", "notes/alpha.md"},
		{"backlinks", "notes/alpha.md"},
		{"write", "--title", "Isolated", "--body", "new note"},
		{"edit", "inbox/isolated.md", "--set", "owner=kim"},
		{"promote", "inbox/isolated.md"},
		{"index"},
		{"doctor"},
	}
	for _, args := range steps {
		if got := runBrain(t, "", args...); got.code != exitOK {
			t.Fatalf("brain %v: exit %d, stderr %q", args, got.code, got.stderr)
		}
	}

	if after := snapshot(t, home); !mapsEqual(homeBefore, after) {
		t.Errorf("HOME changed:\nbefore %v\nafter  %v", homeBefore, after)
	}
	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{"vault": true, "config.yaml": true, "state": true, "tmpdir": true}
	for _, e := range entries {
		if !allowed[e.Name()] {
			t.Errorf(
				"brain wrote %s outside the config, state and vault",
				filepath.Join(tmp, e.Name()),
			)
		}
	}
	if left, _ := os.ReadDir(tmpdir); len(left) != 0 {
		t.Errorf("brain left %d entries in TMPDIR", len(left))
	}
	for _, dir := range []string{"index", "semantic"} {
		if _, err := os.Stat(filepath.Join(stateHome, "brain", dir)); err != nil {
			t.Errorf("state %s: %v", dir, err)
		}
	}
}

// snapshot maps every path under root to its size and modification time.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		files[path] = info.ModTime().Format(time.RFC3339Nano) + " " + info.Mode().String()
		if !d.IsDir() {
			files[path] += " " + strconv.FormatInt(info.Size(), 10)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
