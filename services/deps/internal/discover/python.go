package discover

// Python is the plug point for PyPI discovery (requirements.txt / pyproject.toml
// / poetry.lock). No Python manifest exists across the scanned repos today
// (speak's mlx-audio venv is created at runtime, not declared in-tree), so this
// returns nothing. When a manifest lands, parse it here and emit Packages with
// Ecosystem "PyPI" — the OSV check and the rest of the pipeline already support
// it.
type Python struct{}

func (Python) Name() string { return "PyPI" }

func (Python) Discover(repoRoot string, opts Options) ([]Package, error) {
	return nil, nil
}
