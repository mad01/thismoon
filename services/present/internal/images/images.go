// Package images is present's local image store: the files an image block
// points at when its source was a file on this machine. The MCP tools copy
// the file in under its content hash, the page keeps the served path, and
// the server serves it immutable. A shared instance has no store; the
// images it shows are URLs.
package images

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Prefix is the URL path every stored image is served under.
const Prefix = "/img/"

// dirName is the directory under the workdir that holds the files.
const dirName = "images"

// sweepGrace is how old a file must be before a sweep may remove it. The
// MCP process writes an image before it writes the page that names it, so
// a sweep that ran between the two would take the image from under a page
// about to exist.
const sweepGrace = time.Minute

// exts maps the content types the store takes to the extension their file
// gets. The served path carries it, so the content type comes back from
// the name alone. SVG is not among them: it can carry script.
var exts = map[string]string{
	"image/png":  "png",
	"image/jpeg": "jpg",
	"image/gif":  "gif",
	"image/webp": "webp",
}

// reName is the shape of a stored name: the hex sha256 of the bytes and
// one of the extensions above, jpeg beside jpg for a path written by hand.
var reName = regexp.MustCompile(`^[0-9a-f]{64}\.(png|jpe?g|gif|webp)$`)

// reRef finds served paths in rendered HTML or a Doc.
var reRef = regexp.MustCompile(`/img/([0-9a-f]{64}\.(?:png|jpe?g|gif|webp))\b`)

// ValidName reports whether name is one the store could have written: no
// path separators, no traversal, just the hash and the extension.
func ValidName(name string) bool { return reName.MatchString(name) }

// Served returns the path a stored name is served at.
func Served(name string) string { return Prefix + name }

// NameOf returns the stored name a served path names, and whether p is one.
func NameOf(p string) (string, bool) {
	name, ok := strings.CutPrefix(p, Prefix)
	if !ok || !ValidName(name) {
		return "", false
	}
	return name, true
}

// Referenced returns every stored name a fragment of HTML or JSON points
// at, each once, in order of first use.
func Referenced(s string) []string {
	var names []string
	seen := map[string]bool{}
	for _, m := range reRef.FindAllStringSubmatch(s, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			names = append(names, m[1])
		}
	}
	return names
}

// ContentType is the MIME type a stored name is served with.
func ContentType(name string) string {
	switch path.Ext(name) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	}
	return ""
}

// Sniff returns the extension a file's bytes get, or an error naming the
// type when the bytes are not an image the store takes.
func Sniff(data []byte) (string, error) {
	ct, _, _ := strings.Cut(http.DetectContentType(data), ";")
	ext, ok := exts[ct]
	if !ok {
		return "", fmt.Errorf("images: content type %s; want png, jpeg, gif, or webp", ct)
	}
	return ext, nil
}

// Name is the stored name for data: its sha256 and the extension Sniff
// gave it.
func Name(data []byte, ext string) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]) + "." + ext
}

// Store is the image directory under a present workdir.
type Store struct {
	dir string
}

// New returns the Store for workdir. Nothing is created until the first
// write, so a workdir without images stays as it is.
func New(workdir string) *Store {
	return &Store{dir: filepath.Join(workdir, dirName)}
}

// Dir is the directory the store keeps its files in.
func (s *Store) Dir() string { return s.dir }

func (s *Store) filePath(name string) string { return filepath.Join(s.dir, name) }

// Write stores data under name. A name is its content's hash, so a file
// that already exists holds the same bytes and is left alone.
func (s *Store) Write(name string, data []byte) error {
	if !ValidName(name) {
		return fmt.Errorf("images: invalid name %q", name)
	}
	if _, err := os.Stat(s.filePath(name)); err == nil {
		return nil
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create images dir: %w", err)
	}
	tmp, err := os.CreateTemp(s.dir, "."+name+".*")
	if err != nil {
		return fmt.Errorf("write image: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write image: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write image: %w", err)
	}
	// A temp file is created private; the pages beside it are 0644 and a
	// served file should match.
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write image: %w", err)
	}
	if err := os.Rename(tmp.Name(), s.filePath(name)); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write image: %w", err)
	}
	return nil
}

// Open opens a stored image for serving. A malformed name is
// os.ErrNotExist like an unknown one, so a caller answers 404 either way
// and no name reaches the filesystem unchecked.
func (s *Store) Open(name string) (*os.File, error) {
	if !ValidName(name) {
		return nil, os.ErrNotExist
	}
	return os.Open(s.filePath(name))
}

// Sweep removes every stored image whose name is not in keep and was
// written more than a minute before now, and returns the names it
// removed. A store that has no directory yet has nothing to sweep.
func (s *Store) Sweep(keep map[string]bool, now time.Time) ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read images dir: %w", err)
	}
	var removed []string
	for _, e := range entries {
		name := e.Name()
		if !ValidName(name) || keep[name] {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return removed, fmt.Errorf("stat image %s: %w", name, err)
		}
		if now.Sub(info.ModTime()) < sweepGrace {
			continue
		}
		if err := os.Remove(s.filePath(name)); err != nil {
			return removed, fmt.Errorf("remove image %s: %w", name, err)
		}
		removed = append(removed, name)
	}
	return removed, nil
}
