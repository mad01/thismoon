package spool

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Spool file names and permissions. Everything in the spool is private to
// the user: chunks hold file contents and prompts.
const (
	tmpDir       = "tmp"
	chunkSuffix  = ".chunk"
	envSuffix    = ".json"
	dirPerm      = 0o700
	filePerm     = 0o600
	maxNameIDLen = 128
)

// Writer puts envelopes and their chunks into a spool directory. It is the
// capture side: `moondb record` and `moondb import` write through it and
// return without waiting for serve.
type Writer struct {
	// Dir is the spool directory, <workdir>/spool.
	Dir string
	// Objects is the store's object directory. A chunk already stored there
	// as <Objects>/<sha[:2]>/<sha>.gz is not written to the spool again.
	// Empty disables the check.
	Objects string
}

// ChunkKey returns the key a chunk is stored under: the lowercase hex
// SHA-256 of its bytes.
func ChunkKey(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Write validates env, puts every chunk in place, then renames the envelope
// into the spool as <unix-nanos>-<kind>-<id>.json. chunks maps each key to
// its bytes; a key that does not match its bytes is an error, so a reader
// never meets a chunk that fails verification. A failed Write may leave
// chunk files without an envelope; Reader.Sweep removes them.
func (w *Writer) Write(env Envelope, chunks map[string][]byte) error {
	if err := env.validate(); err != nil {
		return err
	}
	tmp := filepath.Join(w.Dir, tmpDir)
	if err := os.MkdirAll(tmp, dirPerm); err != nil {
		return fmt.Errorf("spool: create %s: %w", tmp, err)
	}
	for key, data := range chunks {
		if err := w.putChunk(key, data); err != nil {
			return err
		}
	}
	body, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("spool: encode envelope %s: %w", env.ID, err)
	}
	name := fmt.Sprintf("%020d-%s-%s%s", time.Now().UnixNano(), env.Kind, nameID(env.ID), envSuffix)
	return w.place(body, name)
}

// Stored reports whether the store's object directory already holds the
// chunk key, so a writer can point an envelope at it without its bytes.
// It is false when Objects is empty or key is not a chunk key.
func (w *Writer) Stored(key string) bool {
	return w.Objects != "" && validKey(key) && exists(filepath.Join(w.Objects, key[:2], key+".gz"))
}

// putChunk writes one chunk unless the spool or the store already holds
// it. An existing spool chunk gets a fresh mtime, so a Sweep running
// between this call and the envelope rename leaves it alone.
func (w *Writer) putChunk(key string, data []byte) error {
	if !validKey(key) || ChunkKey(data) != key {
		return fmt.Errorf("spool: chunk key %q does not match its bytes", key)
	}
	if w.Stored(key) {
		return nil
	}
	path := filepath.Join(w.Dir, key+chunkSuffix)
	if exists(path) {
		now := time.Now()
		if err := os.Chtimes(path, now, now); err == nil {
			return nil
		}
		// Swept between the stat and the touch: write it again.
	}
	return w.place(data, key+chunkSuffix)
}

// place writes data to a temp file under tmp/ and renames it to name in
// the spool directory, so a reader sees the whole file or none of it.
func (w *Writer) place(data []byte, name string) error {
	f, err := os.CreateTemp(filepath.Join(w.Dir, tmpDir), "w-*")
	if err != nil {
		return fmt.Errorf("spool: %w", err)
	}
	_, werr := f.Write(data)
	cerr := f.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = os.Remove(f.Name())
		return fmt.Errorf("spool: write %s: %w", name, err)
	}
	if err := os.Rename(f.Name(), filepath.Join(w.Dir, name)); err != nil {
		_ = os.Remove(f.Name())
		return fmt.Errorf("spool: place %s: %w", name, err)
	}
	return nil
}

// validate checks what Write promises a reader: a known version, a known
// kind, an id, and exactly the payload the kind names.
func (e Envelope) validate() error {
	if e.Version != Version {
		return fmt.Errorf("spool: envelope version %d, want %d", e.Version, Version)
	}
	if e.ID == "" {
		return errors.New("spool: envelope has no id")
	}
	set := map[Kind]bool{
		KindTurnOpen:   e.TurnOpen != nil,
		KindTurnClose:  e.TurnClose != nil,
		KindEdit:       e.Edit != nil,
		KindHeadMoved:  e.HeadMoved != nil,
		KindSnapshot:   e.Snapshot != nil,
		KindSkipped:    e.Skipped != nil,
		KindAgentClose: e.AgentClose != nil,
	}
	if _, known := set[e.Kind]; !known {
		return fmt.Errorf("spool: unknown envelope kind %q", e.Kind)
	}
	for kind, has := range set {
		if has != (kind == e.Kind) {
			return fmt.Errorf("spool: %s envelope %s must carry its own payload and no other",
				e.Kind, e.ID)
		}
	}
	return nil
}

// nameID makes an envelope id safe for a file name: every byte outside
// [A-Za-z0-9._-] becomes '_', and the result is capped in length. The name
// is for people listing the spool; identity stays in the envelope.
func nameID(id string) string {
	if len(id) > maxNameIDLen {
		id = id[:maxNameIDLen]
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '.', r == '_', r == '-':
			return r
		}
		return '_'
	}, id)
}

// validKey reports whether key has the shape ChunkKey produces, so it can
// name a file without escaping the spool directory.
func validKey(key string) bool {
	if len(key) != sha256.Size*2 {
		return false
	}
	for _, c := range key {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
