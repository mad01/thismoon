// Package store is the single source of truth for reminders: an in-memory map
// guarded by a mutex, persisted to one JSON file. The serve process owns a
// Store; the ticker and HTTP handlers mutate it through these methods, so there
// is exactly one writer and no file-lock contention.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// fileName is the JSON file under the workdir that holds all reminders.
const fileName = "reminders.json"

// ErrNotFound is returned when no reminder has the given id.
var ErrNotFound = errors.New("reminder: not found")

// Store holds the reminders and persists them atomically to disk.
type Store struct {
	mu   sync.Mutex
	path string
	now  func() time.Time
	data map[string]*Reminder
}

// New loads the store from <workdir>/reminders.json, creating the directory if
// needed. A missing file yields an empty store (not an error).
func New(workdir string) (*Store, error) {
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		return nil, fmt.Errorf("create workdir: %w", err)
	}
	s := &Store{
		path: filepath.Join(workdir, fileName),
		now:  func() time.Time { return time.Now().UTC() },
		data: map[string]*Reminder{},
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
	var list []*Reminder
	if err := json.Unmarshal(raw, &list); err != nil {
		return fmt.Errorf("parse store: %w", err)
	}
	for _, r := range list {
		s.data[r.ID] = r
	}
	return nil
}

// save writes the whole store atomically (temp file + rename). Callers hold s.mu.
func (s *Store) save() error {
	list := make([]*Reminder, 0, len(s.data))
	for _, r := range s.data {
		list = append(list, r)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Due.Before(list[j].Due) })

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

// CreateInput carries the fields needed to create a reminder. Due is absolute
// (callers resolve any relative "in 2h" form before reaching the store).
type CreateInput struct {
	Title  string
	Body   string
	Due    time.Time
	Repeat string
}

// Create validates and stores a new pending reminder.
func (s *Store) Create(in CreateInput) (Reminder, error) {
	if strings.TrimSpace(in.Title) == "" {
		return Reminder{}, errors.New("title is required")
	}
	if in.Due.IsZero() {
		return Reminder{}, errors.New("due time is required")
	}
	if _, err := ParseRepeat(in.Repeat); err != nil {
		return Reminder{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	r := &Reminder{
		ID:        NewID(),
		Title:     in.Title,
		Body:      in.Body,
		Due:       in.Due.UTC(),
		Repeat:    in.Repeat,
		Status:    StatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.data[r.ID] = r
	if err := s.save(); err != nil {
		delete(s.data, r.ID)
		return Reminder{}, err
	}
	return *r, nil
}

// Get returns a copy of the reminder with the given id.
func (s *Store) Get(id string) (Reminder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.data[id]
	if !ok {
		return Reminder{}, ErrNotFound
	}
	return *r, nil
}

// List returns reminders sorted by due time ascending. A non-empty statusFilter
// restricts the result to that status.
func (s *Store) List(statusFilter string) []Reminder {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Reminder, 0, len(s.data))
	for _, r := range s.data {
		if statusFilter != "" && r.Status != statusFilter {
			continue
		}
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Due.Equal(out[j].Due) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].Due.Before(out[j].Due)
	})
	return out
}

// Patch holds optional field updates; a nil field is left unchanged.
type Patch struct {
	Title  *string
	Body   *string
	Due    *time.Time
	Repeat *string
}

// Update applies a patch, re-arming a fired reminder when its due time moves to
// the future. It bumps UpdatedAt and persists.
func (s *Store) Update(id string, p Patch) (Reminder, error) {
	if p.Repeat != nil {
		if _, err := ParseRepeat(*p.Repeat); err != nil {
			return Reminder{}, err
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.data[id]
	if !ok {
		return Reminder{}, ErrNotFound
	}
	if p.Title != nil {
		if strings.TrimSpace(*p.Title) == "" {
			return Reminder{}, errors.New("title cannot be empty")
		}
		r.Title = *p.Title
	}
	if p.Body != nil {
		r.Body = *p.Body
	}
	if p.Due != nil {
		r.Due = p.Due.UTC()
	}
	if p.Repeat != nil {
		r.Repeat = *p.Repeat
	}
	// Moving the due time into the future re-arms a one-shot that already fired.
	if r.Status == StatusFired && r.Due.After(s.now()) {
		r.Status = StatusPending
		r.FiredAt = nil
	}
	r.UpdatedAt = s.now()
	if err := s.save(); err != nil {
		return Reminder{}, err
	}
	return *r, nil
}

// Cancel soft-cancels a reminder: it keeps the record but stops it firing.
func (s *Store) Cancel(id string) (Reminder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.data[id]
	if !ok {
		return Reminder{}, ErrNotFound
	}
	r.Status = StatusCancelled
	r.UpdatedAt = s.now()
	if err := s.save(); err != nil {
		return Reminder{}, err
	}
	return *r, nil
}

// Delete permanently removes a reminder (web-only; not exposed over MCP).
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[id]; !ok {
		return ErrNotFound
	}
	delete(s.data, id)
	return s.save()
}

// DueReminders returns copies of every pending reminder whose due time has
// passed, soonest first — the set the ticker should fire this cycle.
func (s *Store) DueReminders(now time.Time) []Reminder {
	s.mu.Lock()
	defer s.mu.Unlock()
	var due []Reminder
	for _, r := range s.data {
		if r.Status == StatusPending && !r.Due.After(now) {
			due = append(due, *r)
		}
	}
	sort.Slice(due, func(i, j int) bool { return due[i].Due.Before(due[j].Due) })
	return due
}

// Trigger records that a reminder fired at now. A recurring reminder advances
// to its next future occurrence and stays pending; a one-shot becomes "fired".
// It is a no-op (no error) for a reminder that is no longer pending, so a fire
// that races a cancel does not resurrect it.
func (s *Store) Trigger(id string, now time.Time) (Reminder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.data[id]
	if !ok {
		return Reminder{}, ErrNotFound
	}
	if r.Status != StatusPending {
		return *r, nil
	}
	if r.Repeat != "" {
		next, err := NextDue(r.Due, r.Repeat, now)
		if err != nil {
			return Reminder{}, err
		}
		r.Due = next
	} else {
		r.Status = StatusFired
		fired := now.UTC()
		r.FiredAt = &fired
	}
	r.UpdatedAt = now.UTC()
	if err := s.save(); err != nil {
		return Reminder{}, err
	}
	return *r, nil
}
