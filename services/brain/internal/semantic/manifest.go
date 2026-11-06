package semantic

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/atomicfile"
	"github.com/mad01/thismoon/services/brain/internal/chunk"
)

// manifestFile sits at the root of an index directory. chromem-go keeps its
// collections in subdirectories and never reads a root file.
const manifestFile = "manifest.json"

// ManifestVersion changes when the chunk metadata the index stores changes
// shape or meaning. An index whose manifest has another version is rebuilt.
// Version 2 stores the effective status, not the literal status line.
const ManifestVersion = 2

// Settings are what an index's chunks were made with. Chunks made with
// other settings are not comparable, so a change rebuilds the index.
type Settings struct {
	EmbedModel   string `json:"embed_model"`
	ChunkWords   int    `json:"chunk_words"`
	OverlapWords int    `json:"overlap_words"`
}

// SettingsFor returns the settings of chunks made by opts and embedded with
// model.
func SettingsFor(model string, opts chunk.Options) Settings {
	return Settings{
		EmbedModel:   model,
		ChunkWords:   opts.TargetWords,
		OverlapWords: opts.OverlapWords,
	}
}

// Manifest records what a vault's semantic index holds: the settings its
// chunks were made with, the vault fingerprint it was last synced against,
// and the content hash of every note it embedded.
type Manifest struct {
	Version     int               `json:"version"`
	Settings    Settings          `json:"settings"`
	Fingerprint string            `json:"fingerprint"`
	IndexedAt   time.Time         `json:"indexed_at"`
	Notes       map[string]string `json:"notes"`
}

// LoadManifest reads the manifest in dir. found is false when there is none.
func LoadManifest(dir string) (m Manifest, found bool, err error) {
	data, err := os.ReadFile(filepath.Join(dir, manifestFile))
	if errors.Is(err, fs.ErrNotExist) {
		return Manifest{}, false, nil
	}
	if err != nil {
		return Manifest{}, false, fmt.Errorf("read semantic manifest: %w", err)
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, false, fmt.Errorf("parse semantic manifest in %s: %w", dir, err)
	}
	return m, true, nil
}

// Save writes the manifest into dir.
func (m Manifest) Save(dir string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal semantic manifest: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create semantic index dir: %w", err)
	}
	if err := atomicfile.Write(filepath.Join(dir, manifestFile), data, 0o644); err != nil {
		return fmt.Errorf("write semantic manifest: %w", err)
	}
	return nil
}

// Compatible reports whether the index's chunks were made the way s makes
// them, so a sync may keep the unchanged ones.
func (m Manifest) Compatible(s Settings) bool {
	return m.Version == ManifestVersion && m.Settings == s
}

// Fresh reports whether the index matches a vault with fingerprint, made
// with s.
func (m Manifest) Fresh(fingerprint string, s Settings) bool {
	return m.Compatible(s) && m.Fingerprint != "" && m.Fingerprint == fingerprint
}
