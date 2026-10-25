// Package ingest drains the spool into the store. It is the serve side of
// capture: the Ingester reads each envelope `moondb record` left, turns it
// into session, turn, edit and commit-link records, and deletes it once the
// store holds them. Ingest is idempotent on the envelope id, so an envelope
// replayed after a crash between append and delete changes nothing, and
// envelopes that arrive out of order (an edit before its turn opened, a
// close without an open) still leave whole records behind.
package ingest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// Ingester moves envelopes from a spool into a store. Serve runs one, and
// it is the store's only writer. Now stamps an envelope that carries no
// time and paces retries; nil means time.Now. Logf receives one line per
// envelope that fails, is moved aside, or loses a turn chunk; nil discards
// them.
type Ingester struct {
	Store store.Store
	Spool *spool.Reader
	Now   func() time.Time
	Logf  func(format string, args ...any)
}

// Ingest records one envelope. It first copies the chunks the envelope
// names from the spool into the store, then writes the records its kind
// calls for. A chunk in neither place fails an edit, whose content would be
// lost, and only drops the reference from a turn, which is whole without
// it. Ingest never removes the envelope; Run does that once Ingest returns
// nil.
func (in *Ingester) Ingest(ctx context.Context, env spool.Envelope) error {
	if env.At.IsZero() {
		env.At = in.now()
	}
	missing, err := in.copyChunks(ctx, env)
	if err != nil {
		return err
	}
	switch env.Kind {
	case spool.KindTurnOpen:
		return in.turnOpen(ctx, env, missing)
	case spool.KindTurnClose:
		return in.turnClose(ctx, env, missing)
	case spool.KindEdit:
		return in.edit(ctx, env)
	case spool.KindHeadMoved:
		return in.headMoved(ctx, env)
	default:
		return fmt.Errorf("ingest: envelope %s has unknown kind %q", env.ID, env.Kind)
	}
}

// copyChunks puts every chunk the envelope names into the store unless the
// store has it. It returns the keys found in neither place; for an edit
// envelope the first such key is an error instead.
func (in *Ingester) copyChunks(ctx context.Context, env spool.Envelope) (map[string]bool, error) {
	chunks := in.Store.Chunks()
	var missing map[string]bool
	for _, key := range env.ChunkKeys() {
		has, err := chunks.Has(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("ingest: envelope %s: look up chunk %s: %w", env.ID, key, err)
		}
		if has {
			continue
		}
		data, err := in.Spool.Chunk(key)
		if err != nil && env.Kind == spool.KindEdit {
			return nil, fmt.Errorf("ingest: edit %s: chunk is in neither spool nor store: %w",
				env.ID, err)
		}
		if err != nil {
			missing = setKey(missing, key)
			continue
		}
		got, err := chunks.Put(ctx, data)
		if err != nil {
			return nil, fmt.Errorf("ingest: envelope %s: store chunk %s: %w", env.ID, key, err)
		}
		if got != key {
			return nil, fmt.Errorf("ingest: envelope %s: chunk %s stored as %s", env.ID, key, got)
		}
	}
	return missing, nil
}

func setKey(m map[string]bool, key string) map[string]bool {
	if m == nil {
		m = map[string]bool{}
	}
	m[key] = true
	return m
}

// kept returns key, or "" with a warning when the chunk was lost, so a
// record never points at content the store does not have.
func (in *Ingester) kept(env spool.Envelope, missing map[string]bool, key string) string {
	if !missing[key] {
		return key
	}
	in.logf("ingest: %s %s: chunk %s is in neither spool nor store; recording without it",
		env.Kind, env.ID, key)
	return ""
}

// upsertSession creates the envelope's session, or refreshes the title,
// model and transcript path of a known one. Cwd and Harness keep their
// first value, since the session started there; StartedAt keeps the
// earliest time seen, so a session first known from a late envelope moves
// back when an earlier one arrives. A backfill only fills empty fields.
func (in *Ingester) upsertSession(ctx context.Context, env spool.Envelope) error {
	ref := env.Session
	if ref.ID == "" {
		return fmt.Errorf("ingest: envelope %s has no session id", env.ID)
	}
	cur, err := in.Store.Session(ctx, ref.ID)
	if errors.Is(err, store.ErrNotFound) {
		return in.putSession(ctx, store.Session{
			ID: ref.ID, Harness: env.Harness, Cwd: ref.Cwd, Title: ref.Title,
			Model: ref.Model, TranscriptPath: ref.TranscriptPath, StartedAt: env.At,
		})
	}
	if err != nil {
		return fmt.Errorf("ingest: load session %s: %w", ref.ID, err)
	}
	s := cur.Session
	set := setter(env)
	changed := fill(&s.Harness, env.Harness)
	changed = fill(&s.Cwd, ref.Cwd) || changed
	changed = set(&s.Title, ref.Title) || changed
	changed = set(&s.Model, ref.Model) || changed
	changed = set(&s.TranscriptPath, ref.TranscriptPath) || changed
	if !env.Backfill { // a stored StartedAt is never empty
		changed = earliest(&s.StartedAt, env.At) || changed
	}
	if !changed {
		return nil
	}
	return in.putSession(ctx, s)
}

func (in *Ingester) putSession(ctx context.Context, s store.Session) error {
	if err := in.Store.PutSession(ctx, s); err != nil {
		return fmt.Errorf("ingest: put session %s: %w", s.ID, err)
	}
	return nil
}

// loadTurn loads a turn; found is false when the store has none by id.
func (in *Ingester) loadTurn(ctx context.Context, id string) (store.TurnDetail, bool, error) {
	td, err := in.Store.Turn(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return store.TurnDetail{}, false, nil
	}
	if err != nil {
		return store.TurnDetail{}, false, fmt.Errorf("ingest: load turn %s: %w", id, err)
	}
	return td, true, nil
}

func (in *Ingester) putTurn(ctx context.Context, t store.Turn) error {
	if err := in.Store.PutTurn(ctx, t); err != nil {
		return fmt.Errorf("ingest: put turn %s: %w", t.ID, err)
	}
	return nil
}

func (in *Ingester) now() time.Time {
	if in.Now != nil {
		return in.Now()
	}
	return time.Now()
}

func (in *Ingester) logf(format string, args ...any) {
	if in.Logf != nil {
		in.Logf(format, args...)
	}
}

func errNoPayload(env spool.Envelope) error {
	return fmt.Errorf("ingest: %s envelope %s has no payload", env.Kind, env.ID)
}

// setter is how an envelope's value meets a stored one: a live envelope
// replaces it, a backfill only fills an empty field.
func setter(env spool.Envelope) func(dst *string, v string) bool {
	if env.Backfill {
		return fill
	}
	return replace
}

// fill sets an empty *dst to v and reports whether it changed.
func fill(dst *string, v string) bool {
	if *dst != "" || v == "" {
		return false
	}
	*dst = v
	return true
}

// replace sets *dst to a non-empty v and reports whether it changed.
func replace(dst *string, v string) bool {
	if v == "" || *dst == v {
		return false
	}
	*dst = v
	return true
}

// earliest moves *dst back to t when t is earlier and reports whether it
// changed.
func earliest(dst *time.Time, t time.Time) bool {
	if !t.Before(*dst) {
		return false
	}
	*dst = t
	return true
}
