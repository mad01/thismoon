package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/brain"
	"github.com/mad01/thismoon/services/brain/internal/testhome"
)

func TestLoad(t *testing.T) {
	home := testhome.Isolate(t)
	// vaults lists name/path/default/shared per vault, in config order.
	type v = Vault
	tests := []struct {
		name        string
		yaml        string
		want        []Vault
		wantInvalid string // substring of the error; empty means valid
	}{
		{
			name: "legacy path loads as the default vault",
			yaml: "vault: /tmp/vault\n",
			want: []v{{Name: "default", Path: "/tmp/vault", Default: true}},
		},
		{
			name: "legacy tilde expands",
			yaml: "vault: ~/vault\n",
			want: []v{{Name: "default", Path: filepath.Join(home, "vault"), Default: true}},
		},
		{
			name: "list defaults to the first non-shared vault",
			yaml: "vaults:\n  - {name: team, path: /t, shared: true}\n" +
				"  - {name: mine, path: ~/m}\n  - {name: side, path: /s}\n",
			want: []v{
				{Name: "team", Path: "/t", Shared: true},
				{Name: "mine", Path: filepath.Join(home, "m"), Default: true},
				{Name: "side", Path: "/s"},
			},
		},
		{
			name: "explicit default wins",
			yaml: "vaults:\n  - {name: mine, path: /m}\n  - {name: side, path: /s, default: true}\n",
			want: []v{{Name: "mine", Path: "/m"}, {Name: "side", Path: "/s", Default: true}},
		},
		{
			name: "shared vaults alone have no default",
			yaml: "vaults:\n  - {name: team, path: /t, shared: true}\n",
			want: []v{{Name: "team", Path: "/t", Shared: true}},
		},
		{name: "missing vault", yaml: "other: thing\n", wantInvalid: "vault is required"},
		{name: "invalid yaml", yaml: "vault: [unclosed\n", wantInvalid: "yaml"},
		{name: "not a mapping", yaml: "- a\n- b\n", wantInvalid: "cannot unmarshal"},
		{
			name:        "vault and vaults",
			yaml:        "vault: /a\nvaults:\n  - {name: b, path: /b}\n",
			wantInvalid: "not both",
		},
		{
			name:        "uppercase name",
			yaml:        "vaults:\n  - {name: Team, path: /t}\n",
			wantInvalid: "lowercase slug",
		},
		{
			name:        "name with a slash",
			yaml:        "vaults:\n  - {name: a/b, path: /t}\n",
			wantInvalid: "lowercase slug",
		},
		{
			name:        "missing name",
			yaml:        "vaults:\n  - {path: /t}\n",
			wantInvalid: "needs a name",
		},
		{
			name:        "reserved name",
			yaml:        "vaults:\n  - {name: id, path: /t}\n",
			wantInvalid: "reserved",
		},
		{
			name:        "duplicate name",
			yaml:        "vaults:\n  - {name: a, path: /a}\n  - {name: a, path: /b}\n",
			wantInvalid: "used twice",
		},
		{
			name:        "duplicate path",
			yaml:        "vaults:\n  - {name: a, path: /a}\n  - {name: b, path: /a/}\n",
			wantInvalid: "share the path",
		},
		{
			name:        "missing path",
			yaml:        "vaults:\n  - {name: a}\n",
			wantInvalid: "path is required",
		},
		{
			name: "two defaults",
			yaml: "vaults:\n  - {name: a, path: /a, default: true}\n" +
				"  - {name: b, path: /b, default: true}\n",
			wantInvalid: "both default",
		},
		{
			name:        "shared default",
			yaml:        "vaults:\n  - {name: a, path: /a, shared: true, default: true}\n",
			wantInvalid: "cannot be the default",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tc.yaml), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			cfg, err := Load(path)
			if tc.wantInvalid != "" {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("err = %v, want ErrInvalid", err)
				}
				if !strings.Contains(err.Error(), path) ||
					!strings.Contains(err.Error(), tc.wantInvalid) {
					t.Errorf("error %q does not name the file and %q", err, tc.wantInvalid)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !slices.Equal(cfg.Vaults, tc.want) {
				t.Errorf("Vaults = %+v, want %+v", cfg.Vaults, tc.want)
			}
		})
	}
}

func TestLoadMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope.yaml")
	_, err := Load(path)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("err = %v, want fs.ErrNotExist", err)
	}
	if errors.Is(err, ErrInvalid) {
		t.Errorf("a missing file is not an invalid one: %v", err)
	}
}

func TestSemanticDefaults(t *testing.T) {
	home := testhome.Isolate(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("vault: /v\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s := cfg.Semantic
	if s.OllamaURL != brain.DefaultOllamaURL || s.EmbedModel != brain.DefaultEmbedModel ||
		s.ChatModel != brain.DefaultChatModel || s.ChunkSize != brain.DefaultChunkSize ||
		s.ChunkOverlap != brain.DefaultChunkOverlap {
		t.Errorf("defaults not applied: %+v", s)
	}
	dir, err := s.ResolvedIndexDir("default")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".local", "state", "brain", "semantic", "default"); dir != want {
		t.Errorf("semantic dir = %q, want %q", dir, want)
	}
}

func TestSemanticIndexDirOverride(t *testing.T) {
	home := testhome.Isolate(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := "vault: /v\nsemantic:\n  index_dir: ~/vectors\n"
	if err := os.WriteFile(path, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := loaded.Semantic.ResolvedIndexDir("default")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "vectors", "default"); dir != want {
		t.Errorf("semantic dir = %q, want %q", dir, want)
	}
}
