// Package rules bundles the embedded Vale style pack, exposes rule
// metadata parsed from the YAML files, and runs the `vale` CLI to
// detect AI-writing patterns. Vale is the detection engine; this
// package is the Go-side adapter.
package rules

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

//go:embed all:vale
var valeFS embed.FS

// ensureOnce serialises pack extraction per destination directory. Without
// this, two concurrent Detect calls on a fresh cache race on mkdir+write.
// One sync.Once per dest so different caches (e.g. tests) don't block.
var ensureOnce sync.Map // map[string]*sync.Once

// EnsurePack writes the embedded Vale pack into destDir, creating any
// missing directories. Files are only rewritten when the content differs
// so it's cheap to call on every MCP invocation. destDir typically ends
// up being ~/.cache/humanizer/vale.
//
// Safe to call concurrently — extraction for a given destDir runs at
// most once per process.
//
// After extraction the caller should invoke vale with
// --config=<destDir>/.vale.ini to point at the extracted pack.
func EnsurePack(destDir string) error {
	key, err := filepath.Abs(destDir)
	if err != nil {
		key = destDir
	}
	onceI, _ := ensureOnce.LoadOrStore(key, &sync.Once{})
	var extractErr error
	onceI.(*sync.Once).Do(func() {
		extractErr = extractPack(destDir)
	})
	return extractErr
}

func extractPack(destDir string) error {
	return fs.WalkDir(valeFS, "vale", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Strip the "vale/" prefix so files land at <destDir>/<rest>.
		rel := strings.TrimPrefix(p, "vale")
		rel = strings.TrimPrefix(rel, "/")
		target := filepath.Join(destDir, rel)
		if d.IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", target, err)
			}
			return nil
		}
		data, err := valeFS.ReadFile(p)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", p, err)
		}
		if existing, err := os.ReadFile(target); err == nil && string(existing) == string(data) {
			return nil // unchanged
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(target), err)
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", target, err)
		}
		return nil
	})
}

// DefaultCacheDir returns the canonical extraction target: the first of
//   - $HUMANIZER_CACHE_DIR
//   - $XDG_CACHE_HOME/humanizer/vale
//   - ~/.cache/humanizer/vale
//
// Env-var values with a leading ~ are expanded against the user's home dir.
func DefaultCacheDir() (string, error) {
	if v := os.Getenv("HUMANIZER_CACHE_DIR"); v != "" {
		expanded, err := expandHome(v)
		if err != nil {
			return "", err
		}
		return filepath.Join(expanded, "vale"), nil
	}
	if v := os.Getenv("XDG_CACHE_HOME"); v != "" {
		expanded, err := expandHome(v)
		if err != nil {
			return "", err
		}
		return filepath.Join(expanded, "humanizer", "vale"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home dir: %w", err)
	}
	return filepath.Join(home, ".cache", "humanizer", "vale"), nil
}

// expandHome expands a leading "~" or "~/..." to the user's home dir.
// Other paths are returned unchanged.
func expandHome(p string) (string, error) {
	if !strings.HasPrefix(p, "~") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("expand ~ in %q: %w", p, err)
	}
	if p == "~" {
		return home, nil
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:]), nil
	}
	return p, nil // "~user" not supported; return as-is
}
