package cli

import (
	"path/filepath"
	"testing"
)

// TestPersistentPreRunExpandsEveryPath covers the three path flags together:
// env vars reach a launchd-supervised deps without shell expansion, so a
// literal "~" in any of them has to be resolved before a subcommand opens the
// file.
func TestPersistentPreRunExpandsEveryPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	savedWorkdir, savedRegistry, savedConfig := flagWorkdir, flagRegistry, flagConfig
	t.Cleanup(func() {
		flagWorkdir, flagRegistry, flagConfig = savedWorkdir, savedRegistry, savedConfig
	})
	flagWorkdir = "~/.local/share/deps"
	flagRegistry = "~/.config/catalog/registry.yaml"
	flagConfig = "/etc/deps/config.toml"

	if err := rootCmd.PersistentPreRunE(rootCmd, nil); err != nil {
		t.Fatalf("PersistentPreRunE: %v", err)
	}
	if want := filepath.Join(home, ".local", "share", "deps"); flagWorkdir != want {
		t.Errorf("flagWorkdir = %q, want %q", flagWorkdir, want)
	}
	if want := filepath.Join(home, ".config", "catalog", "registry.yaml"); flagRegistry != want {
		t.Errorf("flagRegistry = %q, want %q", flagRegistry, want)
	}
	if flagConfig != "/etc/deps/config.toml" {
		t.Errorf("flagConfig = %q, want an absolute path left alone", flagConfig)
	}
}

// TestDefaultPathsHonorXDG pins where the two config-directory files resolve.
// The registry lives under catalog's directory, not deps's: deps reads the
// file catalog owns, so both have to move together when XDG_CONFIG_HOME does.
func TestDefaultPathsHonorXDG(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	if got, want := defaultConfigPath(), filepath.Join(xdg, "deps", "config.toml"); got != want {
		t.Errorf("defaultConfigPath() = %q, want %q", got, want)
	}
	if got, want := defaultRegistryPath(), filepath.Join(xdg, "catalog", "registry.yaml"); got != want {
		t.Errorf("defaultRegistryPath() = %q, want %q", got, want)
	}

	t.Setenv("XDG_CONFIG_HOME", "")
	if got, want := defaultConfigPath(), filepath.Join(home, ".config", "deps", "config.toml"); got != want {
		t.Errorf("defaultConfigPath() = %q, want %q", got, want)
	}
}
