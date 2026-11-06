package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mad01/thismoon/services/brain/internal/atomicfile"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// maxLayoutBody caps a POST /api/layout body at 8 MiB.
const maxLayoutBody = 8 << 20

// keepLayouts is how many layout files the layout directory keeps, newest
// first.
const keepLayouts = 3

// layoutExt ends every layout file's name: <key>.json.
const layoutExt = ".json"

// positions maps a graph node id to its x and y.
type positions map[string][2]float64

// layoutFile is one cached layout, as stored in <key>.json.
type layoutFile struct {
	Version   int       `json:"version"`
	Key       string    `json:"key"`
	Topics    bool      `json:"topics"`
	Positions positions `json:"positions"`
}

type layoutRequest struct {
	Key       string               `json:"key"`
	Positions map[string][]float64 `json:"positions"`
}

// handleLayout stores the positions the browser computed for a layout
// key. A key that is no longer current is a 409: the vaults changed while
// the browser laid the graph out, and it lays it out again.
func (s *Server) handleLayout(w http.ResponseWriter, r *http.Request) {
	req, err := decodeLayout(w, r)
	if err != nil {
		writeError(w, err)
		return
	}
	m, err := s.current()
	if err != nil {
		writeError(w, err)
		return
	}
	var topics bool
	switch req.Key {
	case layoutKey(m.prints, false):
	case layoutKey(m.prints, true):
		topics = true
	default:
		writeError(w, &apiError{
			status:  http.StatusConflict,
			code:    codeStaleKey,
			message: "layout key is not the current key; fetch /api/graph again",
		})
		return
	}
	g, err := m.sn.Graph(vault.GraphOptions{TagsAsNodes: topics})
	if err != nil {
		writeError(w, err)
		return
	}
	known := make(map[string]bool, len(g.Nodes))
	for _, n := range g.Nodes {
		known[n.ID] = true
	}
	file := layoutFile{Version: layoutVersion, Key: req.Key, Topics: topics, Positions: positions{}}
	for id, xy := range req.Positions {
		if known[id] {
			file.Positions[id] = [2]float64{xy[0], xy[1]}
		}
	}
	if err := s.writeLayout(file); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// decodeLayout reads a layout body of at most maxLayoutBody bytes.
func decodeLayout(w http.ResponseWriter, r *http.Request) (layoutRequest, error) {
	var req layoutRequest
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxLayoutBody))
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		return req, &apiError{
			status:  http.StatusRequestEntityTooLarge,
			code:    codeTooLarge,
			message: fmt.Sprintf("layout body exceeds %d bytes", maxLayoutBody),
		}
	case err != nil:
		return req, badRequest("read layout body: %v", err)
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return req, badRequest("layout body is not {\"key\",\"positions\"}: %v", err)
	}
	if req.Key == "" {
		return req, badRequest("layout body has no key")
	}
	for id, xy := range req.Positions {
		if len(xy) != 2 {
			return req, badRequest("position of %q has %d numbers, want [x,y]", id, len(xy))
		}
	}
	return req, nil
}

// writeLayout replaces the layout file of file.Key atomically, then drops
// all but the newest keepLayouts files.
func (s *Server) writeLayout(file layoutFile) error {
	data, err := json.Marshal(file)
	if err != nil {
		return err
	}
	s.layoutMu.Lock()
	defer s.layoutMu.Unlock()
	if err := os.MkdirAll(s.layoutDir, 0o755); err != nil {
		return err
	}
	name := file.Key + layoutExt
	if err := atomicfile.Write(filepath.Join(s.layoutDir, name), data, 0o644); err != nil {
		return err
	}
	return s.pruneLayouts(name)
}

// pruneLayouts removes the layout files past the newest keepLayouts,
// always keeping the one just written.
func (s *Server) pruneLayouts(written string) error {
	files, err := s.layoutFiles()
	if err != nil {
		return err
	}
	kept := 1
	for _, f := range files {
		if f.name == written {
			continue
		}
		if kept < keepLayouts {
			kept++
			continue
		}
		err := os.Remove(filepath.Join(s.layoutDir, f.name))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

type layoutEntry struct {
	name string
	info fs.FileInfo
}

// layoutFiles lists the layout files, newest first. Temporary files of a
// write in progress start with a dot and are left out.
func (s *Server) layoutFiles() ([]layoutEntry, error) {
	entries, err := os.ReadDir(s.layoutDir)
	if err != nil {
		return nil, err
	}
	var out []layoutEntry
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") || !strings.HasSuffix(name, layoutExt) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, layoutEntry{name: name, info: info})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].info.ModTime(), out[j].info.ModTime()
		if !a.Equal(b) {
			return a.After(b)
		}
		return out[i].name > out[j].name
	})
	return out, nil
}

// readLayout returns the cached positions of key, and true. On a miss it
// returns the positions of the newest layout file, and false, so notes the
// browser already placed keep their place.
func (s *Server) readLayout(key string) (positions, bool) {
	if p, err := s.loadLayout(key + layoutExt); err == nil {
		return p, true
	}
	files, err := s.layoutFiles()
	if err != nil || len(files) == 0 {
		return nil, false
	}
	p, err := s.loadLayout(files[0].name)
	if err != nil {
		log.Printf("brain: layout %s: %v", files[0].name, err)
		return nil, false
	}
	return p, false
}

func (s *Server) loadLayout(name string) (positions, error) {
	data, err := os.ReadFile(filepath.Join(s.layoutDir, name))
	if err != nil {
		return nil, err
	}
	var f layoutFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	if f.Version != layoutVersion {
		return nil, fmt.Errorf("layout version %d, want %d", f.Version, layoutVersion)
	}
	return f.Positions, nil
}
