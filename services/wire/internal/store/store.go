// Package store is wire's single source of truth for channels and messages:
// an in-memory index guarded by a mutex, persisted as append-only JSONL.
// Channel records are re-appended whole whenever they change and the newest
// record per id wins at load; messages are immutable, so their file order is
// their order and their position is their sequence number. The serve process
// owns a Store and is the only writer — the MCP server and the CLI reach it
// over HTTP — so there is exactly one writer and no file locks.
//
// Readers can block. Wait parks a caller until a message lands past its
// cursor, the channel closes, or a timeout expires, which is what lets one
// session wait for another session's reply in a single call instead of
// spinning on reads. The same primitive drives the web page's event stream.
package store

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	channelsFileName = "channels.jsonl"
	messagesDirName  = "messages"
)

var (
	// ErrNotFound is returned when no channel has the given id or name.
	ErrNotFound = errors.New("channel not found")
	// ErrNameTaken is returned when opening a channel with a name in use.
	ErrNameTaken = errors.New("channel name already taken")
	// ErrClosed is returned when posting to a closed channel.
	ErrClosed = errors.New("channel is closed")
)

// Store holds the channels and their messages and persists both to JSONL.
type Store struct {
	mu       sync.Mutex
	dir      string
	now      func() time.Time
	rnd      io.Reader
	channels map[string]*Channel
	byName   map[string]string
	messages map[string][]Message
	// watchers holds one broadcast channel per conversation, closed and
	// dropped whenever that conversation changes. A waiter takes the current
	// one, releases the mutex, and blocks on the close.
	watchers map[string]chan struct{}
}

