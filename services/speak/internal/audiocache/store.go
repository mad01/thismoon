package audiocache

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/speak/internal/tts"
)

// ErrInvalidKey is returned for a key ValidKey rejects.
var ErrInvalidKey = errors.New("audiocache: invalid key")

// audioExts are the formats a clip is stored in, in lookup order.
var audioExts = []string{".wav", ".mp3"}

// tempSuffix marks a clip still being written; Put renames it into place.
const tempSuffix = ".tmp"

// Store keeps clips as <key>.wav or <key>.mp3 in one directory. A clip's
// mtime is its last use: Get and Touch bump it and Reap ages clips by it.
type Store struct {
	dir string
}

// NewStore returns a store over dir, which Put creates when it first writes.
func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

// Get reads the clip stored for key and marks it used. ok is false when no
// clip is stored.
func (s *Store) Get(key string) (audio tts.Audio, ok bool, err error) {
	if !ValidKey(key) {
		return tts.Audio{}, false, fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	for _, ext := range audioExts {
		path := s.path(key, ext)
		if err := touch(path); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return tts.Audio{}, false, fmt.Errorf("audiocache: mark %s used: %w", path, err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return tts.Audio{}, false, fmt.Errorf("audiocache: read %s: %w", path, err)
		}
		return tts.Audio{Data: data, ContentType: contentType(ext)}, true, nil
	}
	return tts.Audio{}, false, nil
}

// Has reports whether a clip is stored for key, without marking it used.
func (s *Store) Has(key string) bool {
	if !ValidKey(key) {
		return false
	}
	for _, ext := range audioExts {
		if _, err := os.Stat(s.path(key, ext)); err == nil {
			return true
		}
	}
	return false
}

// Touch marks the clip for key used and reports whether one is stored. A
// clip that cannot be touched counts as missing: synthesizing it again then
// surfaces the problem when Put fails.
func (s *Store) Touch(key string) bool {
	if !ValidKey(key) {
		return false
	}
	for _, ext := range audioExts {
		if touch(s.path(key, ext)) == nil {
			return true
		}
	}
	return false
}

// Put stores audio under key. The clip is written to a temporary file beside
// its final name and renamed into place, so a reader never sees half a clip.
func (s *Store) Put(key string, audio tts.Audio) error {
	if !ValidKey(key) {
		return fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("audiocache: create %s: %w", s.dir, err)
	}
	tmp, err := os.CreateTemp(s.dir, key+".*"+tempSuffix)
	if err != nil {
		return fmt.Errorf("audiocache: create temp clip: %w", err)
	}
	_, writeErr := tmp.Write(audio.Data)
	closeErr := tmp.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("audiocache: write %s: %w", tmp.Name(), err)
	}
	path := s.path(key, audio.Ext())
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("audiocache: store %s: %w", path, err)
	}
	return nil
}

// Reaped counts what one Reap removed.
type Reaped struct {
	Expired int // clips and stray temporary files unused for longer than maxAge
	Evicted int // clips removed, least recently used first, to get under maxBytes
}

// cacheFile is one file Reap weighs.
type cacheFile struct {
	name    string
	size    int64
	modTime time.Time
	temp    bool
}

// Reap removes the clips (and temporary files a crash left behind) last used
// more than maxAge before now, then the least recently used clips until the
// rest fit in maxBytes. Other files in the directory are left alone, and so
// are temporary files within maxAge: Put may be writing one.
func (s *Store) Reap(maxAge time.Duration, maxBytes int64, now time.Time) (Reaped, error) {
	files, err := s.list()
	if err != nil {
		return Reaped{}, err
	}
	var reaped Reaped
	var errs []error
	var kept []cacheFile
	var size int64
	for _, f := range files {
		switch {
		case now.Sub(f.modTime) > maxAge:
			if err := s.remove(f.name); err != nil {
				errs = append(errs, err)
				continue
			}
			reaped.Expired++
		case !f.temp:
			kept = append(kept, f)
			size += f.size
		}
	}
	slices.SortFunc(kept, func(a, b cacheFile) int { return a.modTime.Compare(b.modTime) })
	for _, f := range kept {
		if size <= maxBytes {
			break
		}
		if err := s.remove(f.name); err != nil {
			errs = append(errs, err)
			continue
		}
		size -= f.size
		reaped.Evicted++
	}
	return reaped, errors.Join(errs...)
}

// list returns the clips and temporary clips in the directory; a directory
// that does not exist yet holds none.
func (s *Store) list() ([]cacheFile, error) {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("audiocache: list %s: %w", s.dir, err)
	}
	var files []cacheFile
	for _, e := range entries {
		temp, ok := cacheFileKind(e.Name())
		if !ok || !e.Type().IsRegular() {
			continue
		}
		info, err := e.Info()
		if errors.Is(err, fs.ErrNotExist) {
			continue // removed since the listing
		}
		if err != nil {
			return nil, fmt.Errorf("audiocache: stat %s: %w", e.Name(), err)
		}
		files = append(files, cacheFile{
			name: e.Name(), size: info.Size(), modTime: info.ModTime(), temp: temp,
		})
	}
	return files, nil
}

func (s *Store) remove(name string) error {
	if err := os.Remove(filepath.Join(s.dir, name)); err != nil {
		return fmt.Errorf("audiocache: remove %s: %w", name, err)
	}
	return nil
}

func (s *Store) path(key, ext string) string {
	return filepath.Join(s.dir, key+ext)
}

// cacheFileKind reports whether name is a clip or a temporary clip Put
// wrote (ok), and which (temp).
func cacheFileKind(name string) (temp, ok bool) {
	key, rest, _ := strings.Cut(name, ".")
	switch {
	case !ValidKey(key):
		return false, false
	case rest == "wav" || rest == "mp3":
		return false, true
	default:
		return strings.HasSuffix(rest, tempSuffix), strings.HasSuffix(rest, tempSuffix)
	}
}

func contentType(ext string) string {
	if ext == ".mp3" {
		return tts.ContentTypeMP3
	}
	return tts.ContentTypeWAV
}

func touch(path string) error {
	now := time.Now()
	return os.Chtimes(path, now, now)
}
