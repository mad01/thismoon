package catalog

import (
	"context"
	"os"
	"path/filepath"
)

// DefaultRegistryPath returns the conventional registry location,
// ~/.config/catalog/registry.yaml. It falls back to a relative path if the
// home directory cannot be determined.
func DefaultRegistryPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "catalog", "registry.yaml")
	}
	return filepath.Join(home, ".config", "catalog", "registry.yaml")
}

// Load reads the registry at registryPath, scans every source for
// service-info.yaml entities, and returns a built Catalog alongside the
// registry. The global-uniqueness rule is NOT enforced here so the UI can
// still render a catalog that happens to contain a collision; call
// CheckUnique explicitly (e.g. in `catalog validate`) to enforce it.
func Load(ctx context.Context, registryPath string) (*Catalog, *Registry, error) {
	reg, err := LoadRegistry(registryPath)
	if err != nil {
		return nil, nil, err
	}
	entities, err := ScanPaths(ctx, reg.Paths())
	if err != nil {
		return nil, nil, err
	}
	return NewCatalog(entities), reg, nil
}
