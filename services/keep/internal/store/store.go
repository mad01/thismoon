// Package store is the single source of truth for assertions: an in-memory map
// guarded by a mutex, persisted as an append-only JSONL log. Every mutation
// appends one complete record as one line; a written line is never rewritten.
// Load resolves the newest record per id: greater updated_at wins, and a tie
// goes to the later line. The serve process owns a Store and is the only
// writer; the MCP server and CLI reach it over HTTP, so there is exactly one
// writer and no file-lock contention.
//
// Check appends each changed record independently, so a failure mid-run leaves
// the earlier appends persisted. That is deliberate: status and checked_at are
// recomputable by re-running check, which costs less than multi-line rollback.
package store

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mad01/thismoon/services/keep/internal/pin"
)

const (
	// logFileName holds every assertion except machine-scoped ones; it is the
	// file federation will sync. localFileName holds machine:-subject records
	// and never leaves the machine.
	logFileName   = "assertions.jsonl"
	localFileName = "local.jsonl"

	// legacyFileName is the pre-JSONL single-array store, split into the log
	// on startup and renamed to migratedFileName as a backup.
	legacyFileName   = "assertions.json"
	migratedFileName = "assertions.json.migrated"

	// localSubjectPrefix routes a record to localFileName at write time.
	// Subjects are immutable, so a record never moves between files.
	localSubjectPrefix = "machine:"
)

// logf reports non-fatal store conditions (torn lines, stray legacy files);
// tests swap it to capture warnings.
var logf = log.Printf

// ErrNotFound is returned when no assertion has the given id.
var ErrNotFound = errors.New("assertion: not found")

// Store holds the assertions and persists them to the JSONL log.
type Store struct {
	mu       sync.Mutex
	dir      string
	now      func() time.Time
	rnd      io.Reader
	checkPin func(pin.Pin) (bool, string)
	data     map[string]*Assertion
}

// New loads the store from the JSONL log under workdir, creating the directory
// if needed and first migrating a legacy assertions.json array when one is
// present. Missing files yield an empty store (not an error).
func New(workdir string) (*Store, error) {
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		return nil, fmt.Errorf("create workdir: %w", err)
	}
	s := &Store{
		dir:      workdir,
		now:      func() time.Time { return time.Now().UTC() },
		rnd:      rand.Reader,
		checkPin: pin.Check,
		data:     map[string]*Assertion{},
	}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// migrate splits a legacy assertions.json array into per-record log lines
// (oldest first, so the log reads chronologically) and renames the legacy file
// to keep it as a backup. When a log file already exists the log wins: the
// legacy file is left alone with a warning.
func (s *Store) migrate() error {
	legacy := filepath.Join(s.dir, legacyFileName)
	if _, err := os.Stat(legacy); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("stat legacy store: %w", err)
	}
	for _, name := range []string{logFileName, localFileName} {
		if _, err := os.Stat(filepath.Join(s.dir, name)); err == nil {
			logf(
				"keep: stray %s next to %s; the log wins, leaving the legacy file alone",
				legacyFileName,
				name,
			)
			return nil
		}
	}
	raw, err := os.ReadFile(legacy)
	if err != nil {
		return fmt.Errorf("read legacy store: %w", err)
	}
	var list []*Assertion
	if err := json.Unmarshal(raw, &list); err != nil {
		return fmt.Errorf("parse legacy store: %w", err)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	for _, a := range list {
		if err := s.appendOne(a); err != nil {
			return fmt.Errorf("migrate %s: %w", a.ID, err)
		}
	}
	if err := os.Rename(legacy, filepath.Join(s.dir, migratedFileName)); err != nil {
		return fmt.Errorf("back up legacy store: %w", err)
	}
	return nil
}

func (s *Store) load() error {
	for _, name := range []string{logFileName, localFileName} {
		if err := s.loadFile(name); err != nil {
			return err
		}
	}
	return nil
}

// loadFile folds one log file into the map, newest record per id winning. An
// unparseable line is fatal with its file and line number — except a torn
// final line without its newline, the crash artifact of an interrupted append:
// that one is dropped and truncated away. A parseable final line that only
// lacks its newline is kept and repaired, so the next append cannot merge into
// it. Repairs are safe here because load runs before any writer exists.
func (s *Store) loadFile(name string) error {
	path := filepath.Join(s.dir, name)
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer f.Close()

	r := bufio.NewReader(f)
	var offset int64
	for lineNo := 1; ; lineNo++ {
		line, err := r.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("read %s: %w", name, err)
		}
		complete := strings.HasSuffix(line, "\n")
		if text := strings.TrimSpace(line); text != "" {
			var a Assertion
			switch uerr := json.Unmarshal([]byte(text), &a); {
			case uerr == nil:
				if cur, ok := s.data[a.ID]; !ok || !a.UpdatedAt.Before(cur.UpdatedAt) {
					rec := a
					s.data[a.ID] = &rec
				}
				if !complete {
					if aerr := appendNewline(path); aerr != nil {
						return fmt.Errorf("repair %s: %w", name, aerr)
					}
				}
			case !complete:
				logf("keep: dropping torn final line %s:%d (interrupted append)", name, lineNo)
				if terr := os.Truncate(path, offset); terr != nil {
					return fmt.Errorf("repair %s: %w", name, terr)
				}
			default:
				return fmt.Errorf("parse %s:%d: %w", name, lineNo, uerr)
			}
		}
		offset += int64(len(line))
		if err != nil { // io.EOF after the final line
			return nil
		}
	}
}

// appendNewline terminates a final line that parsed but lost its newline.
func appendNewline(path string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write([]byte("\n"))
	return err
}

// appendOne persists a single record as one JSONL line, routed by its subject.
// The append is the atomic unit, so there is no temp+rename. Callers hold s.mu
// (or, in New, have exclusive access).
func (s *Store) appendOne(a *Assertion) error {
	raw, err := json.Marshal(a)
	if err != nil {
		return fmt.Errorf("marshal assertion: %w", err)
	}
	name := logFileName
	if strings.HasPrefix(a.Subject, localSubjectPrefix) {
		name = localFileName
	}
	f, err := os.OpenFile(filepath.Join(s.dir, name), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return fmt.Errorf("append assertion: %w", err)
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
	if err := s.appendOne(a); err != nil {
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
	if err := s.appendOne(a); err != nil {
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
// the failing pin's location. Each checked record is appended to the log as it
// goes; a failure mid-run leaves the earlier appends persisted, which is fine
// because a re-run recomputes them (see the package doc).
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
		if err := s.appendOne(a); err != nil {
			return CheckReport{}, err
		}

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
	return report, nil
}
