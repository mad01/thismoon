package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFrom(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		wantVault string
		wantErr   string
	}{
		{
			name:      "absolute path",
			yaml:      "vault: /tmp/vault\n",
			wantVault: "/tmp/vault",
		},
		{
			name:      "tilde expands",
			yaml:      "vault: ~/vault\n",
			wantVault: filepath.Join(must(os.UserHomeDir), "vault"),
		},
		{
			name:    "missing vault",
			yaml:    "other: thing\n",
			wantErr: "vault field is required",
		},
		{
			name:    "invalid yaml",
			yaml:    "vault: [unclosed\n",
			wantErr: "parsing",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			if err := os.WriteFile(path, []byte(tc.yaml), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			cfg, err := LoadFrom(path)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("want error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want error containing %q, got %q", tc.wantErr, err.Error())
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

func TestLoadFromMissingFile(t *testing.T) {
	_, err := LoadFrom(filepath.Join(t.TempDir(), "nope.yaml"))
	if err == nil {
		t.Fatal("want error for missing file")
	}
}

func TestDefaultPath(t *testing.T) {
	p, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	if !strings.HasSuffix(p, filepath.Join(".config", "brain", "config.yaml")) {
		t.Errorf("unexpected path: %s", p)
	}
}

func must[T any](f func() (T, error)) T {
	v, err := f()
	if err != nil {
		panic(err)
	}
	return v
}
