package catalog

import "context"

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
	roots, err := reg.Paths()
	if err != nil {
		return nil, nil, err
	}
	entities, err := ScanPaths(ctx, roots)
	if err != nil {
		return nil, nil, err
	}
	return NewCatalog(entities), reg, nil
}
