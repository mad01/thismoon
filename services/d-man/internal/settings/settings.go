// Package settings keeps the webkit UI settings that follow the user between
// .this hosts: one small JSON document d-man serves on every configured host,
// so a theme picked on one host is there on the next.
//
// The keys are the kit's own localStorage keys and every value is a string.
// Only keys in the value table are stored, and only with a value the kit
// accepts. Store holds the document in memory and saves it to one file.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"maps"
	"os"
	"slices"
	"strconv"
	"sync"

	"github.com/mad01/thismoon/services/d-man/internal/atomicfile"
)

// Version is the document version d-man reads and writes.
const Version = 1

// fileMode is the settings file's mode: the root daemon writes it, and
// nothing in it is secret.
const fileMode = 0o644

// The kit's font-size bounds in px (webkit/src/size.ts).
const (
	sizeMin = 12
	sizeMax = 24
)

// families are the palette family names the kit ships, default first
// (webkit.Themes()). A drift test keeps the list equal to the kit's.
var families = []string{
	"default", "catppuccin", "gruvbox", "nord", "one", "rose-pine", "solarized", "tokyo-night",
}

// fonts are the font names the kit's font picker offers (FONT_STACKS in
// webkit/src/webkit.ts).
var fonts = []string{"Fira Code", "Inter", "Lexend", "Work Sans"}

// valueTable maps each stored key to the check its value must pass.
var valueTable = map[string]func(string) bool{
	"webkit-theme":         oneOf("light", "dark", "system"),
	"webkit-palette-light": oneOf(families...),
	"webkit-palette-dark":  oneOf(families...),
	"webkit-font":          oneOf(fonts...),
	"webkit-size":          wholeNumber(sizeMin, sizeMax),
	"webkit-fixation":      oneOf("true", "false"),
}

// ErrInvalid marks a patch the store refused: a known key with a value
// outside its set.
var ErrInvalid = errors.New("invalid setting")

// Document is the settings document on the wire and on disk.
type Document struct {
	V        int               `json:"v"`
	Settings map[string]string `json:"settings"`
}

func oneOf(values ...string) func(string) bool {
	return func(v string) bool { return slices.Contains(values, v) }
}

// wholeNumber accepts a base-10 integer in [lo, hi] written the way the kit
// writes one: no sign, no leading zero, no fraction.
func wholeNumber(lo, hi int) func(string) bool {
	return func(v string) bool {
		n, err := strconv.Atoi(v)
		return err == nil && strconv.Itoa(n) == v && n >= lo && n <= hi
	}
}

// Known reports whether key is a setting the store keeps.
func Known(key string) bool {
	_, ok := valueTable[key]
	return ok
}

// Valid reports whether value is one the kit accepts for the known key.
func Valid(key, value string) bool {
	check, ok := valueTable[key]
	return ok && check(value)
}

// Store is the settings document in memory plus the file it is saved to.
// It is safe for concurrent use; patches are applied and saved one at a time.
type Store struct {
	path string

	mu       sync.Mutex
	settings map[string]string
}

// Open loads the store saved at path. A missing file opens empty. A file
// that cannot be read or parsed, or that holds another version, is logged
// to errLog and the store opens empty, so a bad settings file never keeps
// the front door down. Entries the value table refuses are dropped with a
// warning.
func Open(path string, errLog *log.Logger) *Store {
	s := &Store{path: path, settings: map[string]string{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s
	}
	if err != nil {
		errLog.Printf("settings: %v; starting empty", err)
		return s
	}
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		errLog.Printf("settings: %s is not a settings document (%v); starting empty", path, err)
		return s
	}
	if doc.V != Version {
		errLog.Printf("settings: %s has version %d, want %d; starting empty", path, doc.V, Version)
		return s
	}
	for key, value := range doc.Settings {
		if !Valid(key, value) {
			errLog.Printf("settings: %s: dropping %s=%q", path, key, value)
			continue
		}
		s.settings[key] = value
	}
	return s
}

// Document returns a copy of the current settings.
func (s *Store) Document() Document {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Document{V: Version, Settings: maps.Clone(s.settings)}
}

// Patch merges patch into the settings key by key, saves the result when it
// differs, and returns the full document after the merge. Unknown keys are
// dropped. A known key with a refused value fails the whole patch with
// ErrInvalid, and a failed save returns its error; either way the settings
// stay as they were.
func (s *Store) Patch(patch map[string]string) (Document, error) {
	for key, value := range patch {
		if Known(key) && !Valid(key, value) {
			return Document{}, fmt.Errorf("%w: %s=%q", ErrInvalid, key, value)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := maps.Clone(s.settings)
	for key, value := range patch {
		if Known(key) {
			next[key] = value
		}
	}
	if !maps.Equal(next, s.settings) {
		if err := s.save(Document{V: Version, Settings: next}); err != nil {
			return Document{}, err
		}
		s.settings = next
	}
	return Document{V: Version, Settings: maps.Clone(next)}, nil
}

// save writes doc to the store's file, indented, through a temp file and a
// rename.
func (s *Store) save(doc Document) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	if err := atomicfile.Write(s.path, append(data, '\n'), fileMode); err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	return nil
}
