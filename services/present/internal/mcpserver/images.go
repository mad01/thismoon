package mcpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mad01/thismoon/kit/confdir"
	present "github.com/mad01/thismoon/services/present"
	"github.com/mad01/thismoon/services/present/internal/images"
	"github.com/mad01/thismoon/services/present/internal/render"
)

// errNoImageStore is the shared instance's answer to an image block whose
// src is not a URL: it has no store to put a file in, and nothing here could
// read a file on the caller's machine anyway.
var errNoImageStore = errors.New(
	"this instance stores no local images; use an http or https URL for the image instead",
)

// imageIngest turns the image blocks of a Doc whose src is a file on this
// machine into stored images: the file is read, checked, named by its
// hash, and the src rewritten to the path the server serves it at. The
// bytes are held until commit, which runs once the Doc has compiled, so a
// refused Doc leaves no file behind and a stored page never names an
// image that is not there.
type imageIngest struct {
	store   *images.Store // nil on a shared instance: no local files
	pending map[string][]byte
}

func (h *handlers) newImageIngest() *imageIngest {
	return &imageIngest{store: h.images, pending: map[string][]byte{}}
}

// doc rewrites the local image sources in a Doc JSON string and returns the
// JSON to compile. Anything that is not Doc JSON (legacy HTML, an empty
// string, a string the parser refuses, which Compile reports) and a Doc
// without a local image source pass through byte for byte.
func (g *imageIngest) doc(s string) (string, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" || trimmed[0] != '{' {
		return s, nil
	}
	var d render.Doc
	if err := json.Unmarshal([]byte(trimmed), &d); err != nil {
		return s, nil
	}
	changed := false
	err := d.EachBlock(func(b *render.Block) error {
		if b.T != "image" || render.IsHTTPURL(b.Src) {
			return nil
		}
		if g.store == nil {
			return fmt.Errorf("image %q: %w", b.Src, errNoImageStore)
		}
		if name, stored := images.NameOf(b.Src); stored {
			// A served path from an earlier page is fine while its file is
			// here; one from another machine or a swept file would render
			// a broken image.
			if !g.store.Has(name) {
				return fmt.Errorf(
					"image %q: not in this machine's image store; pass the file's path instead",
					b.Src,
				)
			}
			return nil
		}
		name, err := g.take(b.Src)
		if err != nil {
			return err
		}
		b.Src = images.Served(name)
		changed = true
		return nil
	})
	if err != nil {
		return "", err
	}
	if !changed {
		return s, nil
	}
	out, err := json.Marshal(d)
	if err != nil {
		return "", fmt.Errorf("image sources: %w", err)
	}
	return string(out), nil
}

// update rewrites the image sources of the content and deck an update
// carries, leaving the inputs it does not carry alone.
func (g *imageIngest) update(in updateInput) (updateInput, error) {
	if in.Content != nil {
		s, err := g.doc(*in.Content)
		if err != nil {
			return updateInput{}, fmt.Errorf("content: %w", err)
		}
		in.Content = &s
	}
	if in.Deck != nil {
		s, err := g.doc(*in.Deck)
		if err != nil {
			return updateInput{}, fmt.Errorf("deck: %w", err)
		}
		in.Deck = &s
	}
	return in, nil
}

// take reads the image file src names, checks its size and type, and holds
// its bytes under their stored name.
func (g *imageIngest) take(src string) (string, error) {
	path, err := localImagePath(src)
	if err != nil {
		return "", err
	}
	data, err := readCapped(path, present.MaxImageBytes)
	if err != nil {
		return "", fmt.Errorf("image %q: %w", src, err)
	}
	ext, err := images.Sniff(data)
	if err != nil {
		return "", fmt.Errorf("image %q: %w", src, err)
	}
	name := images.Name(data, ext)
	g.pending[name] = data
	return name, nil
}

// commit writes the images the Doc now names. It runs once the Doc has
// compiled and before the page is stored.
func (g *imageIngest) commit() error {
	for name, data := range g.pending {
		if err := g.store.Write(name, data); err != nil {
			return err
		}
	}
	return nil
}

// localImagePath resolves an image src that names a file on this machine:
// an absolute path or a ~ path. A relative path is refused, because the
// MCP process's working directory is not the caller's.
func localImagePath(src string) (string, error) {
	path, err := confdir.Expand(src)
	if err != nil {
		return "", fmt.Errorf("image %q: %w", src, err)
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf(
			"image %q: want an absolute path or a ~ path to the file, or an http(s) URL", src,
		)
	}
	return path, nil
}

// readCapped reads a regular file of at most limit bytes and refuses a
// longer one without reading it all. Anything but a regular file is
// refused before it is opened: a FIFO would hold the tool call open.
func readCapped(path string, limit int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("over the %d MiB cap for an image", limit>>20)
	}
	return data, nil
}
