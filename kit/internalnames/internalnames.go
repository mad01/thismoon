// Package internalnames reads the shared internal-names files that belt and
// suspenders each include from their own config (docs/adr/0016). A names
// file carries up to three lists and nothing else: a misspelled key would
// otherwise parse cleanly and guard nothing, so unknown keys are an error.
//
// Read returns the lists as written. Each consumer applies its own
// derivation rules (case folding, minimum length, deduplication) on top,
// exactly as it does for the lists in its own config section.
package internalnames

import (
	"errors"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

// File is the content of one names file.
type File struct {
	// BlockedWords are names blocked outright, independent of any repo a
	// consumer discovers under its workspace directories.
	BlockedWords []string `yaml:"blocked_words"`
	// Allowlist names (or their basenames) that leave the blocked set even
	// though a consumer would otherwise derive them.
	Allowlist []string `yaml:"allowlist"`
	// AllowPhrases are exact phrases neutralized in checked content before
	// name matching, so a sanctioned compound that contains a blocked name
	// passes while the bare name anywhere else still matches.
	AllowPhrases []string `yaml:"allow_phrases"`
}

// Read parses the names file at path. An empty file is an empty File. An
// unknown key, a non-list value, or malformed YAML is an error naming the
// file. A missing file is returned as the open error unwrapped, so callers
// can tell absence from breakage with errors.Is(err, fs.ErrNotExist).
func Read(path string) (File, error) {
	f, err := os.Open(path)
	if err != nil {
		return File{}, err
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	var out File
	if err := dec.Decode(&out); err != nil {
		if errors.Is(err, io.EOF) {
			return File{}, nil
		}
		return File{}, fmt.Errorf("internalnames: parse %s: %w", path, err)
	}
	return out, nil
}
