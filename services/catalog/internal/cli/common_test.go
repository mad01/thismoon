package cli

import (
	"path/filepath"
	"testing"
)

// TestPersistentPreRunExpandsRegistryTilde is the crash-loop guard: a
// --registry or CATALOG_REGISTRY value handed over by a launchd agent never
// went through a shell, so it arrives with a literal "~". Before the hook
// existed, only the compiled default was expanded and such a value reached
// os.ReadFile unchanged — failing under the service manager while the same
// command worked in a terminal.
func TestPersistentPreRunExpandsRegistryTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	saved := registryPath
	t.Cleanup(func() { registryPath = saved })
	registryPath = "~/.config/catalog/registry.yaml"

	if err := rootCmd.PersistentPreRunE(rootCmd, nil); err != nil {
		t.Fatalf("PersistentPreRunE: %v", err)
	}
	want := filepath.Join(home, ".config", "catalog", "registry.yaml")
	if registryPath != want {
		t.Errorf("registryPath = %q, want %q", registryPath, want)
	}
}

// TestPersistentPreRunLeavesAbsolutePathsAlone keeps the hook from touching a
// path that needs no expansion.
func TestPersistentPreRunLeavesAbsolutePathsAlone(t *testing.T) {
	saved := registryPath
	t.Cleanup(func() { registryPath = saved })
	registryPath = "/etc/catalog/registry.yaml"

	if err := rootCmd.PersistentPreRunE(rootCmd, nil); err != nil {
		t.Fatalf("PersistentPreRunE: %v", err)
	}
	if registryPath != "/etc/catalog/registry.yaml" {
		t.Errorf("registryPath = %q, want it unchanged", registryPath)
	}
}

// TestDefaultRegistryPathHonorsXDG pins the config-directory rule: an absolute
// XDG_CONFIG_HOME moves the registry, and everything else falls back to
// ~/.config/catalog.
func TestDefaultRegistryPathHonorsXDG(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	if got, want := defaultRegistryPath(), filepath.Join(xdg, "catalog", "registry.yaml"); got != want {
		t.Errorf("defaultRegistryPath() = %q, want %q", got, want)
	}

	t.Setenv("XDG_CONFIG_HOME", "")
	if got, want := defaultRegistryPath(), filepath.Join(home, ".config", "catalog", "registry.yaml"); got != want {
		t.Errorf("defaultRegistryPath() = %q, want %q", got, want)
	}
}
