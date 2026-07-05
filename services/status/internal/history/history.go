// Package history persists per-service, per-day check counts so the page can
// draw uptime bars across restarts. One JSON file, written atomically after
// each poll cycle.
package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const dateLayout = "2006-01-02"

// DayCount is the raw tally for one service on one day.
type DayCount struct {
	OK   int `json:"ok"`
	Fail int `json:"fail"`
}

// Day is a derived per-day stat for rendering.
type Day struct {
	Date    string  `json:"date"`
	OK      int     `json:"ok"`
	Fail    int     `json:"fail"`
	HasData bool    `json:"has_data"`
	Pct     float64 `json:"pct"` // 0 when HasData is false
}

// Store holds day buckets keyed by service label then date.
type Store struct {
	mu   sync.Mutex
	path string
	data map[string]map[string]*DayCount
}

// Load reads the store from path; a missing file yields an empty store.
func Load(path string) (*Store, error) {
	s := &Store{path: path, data: map[string]map[string]*DayCount{}}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(raw, &s.data); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return s, nil
}

// Record tallies one check outcome into now's day bucket.
func (s *Store) Record(label string, now time.Time, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	day := now.Format(dateLayout)
	if s.data[label] == nil {
		s.data[label] = map[string]*DayCount{}
	}
	if s.data[label][day] == nil {
		s.data[label][day] = &DayCount{}
	}
	if ok {
		s.data[label][day].OK++
	} else {
		s.data[label][day].Fail++
	}
}

// Prune drops buckets older than keepDays before now; services left with no
// buckets disappear entirely (e.g. a removed t-man agent).
func (s *Store) Prune(now time.Time, keepDays int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := now.AddDate(0, 0, -keepDays).Format(dateLayout)
	for label, days := range s.data {
		for day := range days {
			if day < cutoff {
				delete(days, day)
			}
		}
		if len(days) == 0 {
			delete(s.data, label)
		}
	}
}

// Save writes the store atomically (temp file + rename).
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(s.path), err)
	}
	raw, err := json.MarshalIndent(s.data, "", " ")
	if err != nil {
		return fmt.Errorf("marshal history: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("rename %s: %w", s.path, err)
	}
	return nil
}

// Days returns the last n days for a service, oldest first, ending today.
func (s *Store) Days(label string, now time.Time, n int) []Day {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Day, 0, n)
	for i := n - 1; i >= 0; i-- {
		date := now.AddDate(0, 0, -i).Format(dateLayout)
		d := Day{Date: date}
		if c := s.data[label][date]; c != nil && c.OK+c.Fail > 0 {
			d.OK, d.Fail = c.OK, c.Fail
			d.HasData = true
			d.Pct = 100 * float64(c.OK) / float64(c.OK+c.Fail)
		}
		out = append(out, d)
	}
	return out
}

// Uptime aggregates the last n days into one percentage. ok is false when no
// checks were recorded in the window.
func (s *Store) Uptime(label string, now time.Time, n int) (pct float64, ok bool) {
	var okN, failN int
	for _, d := range s.Days(label, now, n) {
		okN += d.OK
		failN += d.Fail
	}
	if okN+failN == 0 {
		return 0, false
	}
	return 100 * float64(okN) / float64(okN+failN), true
}
