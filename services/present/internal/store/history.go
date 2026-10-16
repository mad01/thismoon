package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// historyDir is the directory under a page holding one file per version,
// history/<version>.json.
const historyDir = "history"

// historyKeep is how many versions a page keeps, the newest ones.
const historyKeep = 20

// ErrRevisionNotKept is a version the page's history does not hold: older
// than the kept window, newer than the page, or never recorded.
var ErrRevisionNotKept = errors.New("present: that version is not kept")

// Revision is one version of a page as its history keeps it: what
// present_source returns for that version. The brief's HTML and the
// graph's script are kept only when no source exists to rebuild them from.
type Revision struct {
	Version       int         `json:"version"`
	UpdatedAt     time.Time   `json:"updated_at"`
	Title         string      `json:"title"`
	References    []Reference `json:"references,omitempty"`
	Doc           string      `json:"doc,omitempty"`
	DeckSource    string      `json:"deck_source,omitempty"`
	GraphSource   string      `json:"graph_source,omitempty"`
	LegacyContent string      `json:"legacy_content,omitempty"`
	LegacyGraph   string      `json:"legacy_graph,omitempty"`
}

// RevisionInfo is a revision as a listing names it.
type RevisionInfo struct {
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
	Title     string    `json:"title"`
}

// Historian is the history surface of a store that keeps one: the
// filesystem store, which records every version it writes and keeps the
// newest historyKeep. A wrapped or cluster store does not implement it.
type Historian interface {
	// Revisions lists the kept versions of a page, newest first.
	Revisions(ctx context.Context, id string) ([]RevisionInfo, error)
	// Revision loads one kept version. ErrRevisionNotKept when the history
	// does not hold it.
	Revision(ctx context.Context, id string, version int) (Revision, error)
}

var _ Historian = (*FS)(nil)

// revisionOf is the history record of snap.
func revisionOf(snap Snapshot) Revision {
	p := snap.Page
	r := Revision{
		Version: p.Version, UpdatedAt: p.UpdatedAt, Title: p.Title, References: p.References,
		Doc: string(snap.Sources.Doc), DeckSource: string(snap.Sources.Deck),
		GraphSource: string(snap.Sources.Graph),
	}
	if snap.Sources.Doc == nil {
		r.LegacyContent = p.Content
	}
	if snap.Sources.Graph == nil {
		r.LegacyGraph = p.Graph
	}
	return r
}

// ContentSource is the revision's brief as present_source names it: "doc"
// with the Doc JSON, or "html" with the stored HTML of a page without one.
func (r Revision) ContentSource() (format, content string) {
	if r.Doc != "" {
		return "doc", r.Doc
	}
	return "html", r.LegacyContent
}

// GraphSourceOf is the revision's graph as present_source names it: "json"
// with the GraphInput, "js" with a legacy script, or empty for no graph.
func (r Revision) GraphSourceOf() (format, graph string) {
	switch {
	case r.GraphSource != "":
		return "json", r.GraphSource
	case r.LegacyGraph != "":
		return "js", r.LegacyGraph
	}
	return "", ""
}

// writeRevision records snap in the page's history and drops the versions
// past the newest historyKeep. It runs before meta.json moves the version,
// so a crash leaves at most a record for a version that never committed,
// which the next edit, writing that same version number, overwrites.
func (s *FS) writeRevision(snap Snapshot) error {
	dir := filepath.Join(s.pageDir(snap.Page.ID), historyDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create history dir: %w", err)
	}
	data, err := json.Marshal(revisionOf(snap))
	if err != nil {
		return fmt.Errorf("encode revision: %w", err)
	}
	if err := writeFileAtomic(revisionPath(dir, snap.Page.Version), data); err != nil {
		return fmt.Errorf("write revision: %w", err)
	}
	return pruneHistory(dir)
}

// pruneHistory removes every recorded version but the newest historyKeep.
func pruneHistory(dir string) error {
	versions, err := keptVersions(dir)
	if err != nil {
		return err
	}
	for len(versions) > historyKeep {
		if err := os.Remove(revisionPath(dir, versions[0])); err != nil {
			return fmt.Errorf("prune revision %d: %w", versions[0], err)
		}
		versions = versions[1:]
	}
	return nil
}

// keptVersions lists the versions recorded in dir, oldest first. A file
// whose name is not <version>.json is not a revision and is skipped.
func keptVersions(dir string) ([]int, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read history dir: %w", err)
	}
	var versions []int
	for _, e := range entries {
		v, err := strconv.Atoi(strings.TrimSuffix(e.Name(), ".json"))
		if err == nil && v > 0 && strings.HasSuffix(e.Name(), ".json") {
			versions = append(versions, v)
		}
	}
	slices.Sort(versions)
	return versions, nil
}

func revisionPath(dir string, version int) string {
	return filepath.Join(dir, strconv.Itoa(version)+".json")
}

// Revisions lists the kept versions of a page, newest first. A page from
// before history was kept lists none until its next write.
func (s *FS) Revisions(_ context.Context, id string) ([]RevisionInfo, error) {
	if _, err := s.readMeta(id); err != nil {
		return nil, err
	}
	dir := filepath.Join(s.pageDir(id), historyDir)
	versions, err := keptVersions(dir)
	if err != nil {
		return nil, err
	}
	out := make([]RevisionInfo, 0, len(versions))
	for _, v := range slices.Backward(versions) {
		r, err := readRevision(dir, v)
		if err != nil {
			return nil, err
		}
		out = append(out, RevisionInfo{Version: r.Version, UpdatedAt: r.UpdatedAt, Title: r.Title})
	}
	return out, nil
}

// Revision loads one kept version of a page.
func (s *FS) Revision(_ context.Context, id string, version int) (Revision, error) {
	if _, err := s.readMeta(id); err != nil {
		return Revision{}, err
	}
	if version <= 0 {
		return Revision{}, fmt.Errorf("%w: %d", ErrRevisionNotKept, version)
	}
	return readRevision(filepath.Join(s.pageDir(id), historyDir), version)
}

func readRevision(dir string, version int) (Revision, error) {
	data, err := os.ReadFile(revisionPath(dir, version))
	if errors.Is(err, os.ErrNotExist) {
		return Revision{}, fmt.Errorf("%w: %d", ErrRevisionNotKept, version)
	}
	if err != nil {
		return Revision{}, fmt.Errorf("read revision %d: %w", version, err)
	}
	var r Revision
	if err := json.Unmarshal(data, &r); err != nil {
		return Revision{}, fmt.Errorf("decode revision %d: %w", version, err)
	}
	return r, nil
}
