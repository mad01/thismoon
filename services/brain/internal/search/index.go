package search

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const stateFileName = "state.json"

// skipDirs are directory names (at any depth) that the walker never descends
// into. Keep in sync with the vault layout — we only want markdown notes.
var skipDirs = map[string]bool{
	".git":         true,
	".obsidian":    true,
	".trash":       true,
	"attachments":  true,
	"node_modules": true,
}

// FileState tracks a single indexed file's size and mtime. Combined across the
// vault these tuples form the fingerprint used to detect staleness.
type FileState struct {
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mtime"`
}

// IndexState holds the indexed state of the vault.
//
// Two fingerprints are tracked so the literal (zoekt) and semantic
// (chromem-go) indexes can be re-checked independently against the same
// vault walk: editing one note may need a zoekt rebuild and a re-embed of
// just that note, but zoekt's coarse-grained shard rebuild and chromem's
// per-chunk upsert run on different schedules.
type IndexState struct {
	mu sync.RWMutex
	// Fingerprint is the literal-index fingerprint. Mismatch → zoekt rebuild.
	Fingerprint string `json:"fingerprint"`
	// SemanticFingerprint is the semantic-index fingerprint. Mismatch → re-embed.
	// Empty (default) means the semantic index has never been built; the next
	// `brain index` run with semantic enabled will populate it.
	SemanticFingerprint string `json:"semantic_fingerprint,omitempty"`
	// IndexedAt is the wall-clock time of the most recent literal-index build.
	IndexedAt time.Time `json:"indexed_at"`
	// SemanticIndexedAt is the wall-clock time of the most recent semantic
	// build. Zero when the semantic layer has never run.
	SemanticIndexedAt time.Time `json:"semantic_indexed_at,omitempty"`
	// Files is the per-file state map captured during the most recent walk;
	// shared by both indexers.
	Files map[string]FileState `json:"files"` // key: vault-relative path
}

// LoadState reads the index state from disk.
// Returns an empty state if the file does not exist.
func LoadState(indexDir string) (*IndexState, error) {
	path := filepath.Join(indexDir, stateFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &IndexState{Files: make(map[string]FileState)}, nil
		}
		return nil, fmt.Errorf("read index state %s: %w", path, err)
	}

	var state IndexState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse index state %s: %w", path, err)
	}
	if state.Files == nil {
		state.Files = make(map[string]FileState)
	}
	return &state, nil
}

// Save writes the index state to disk atomically.
func (s *IndexState) Save(indexDir string) error {
	s.mu.RLock()
	data, err := json.MarshalIndent(s, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("marshal index state: %w", err)
	}

	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		return fmt.Errorf("create index dir %s: %w", indexDir, err)
	}

	path := filepath.Join(indexDir, stateFileName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write index state %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename index state %s -> %s: %w", tmp, path, err)
	}
	return nil
}

// SetFingerprint updates the literal-index fingerprint and file map (thread-safe).
func (s *IndexState) SetFingerprint(
	fingerprint string,
	files map[string]FileState,
	indexedAt time.Time,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Fingerprint = fingerprint
	s.Files = files
	s.IndexedAt = indexedAt
}

// SetSemanticFingerprint updates the semantic-index fingerprint and timestamp
// (thread-safe). The file map is intentionally not touched here — callers
// invariably update both fingerprints in lockstep against the same walk.
func (s *IndexState) SetSemanticFingerprint(fingerprint string, indexedAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SemanticFingerprint = fingerprint
	s.SemanticIndexedAt = indexedAt
}

// IsSemanticStale reports whether the stored semantic fingerprint differs
// from the current vault fingerprint. An empty stored semantic fingerprint
// is always stale.
func (s *IndexState) IsSemanticStale(fingerprint string) bool {
	if s == nil {
		return true
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.SemanticFingerprint == "" || s.SemanticFingerprint != fingerprint
}

// ComputeFingerprint walks the vault and returns a sha256 hex fingerprint plus
// the per-file state map. Skips hidden dirs, attachments, and non-markdown files.
// Any change to the set of files, their sizes, or their mtimes yields a new
// fingerprint.
func ComputeFingerprint(vaultRoot string) (string, map[string]FileState, error) {
	files := make(map[string]FileState)

	walkErr := filepath.Walk(vaultRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // best-effort, skip unreadable entries
		}

		if info.IsDir() {
			base := filepath.Base(path)
			if path != vaultRoot && (skipDirs[base] || strings.HasPrefix(base, ".")) {
				return filepath.SkipDir
			}
			return nil
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".md" && ext != ".mdx" {
			return nil
		}

		rel, err := filepath.Rel(vaultRoot, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		files[rel] = FileState{
			Size:    info.Size(),
			ModTime: info.ModTime(),
		}
		return nil
	})
	if walkErr != nil {
		return "", nil, fmt.Errorf("walk vault %s: %w", vaultRoot, walkErr)
	}

	// Build a stable, sorted input string so the hash is deterministic.
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := sha256.New()
	for _, k := range keys {
		f := files[k]
		fmt.Fprintf(h, "%s|%d|%d\n", k, f.Size, f.ModTime.Unix())
	}
	fingerprint := fmt.Sprintf("%x", h.Sum(nil))

	return fingerprint, files, nil
}

// IsStale reports whether the stored state's fingerprint differs from the
// current one. An empty stored fingerprint is always stale.
func IsStale(state *IndexState, fingerprint string) bool {
	if state == nil {
		return true
	}
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.Fingerprint == "" || state.Fingerprint != fingerprint
}

// DefaultIndexDir returns the default index directory path at
// ~/.config/brain/index/.
func DefaultIndexDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "brain", "index"), nil
}

// IndexDirSize returns the total size of files in the index directory.
func IndexDirSize(indexDir string) (int64, error) {
	var size int64
	err := filepath.Walk(indexDir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // best-effort
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}