// New loads the store from the JSONL logs under workdir, creating the
// directories if needed. Missing files yield an empty store, not an error.
func New(workdir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(workdir, messagesDirName), 0o755); err != nil {
		return nil, fmt.Errorf("create workdir: %w", err)
	}
	s := &Store{
		dir:      workdir,
		now:      func() time.Time { return time.Now().UTC() },
		rnd:      rand.Reader,
		channels: map[string]*Channel{},
		byName:   map[string]string{},
		messages: map[string][]Message{},
		watchers: map[string]chan struct{}{},
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// load reads the channel log, newest record per id winning, then each
// channel's message log. A message file with no channel record is ignored:
// the channel log is the index, and a stray file is not an invitation to
// invent one.
func (s *Store) load() error {
	err := scanJSONL(filepath.Join(s.dir, channelsFileName), channelsFileName,
		func(c Channel) {
			if cur, ok := s.channels[c.ID]; !ok || !c.UpdatedAt.Before(cur.UpdatedAt) {
				rec := c
				s.channels[c.ID] = &rec
			}
		})
	if err != nil {
		return err
	}
	for id, c := range s.channels {
		s.byName[c.Name] = id
		name := messagesFileName(id)
		err := scanJSONL(s.messagePath(id), name, func(m Message) {
			s.messages[id] = append(s.messages[id], m)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func messagesFileName(channelID string) string { return channelID + ".jsonl" }

func (s *Store) messagePath(channelID string) string {
	return filepath.Join(s.dir, messagesDirName, messagesFileName(channelID))
}

// OpenInput carries the fields needed to open a channel. An empty Name gets a
// generated one; the caller does not have to invent a unique handle.
type OpenInput struct {
	Name string
	Topic string
	From string
	// Conventions is the opener's ground rules for the conversation, carried
	// on the channel so a late joiner sees them without reading from seq 1.
	Conventions string
}

// Open creates a channel. A name already in use is ErrNameTaken rather than a
// silent join: two sessions meant to share a channel should share the handle,
// not race to create it.
func (s *Store) Open(in OpenInput) (Channel, error) {
	name, err := NormalizeName(in.Name)
	if err != nil {
		return Channel{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if name == "" {
		name = s.freeNameLocked()
	} else if _, taken := s.byName[name]; taken {
		return Channel{}, fmt.Errorf("%w: %s", ErrNameTaken, name)
	}

	now := s.now()
	c := &Channel{
		ID:          s.freeIDLocked(),
		Name:        name,
		Topic:       in.Topic,
		OpenedBy:    in.From,
		Conventions: in.Conventions,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := appendJSONL(filepath.Join(s.dir, channelsFileName), channelsFileName, c); err != nil {
		return Channel{}, err
	}
	s.channels[c.ID] = c
	s.byName[c.Name] = c.ID
	return *c, nil
}

// freeIDLocked mints a channel id that is not already in use.
func (s *Store) freeIDLocked() string {
	for {
		if id := newID(s.rnd); s.channels[id] == nil {
			return id
		}
	}
}

// freeNameLocked mints a fallback channel name that is not already in use.
func (s *Store) freeNameLocked() string {
	for {
		if n := newName(s.rnd); s.byName[n] == "" {
			return n
		}
	}
}

// resolveLocked finds a channel by id or by name and returns a copy. Ids and
// names live in disjoint spaces (an id starts with "ch_", which no name may
// contain), so a ref never matches both.
func (s *Store) resolveLocked(ref string) (Channel, error) {
	if c, ok := s.channels[ref]; ok {
		return *c, nil
	}
	name, err := NormalizeName(ref)
	if err != nil {
		return Channel{}, fmt.Errorf("%w: %s", ErrNotFound, ref)
	}
	if id, ok := s.byName[name]; ok {
		return *s.channels[id], nil
	}
	return Channel{}, fmt.Errorf("%w: %s", ErrNotFound, ref)
}

// Resolve returns the channel with the given id or name.
func (s *Store) Resolve(ref string) (Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resolveLocked(ref)
}

// Get returns a channel with its derived counts and participants.
func (s *Store) Get(ref string) (Summary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.resolveLocked(ref)
	if err != nil {
		return Summary{}, err
	}
	return summarize(c, s.messages[c.ID]), nil
}

// List returns every channel as a summary, most recently active first. Closed
// channels are left out unless includeClosed is set.
func (s *Store) List(includeClosed bool) []Summary {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Summary, 0, len(s.channels))
	for id, c := range s.channels {
		if c.Closed() && !includeClosed {
			continue
		}
		out = append(out, summarize(*c, s.messages[id]))
	}
	sort.Slice(out, func(i, j int) bool {
		ai, aj := out[i].activityAt(), out[j].activityAt()
		if ai.Equal(aj) {
			return out[i].ID > out[j].ID
		}
		return ai.After(aj)
	})
	return out
}

// PostInput carries one message. From and Body are required: an empty body
// says nothing, and an unsigned message cannot be answered. Kind, ReplyTo,
// and ReplyNeeded are the optional protocol fields that keep an interleaved
// transcript followable without conventions living in prose.
type PostInput struct {
	From string
	Body string
	// Kind classifies intent from the closed set: task, result, question,
	// answer, or ack. Empty is a plain message.
	Kind string
	// ReplyTo names the seq this message answers; it must exist on the
	// channel. Zero means the message stands alone.
	ReplyTo int64
	// ReplyNeeded marks that the sender expects an answer.
	ReplyNeeded bool
}

// Post appends a message to a channel and wakes everyone waiting on it. The
// message's sequence number is its position in the channel, starting at 1.
func (s *Store) Post(ref string, in PostInput) (Message, error) {
	from, err := checkFrom(in.From)
	if err != nil {
		return Message{}, err
	}
	body, err := checkBody(in.Body)
	if err != nil {
		return Message{}, err
	}
	kind, err := checkKind(in.Kind)
	if err != nil {
		return Message{}, err
	}
	if in.ReplyTo < 0 {
		return Message{}, fmt.Errorf("invalid reply_to %d: want a message seq", in.ReplyTo)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	c, err := s.resolveLocked(ref)
	if err != nil {
		return Message{}, err
	}
	if c.Closed() {
		return Message{}, fmt.Errorf("%w: %s", ErrClosed, c.Name)
	}
	if last := int64(len(s.messages[c.ID])); in.ReplyTo > last {
		return Message{}, fmt.Errorf(
			"reply_to %d names no message on %s (last seq is %d)", in.ReplyTo, c.Name, last)
	}

	m := Message{
		ChannelID:   c.ID,
		Seq:         int64(len(s.messages[c.ID])) + 1,
		From:        from,
		Body:        body,
		Kind:        kind,
		ReplyTo:     in.ReplyTo,
		ReplyNeeded: in.ReplyNeeded,
		CreatedAt:   s.now(),
	}
	if err := appendJSONL(s.messagePath(c.ID), messagesFileName(c.ID), m); err != nil {
		return Message{}, err
	}
	s.messages[c.ID] = append(s.messages[c.ID], m)
	s.notifyLocked(c.ID)
	return m, nil
}

// Close ends a conversation: no further messages, and every waiter wakes so
// none of them blocks for a reply that will never come. Closing is terminal
// and idempotent — a second close keeps the original note.
func (s *Store) Close(ref, note string) (Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, err := s.resolveLocked(ref)
	if err != nil {
		return Channel{}, err
	}
	if c.Closed() {
		return c, nil
	}
	now := s.now()
	c.ClosedAt = &now
	c.CloseNote = note
	c.UpdatedAt = now
	if err := appendJSONL(filepath.Join(s.dir, channelsFileName), channelsFileName, c); err != nil {
		return Channel{}, err
	}
	rec := c
	s.channels[c.ID] = &rec
	s.notifyLocked(c.ID)
	return c, nil
}

// Batch is one read's result: the channel as it stands, the messages after the
// cursor the reader gave, the cursor to resume from next time, and the seqs
// still owed an answer — so a reader knows what it must respond to without
// re-reading the whole transcript.
type Batch struct {
	Channel       Channel   `json:"channel"`
	Messages      []Message `json:"messages"`
	Cursor        int64     `json:"cursor"`
	AwaitingReply []int64   `json:"awaiting_reply,omitempty"`
}

// Read returns the messages after the given cursor without blocking. A cursor
// of 0 reads from the start; limit of 0 means no limit.
func (s *Store) Read(ref string, since int64, limit int) (Batch, error) {
	return s.Wait(context.Background(), ref, since, limit, 0)
}

// Wait returns the messages after the given cursor, blocking up to timeout for
// the first one to arrive. It returns as soon as there is anything to report,
// and immediately (with whatever is there) when the channel is closed, the
// timeout expires, or ctx is cancelled — so a caller waiting on a peer that
// never answers still gets an answer of its own.
func (s *Store) Wait(
	ctx context.Context,
	ref string,
	since int64,
	limit int,
	timeout time.Duration,
) (Batch, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	for {
		s.mu.Lock()
		c, err := s.resolveLocked(ref)
		if err != nil {
			s.mu.Unlock()
			return Batch{}, err
		}
		all := s.messages[c.ID]
		msgs := messagesSince(all, since, limit)
		if len(msgs) > 0 || c.Closed() || timeout <= 0 {
			s.mu.Unlock()
			return Batch{
				Channel:       c,
				Messages:      msgs,
				Cursor:        cursorAfter(len(all), since, msgs),
				AwaitingReply: awaitingReply(all),
			}, nil
		}
		changed := s.watcherLocked(c.ID)
		s.mu.Unlock()

		select {
		case <-changed:
		case <-deadline.C:
			return Batch{
				Channel:       c,
				Cursor:        cursorAfter(len(all), since, nil),
				AwaitingReply: awaitingReply(all),
			}, nil
		case <-ctx.Done():
			return Batch{Channel: c, Cursor: cursorAfter(len(all), since, nil)}, ctx.Err()
		}
	}
}

// cursorAfter is where the reader should resume: the last message it just
// received, or its own cursor clamped to what the channel actually holds (a
// reader that asked from beyond the end is walked back rather than left
// pinned at an impossible position).
func cursorAfter(total int, since int64, msgs []Message) int64 {
	if n := len(msgs); n > 0 {
		return msgs[n-1].Seq
	}
	if since > int64(total) {
		return int64(total)
	}
	if since < 0 {
		return 0
	}
	return since
}

// messagesSince copies the messages past a cursor. Sequence numbers start at 1
// and have no gaps, so the cursor is also the index to slice from.
func messagesSince(all []Message, since int64, limit int) []Message {
	if since < 0 {
		since = 0
	}
	if since >= int64(len(all)) {
		return nil
	}
	out := all[since:]
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return append([]Message(nil), out...)
}

// watcherLocked returns the broadcast channel for a conversation, creating it
// on first use.
func (s *Store) watcherLocked(channelID string) chan struct{} {
	w, ok := s.watchers[channelID]
	if !ok {
		w = make(chan struct{})
		s.watchers[channelID] = w
	}
	return w
}

// notifyLocked wakes every waiter on a conversation by closing its broadcast
// channel and dropping it; the next waiter makes a fresh one.
func (s *Store) notifyLocked(channelID string) {
	if w, ok := s.watchers[channelID]; ok {
		close(w)
		delete(s.watchers, channelID)
	}
}
