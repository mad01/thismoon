// Package store is the single source of truth for events: per-source in-memory
// ring buffers guarded by a mutex, each persisted to its own append-only JSONL
// file. The serve process owns a Store; HTTP handlers append and query through
// these methods, so there is exactly one writer and no file-lock contention.
package store

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mad01/thismoon/services/events/internal/event"
)

// sourcesSubdir holds one <sanitizedSource>.jsonl file per source under the workdir.
const sourcesSubdir = "sources"

// compactFactor triggers a rewrite once a file is estimated to hold more than
// perSourceCap * compactFactor lines, bounding the append-only growth.
const compactFactor = 1.5

// Defaults for the in-memory caps when a caller passes a non-positive value.
const (
	DefaultPerSourceCap = 500
	DefaultGlobalCap    = 1500
)

// SourceCount is one source and how many events it currently holds in memory.
type SourceCount struct {
	Source string `json:"source"`
	Count  int    `json:"count"`
}

// Filter selects events for Query. Zero values mean "no constraint".
type Filter struct {
	Source string // exact source (sanitized before matching); empty = all sources
	Level  string // exact level
	Q      string // case-insensitive substring over title+message+component+tags
	Since  string // return only events with ID strictly greater than this (exclusive)
	Limit  int    // max events returned; <=0 or > globalCap means globalCap
}

// Store keeps a capped, time-ordered slice per source and appends each new
// event to that source's JSONL file.
type Store struct {
	mu           sync.Mutex
	dir          string
	perSourceCap int
	globalCap    int
	now          func() time.Time
	rand         io.Reader
	data         map[string]*[]event.Event // source -> events, oldest→newest, len ≤ perSourceCap
	diskLines    map[string]int            // estimated line count on disk per source
}

// New loads the store from <workdir>/sources/*.jsonl, creating the directory if
// needed. Non-positive caps fall back to the defaults; a nil clock uses the wall
// clock. A missing directory yields an empty store (not an error).
func New(workdir string, perSourceCap, globalCap int, now func() time.Time) (*Store, error) {
	if perSourceCap <= 0 {
		perSourceCap = DefaultPerSourceCap
	}
	if globalCap <= 0 {
		globalCap = DefaultGlobalCap
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	dir := filepath.Join(workdir, sourcesSubdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create workdir: %w", err)
	}
	s := &Store{
		dir:          dir,
		perSourceCap: perSourceCap,
		globalCap:    globalCap,
		now:          now,
		rand:         rand.Reader,
		data:         map[string]*[]event.Event{},
		diskLines:    map[string]int{},
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// load reads every sources/*.jsonl file, keeping the last perSourceCap events of
// each in memory. A malformed line is logged and skipped, not fatal.
func (s *Store) load() error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read sources dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		src := strings.TrimSuffix(e.Name(), ".jsonl")
		evs, lines, err := readJSONL(filepath.Join(s.dir, e.Name()))
		if err != nil {
			log.Printf("events: load %s: %v", e.Name(), err)
			continue
		}
		if len(evs) > s.perSourceCap {
			evs = evs[len(evs)-s.perSourceCap:]
		}
		buf := make([]event.Event, len(evs))
		copy(buf, evs)
		s.data[src] = &buf
		s.diskLines[src] = lines
	}
	return nil
}

// readJSONL parses one JSONL file, returning the valid events (oldest→newest as
// written) and the total count of non-empty lines (the on-disk estimate used for
// compaction). Malformed lines are logged and skipped.
func readJSONL(path string) ([]event.Event, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = f.Close() }()

	var evs []event.Event
	lines := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		lines++
		var ev event.Event
		if err := json.Unmarshal(line, &ev); err != nil {
			log.Printf("events: skip malformed line in %s: %v", filepath.Base(path), err)
			continue
		}
		evs = append(evs, ev)
	}
	if err := sc.Err(); err != nil {
		return nil, 0, fmt.Errorf("scan %s: %w", filepath.Base(path), err)
	}
	return evs, lines, nil
}

