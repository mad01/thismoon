// Package store is the single source of truth for assertions: an in-memory map
// guarded by a mutex, persisted to one JSON file. The serve process owns a
// Store and is the only writer; the MCP server and CLI reach it over HTTP, so
// there is exactly one writer and no file-lock contention.
package store

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mad01/thismoon/services/keep/internal/pin"
)

// fileName is the JSON file under the workdir that holds all assertions.
const fileName = "assertions.json"

// ErrNotFound is returned when no assertion has the given id.
var ErrNotFound = errors.New("assertion: not found")

// Store holds the assertions and persists them atomically to disk.
type Store struct {
	mu       sync.Mutex
	path     string
	now      func() time.Time
	rnd      io.Reader
	checkPin func(pin.Pin) (bool, string)
	data     map[string]*Assertion
}

// New loads the store from <workdir>/assertions.json, creating the directory if
// needed. A missing file yields an empty store (not an error).
func New(workdir string) (*Store, error) {
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		return nil, fmt.Errorf("create workdir: %w", err)
	}
	s := &Store{
		path:     filepath.Join(workdir, fileName),
		now:      func() time.Time { return time.Now().UTC() },
		rnd:      rand.Reader,
		checkPin: pin.Check,
		data:     map[string]*Assertion{},
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read store: %w", err)
	}
	var list []*Assertion
	if err := json.Unmarshal(raw, &list); err != nil {
		return fmt.Errorf("parse store: %w", err)
	}
	for _, a := range list {
		s.data[a.ID] = a
	}
	return nil
}

// save writes the whole store atomically (temp file + rename), newest first.
// Callers hold s.mu.
func (s *Store) save() error {
	list := make([]*Assertion, 0, len(s.data))
	for _, a := range s.data {
		list = append(list, a)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID > list[j].ID })

	raw, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal store: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("write store: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("commit store: %w", err)
	}
	return nil
}

// AssertInput carries the fields needed to record an assertion. Pins are
// already resolved — the server resolves refs before reaching the store.
type AssertInput struct {
	Kind       string
	Subject    string
	Statement  string
	Confidence string
	Links      []string
	SessionID  string
	CostTokens int
	Pins       []pin.Pin
}

// Assert validates and stores a new fresh assertion.
func (s *Store) Assert(in AssertInput) (Assertion, error) {
	if !ValidKind(in.Kind) {
		return Assertion{}, fmt.Errorf("invalid kind %q", in.Kind)
	}
	if !ValidConfidence(in.Confidence) {
		return Assertion{}, fmt.Errorf("invalid confidence %q", in.Confidence)
	}
	if strings.TrimSpace(in.Subject) == "" {
		return Assertion{}, errors.New("subject is required")
	}
	if strings.TrimSpace(in.Statement) == "" {
		return Assertion{}, errors.New("statement is required")
	}
	if len(in.Pins) == 0 {
		return Assertion{}, errors.New("at least one evidence pin is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	a := &Assertion{
		ID:         NewID(now, s.rnd),
		Kind:       in.Kind,
		Subject:    in.Subject,
		Statement:  in.Statement,
		Pins:       in.Pins,
		Confidence: in.Confidence,
		Provenance: Provenance{
			SessionID:  in.SessionID,
			DerivedAt:  now,
			CostTokens: in.CostTokens,
		},
		Status:    StatusFresh,
		Links:     in.Links,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.data[a.ID] = a
	if err := s.save(); err != nil {
		delete(s.data, a.ID)
		return Assertion{}, err
	}
	return *a, nil
}

// Get returns a copy of the assertion with the given id.
func (s *Store) Get(id string) (Assertion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.data[id]
	if !ok {
		return Assertion{}, ErrNotFound
	}
	return *a, nil
}

// Filter restricts List. Subject is a prefix match; Kind and Status are exact.
// An empty field matches everything.
type Filter struct {
	Subject string
	Kind    string
	Status  string
}

// List returns copies of the assertions matching f, newest first.
func (s *Store) List(f Filter) []Assertion {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Assertion, 0, len(s.data))
	for _, a := range s.data {
		if f.Subject != "" && !strings.HasPrefix(a.Subject, f.Subject) {
			continue
		}
		if f.Kind != "" && a.Kind != f.Kind {
			continue
		}
		if f.Status != "" && a.Status != f.Status {
			continue
		}
		out = append(out, *a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

// Retract withdraws an assertion. A fresh or stale assertion becomes retracted
// with the note and timestamp recorded; an already-retracted one is returned
// unchanged, so a repeat retract does not overwrite the original note.
func (s *Store) Retract(id, note string) (Assertion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.data[id]
	if !ok {
		return Assertion{}, ErrNotFound
	}
	if a.Status == StatusRetracted {
		return *a, nil
	}
	now := s.now()
	a.Status = StatusRetracted
	a.RetractNote = note
	a.RetractedAt = &now
	a.UpdatedAt = now
	if err := s.save(); err != nil {
		return Assertion{}, err
	}
	return *a, nil
}

// CheckReport summarizes a Check run: how many assertions were re-checked and
// their resulting status counts, with Flipped counting status changes.
type CheckReport struct {
	Checked    int
	Fresh      int
	Stale      int
	Flipped    int
	Assertions []Assertion
}

// Check re-verifies pins and updates status. An empty id checks every
// non-retracted assertion; a specific id checks just that one (ErrNotFound if
// absent). A retracted assertion is skipped: it is reported untouched and does
// not count as checked. For each checked assertion every pin is re-run; all
// holding leaves it fresh, the first failure marks it stale with the reason and
// the failing pin's location. The whole run persists once at the end.
func (s *Store) Check(id string) (CheckReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var targets []*Assertion
	if id == "" {
		for _, a := range s.data {
			targets = append(targets, a)
		}
	} else {
		a, ok := s.data[id]
		if !ok {
			return CheckReport{}, ErrNotFound
		}
		targets = append(targets, a)
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].ID > targets[j].ID })

	now := s.now()
	var report CheckReport
	changed := false
	for _, a := range targets {
		if a.Status == StatusRetracted {
			report.Assertions = append(report.Assertions, *a)
			continue
		}
		prev := a.Status
		status, reason := StatusFresh, ""
		for _, p := range a.Pins {
			if ok, r := s.checkPin(p); !ok {
				status = StatusStale
				reason = fmt.Sprintf("%s (%s:%d-%d)", r, p.File, p.StartLine, p.EndLine)
				break
			}
		}
		a.Status = status
		a.StaleReason = reason
		a.CheckedAt = &now
		a.UpdatedAt = now
		changed = true

		report.Checked++
		if status == StatusFresh {
			report.Fresh++
		} else {
			report.Stale++
		}
		if status != prev {
			report.Flipped++
		}
		report.Assertions = append(report.Assertions, *a)
	}

	if changed {
		if err := s.save(); err != nil {
			return CheckReport{}, err
		}
	}
	return report, nil
}
