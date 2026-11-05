package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/brain"
	"github.com/mad01/thismoon/services/brain/internal/testhome"
)

func TestLoad(t *testing.T) {
	home := testhome.Isolate(t)
	tests := []struct {
		name        string
		yaml        string
		wantVault   string
		wantInvalid bool
	}{
		{name: "absolute path", yaml: "vault: /tmp/vault\n", wantVault: "/tmp/vault"},
		{name: "tilde expands", yaml: "vault: ~/vault\n", wantVault: filepath.Join(home, "vault")},
		{name: "missing vault", yaml: "other: thing\n", wantInvalid: true},
		{name: "invalid yaml", yaml: "vault: [unclosed\n", wantInvalid: true},
		{name: "not a mapping", yaml: "- a\n- b\n", wantInvalid: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tc.yaml), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			cfg, err := Load(path)
			if tc.wantInvalid {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("err = %v, want ErrInvalid", err)
				}
				if !strings.Contains(err.Error(), path) {
					t.Errorf("error %q does not name the file", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.Vault != tc.wantVault {
				t.Errorf("Vault = %q, want %q", cfg.Vault, tc.wantVault)
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
	dir, err := s.ResolvedIndexDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".local", "state", "brain", "semantic"); dir != want {
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
	dir, err := loaded.Semantic.ResolvedIndexDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "vectors"); dir != want {
		t.Errorf("semantic dir = %q, want %q", dir, want)
	}
}
