// Package rules bundles the embedded Vale style pack, exposes rule
// metadata parsed from the YAML files, and runs the `vale` CLI to
// detect AI-writing patterns. Vale is the detection engine; this
// package is the Go-side adapter.
package rules

import (
	"bytes"
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
	return extractTree(packRoot, destDir)
}

// packRoot is the embedded directory that extracts to the cache root.
const packRoot = "vale"

// extractTree copies the embedded tree under root into destDir, so
// <root>/a/b lands at <destDir>/a/b. Unchanged files are left alone.
func extractTree(root, destDir string) error {
	return fs.WalkDir(valeFS, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, root), "/")
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
		return writeFileIfChanged(target, data)
	})
}

// writeFileIfChanged writes data to target unless target already holds it.
// The data goes to a temp file beside target that is renamed into place, so
// a vale process running at the same time never reads a half-written rule.
// The temp name has no .yml suffix, so vale never loads it as a rule.
func writeFileIfChanged(target string, data []byte) error {
	if existing, err := os.ReadFile(target); err == nil && bytes.Equal(existing, data) {
		return nil
	}
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp in %s: %w", dir, err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write %s: %w", tmp.Name(), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmp.Name(), err)
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return fmt.Errorf("chmod %s: %w", tmp.Name(), err)
	}
	if err := os.Rename(tmp.Name(), target); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}
	return nil
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