// Append validates ev, stamps Time and ID, appends it to the in-memory slice
// (evicting the oldest past perSourceCap), and appends one JSON line to the
// source file. When the file grows past perSourceCap * compactFactor lines it is
// rewritten atomically from the capped in-memory slice.
func (s *Store) Append(ev event.Event) (event.Event, error) {
	if err := ev.Validate(); err != nil {
		return event.Event{}, err
	}
	src, err := event.SanitizeSource(ev.Source)
	if err != nil {
		return event.Event{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ev.Time = s.now().UTC()
	ev.Level = event.NormalizeLevel(ev.Level)
	ev.Source = src
	ev.ID = event.NewID(ev.Time, s.rand)

	sl := s.data[src]
	if sl == nil {
		empty := make([]event.Event, 0, 1)
		sl = &empty
		s.data[src] = sl
	}
	*sl = append(*sl, ev)
	if len(*sl) > s.perSourceCap {
		*sl = (*sl)[len(*sl)-s.perSourceCap:]
	}

	if err := s.appendLine(src, ev); err != nil {
		return event.Event{}, err
	}
	s.diskLines[src]++
	if s.diskLines[src] > int(float64(s.perSourceCap)*compactFactor) {
		if err := s.compact(src); err != nil {
			return event.Event{}, err
		}
	}
	return ev, nil
}

// appendLine writes one JSON event line to the source file (O(1) append).
func (s *Store) appendLine(src string, ev event.Event) error {
	raw, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	raw = append(raw, '\n')
	f, err := os.OpenFile(s.filePath(src), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open source file: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(raw); err != nil {
		return fmt.Errorf("append event: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync source file: %w", err)
	}
	return nil
}

// compact rewrites a source file atomically (temp + rename) from the capped
// in-memory slice, shrinking it back to ≤ perSourceCap lines.
func (s *Store) compact(src string) error {
	var buf bytes.Buffer
	sl := s.data[src]
	if sl != nil {
		for _, ev := range *sl {
			raw, err := json.Marshal(ev)
			if err != nil {
				return fmt.Errorf("marshal event: %w", err)
			}
			buf.Write(raw)
			buf.WriteByte('\n')
		}
	}
	path := s.filePath(src)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write source file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("commit source file: %w", err)
	}
	if sl != nil {
		s.diskLines[src] = len(*sl)
	} else {
		s.diskLines[src] = 0
	}
	return nil
}

// Query merges events across sources (or one source if Filter.Source is set),
// applies the filters, and returns them newest-first, capped to the limit.
func (s *Store) Query(f Filter) []event.Event {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []event.Event
	collect := func(sl *[]event.Event) {
		for _, ev := range *sl {
			if f.Level != "" && ev.Level != f.Level {
				continue
			}
			if f.Since != "" && ev.ID <= f.Since {
				continue
			}
			if f.Q != "" && !matchQ(ev, f.Q) {
				continue
			}
			out = append(out, ev)
		}
	}

	if f.Source != "" {
		if src, err := event.SanitizeSource(f.Source); err == nil {
			if sl := s.data[src]; sl != nil {
				collect(sl)
			}
		}
	} else {
		for _, sl := range s.data {
			collect(sl)
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })

	limit := f.Limit
	if limit <= 0 || limit > s.globalCap {
		limit = s.globalCap
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// matchQ reports whether q (case-insensitive) is a substring of any searchable
// field: title, message, component, or any tag key/value.
func matchQ(ev event.Event, q string) bool {
	q = strings.ToLower(q)
	if strings.Contains(strings.ToLower(ev.Title), q) ||
		strings.Contains(strings.ToLower(ev.Message), q) ||
		strings.Contains(strings.ToLower(ev.Component), q) {
		return true
	}
	for k, v := range ev.Tags {
		if strings.Contains(strings.ToLower(k), q) || strings.Contains(strings.ToLower(v), q) {
			return true
		}
	}
	return false
}

// Purge removes events from one source and returns how many were dropped.
// An empty before removes the source entirely (memory + file); otherwise only
// events with ID <= before are dropped and the file is rewritten atomically.
// Purging an unknown source is a no-op, not an error.
func (s *Store) Purge(source, before string) (int, error) {
	src, err := event.SanitizeSource(source)
	if err != nil {
		return 0, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sl := s.data[src]
	if sl == nil {
		return 0, nil
	}

	if before == "" {
		n := len(*sl)
		delete(s.data, src)
		delete(s.diskLines, src)
		if err := os.Remove(s.filePath(src)); err != nil && !os.IsNotExist(err) {
			return 0, fmt.Errorf("remove source file: %w", err)
		}
		return n, nil
	}

	kept := (*sl)[:0]
	for _, ev := range *sl {
		if ev.ID > before {
			kept = append(kept, ev)
		}
	}
	n := len(*sl) - len(kept)
	*sl = kept
	if err := s.compact(src); err != nil {
		return 0, err
	}
	return n, nil
}

// Sources lists each source with its in-memory event count, sorted by name.
func (s *Store) Sources() []SourceCount {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SourceCount, 0, len(s.data))
	for src, sl := range s.data {
		n := 0
		if sl != nil {
			n = len(*sl)
		}
		out = append(out, SourceCount{Source: src, Count: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Source < out[j].Source })
	return out
}

func (s *Store) filePath(src string) string {
	return filepath.Join(s.dir, src+".jsonl")
}
