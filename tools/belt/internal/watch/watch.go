// Package watch wakes an idle Claude Code session when an item addressed to
// it lands on the events service. `belt watch` runs it as an asyncRewake
// hook on Stop and SessionStart, so Claude Code starts a fresh watcher each
// time the session goes idle. A watcher that finds items hands them back to
// be printed on stderr with exit 2, which Claude Code shows the session as
// a system reminder and wakes it with.
//
// There is no store and no port: the events service is the inbox and the
// cursor is a cache file (docs/adr/0024). Delivery is at most once, because
// the cursor moves before the wake, and every failure ends as a watcher
// that wakes nobody.
package watch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mad01/thismoon/tools/belt/internal/cache"
)

// Tag keys an inbox item carries: to addresses it to a session id, from
// names the sender in the wake text.
const (
	TagTo   = "to"
	TagFrom = "from"
)

// Per-poll bounds. queryLimit caps one page of candidates; the q filter
// already narrows them to entries mentioning the session id, so a page this
// size is far more than one idle period produces. requestTimeout keeps one
// slow answer from eating several poll intervals.
const (
	queryLimit     = 100
	requestTimeout = 2 * time.Second
)

// Config is one watcher run. Every field is required; a run missing the
// session id or the cache files it needs returns at once.
type Config struct {
	BaseURL      string // events service
	Source       string // events-service source items arrive under
	SessionID    string // the session this watcher wakes
	PollInterval time.Duration
	MaxWait      time.Duration
	CacheDir     string // where the cursor and owner files live
	// Owner is this watcher's token in the owner file: the newest watcher
	// for a session writes its own, and an older one exits once the file
	// names someone else.
	Owner string
}

// Item is one delivery: an events entry addressed to the session.
type Item struct {
	ID      string
	From    string
	Title   string
	Message string
}

// entry is the subset of an events-service entry the watcher reads.
type entry struct {
	ID      string            `json:"id"`
	Title   string            `json:"title"`
	Message string            `json:"message"`
	Tags    map[string]string `json:"tags"`
}

// Run polls until items addressed to the session arrive, MaxWait passes, a
// newer watcher takes over, or ctx is done. It returns the items, oldest
// first, only after the cursor has moved past them; in every other case it
// returns nil. A cursor that cannot be written means no delivery at all: an
// item delivered without one would wake the session again on every Stop.
func Run(ctx context.Context, cfg Config) []Item {
	files := filesFor(cfg)
	if files.cursor == "" || cfg.BaseURL == "" || !files.claim(cfg.Owner) {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, cfg.MaxWait)
	defer cancel()
	for {
		if !files.ownedBy(cfg.Owner) {
			return nil
		}
		if items := poll(ctx, cfg, files.readCursor()); len(items) > 0 {
			return files.deliver(cfg.Owner, items)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(cfg.PollInterval):
		}
	}
}

// Render formats delivered items for the wake reminder, prefixed belt[watch]:
// like every other belt output the model reads.
func Render(items []Item) string {
	var b strings.Builder
	for i, it := range items {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "belt[watch]: from %s: %s", orUnknown(it.From), it.Title)
		if it.Message != "" {
			b.WriteString("\n" + it.Message)
		}
	}
	return b.String()
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown sender"
	}
	return s
}

// poll asks the events service once for entries past the cursor and keeps
// the ones addressed to the session. The q filter is a substring match over
// every field, so an entry that merely mentions the session id comes back
// too; the to tag is what addresses an item. The cursor is enforced here as
// well as sent as since: a service that ignored since would otherwise hand
// back a delivered item on every poll and wake the session on every Stop.
// Any failure reads as no items.
func poll(ctx context.Context, cfg Config, cursor string) []Item {
	entries, err := query(ctx, cfg, cursor)
	if err != nil {
		return nil
	}
	var items []Item
	for _, e := range entries {
		if e.Tags[TagTo] != cfg.SessionID || (cursor != "" && e.ID <= cursor) {
			continue
		}
		items = append(
			items,
			Item{ID: e.ID, From: e.Tags[TagFrom], Title: e.Title, Message: e.Message},
		)
	}
	// The service answers newest first; ids sort lexically by time.
	slices.SortFunc(items, func(a, b Item) int { return strings.Compare(a.ID, b.ID) })
	return items
}

// query is the one GET the watcher makes. The events service's own client
// is internal to that service, so this is a minimal copy of its query call.
func query(ctx context.Context, cfg Config, cursor string) ([]entry, error) {
	params := url.Values{
		"source": {cfg.Source},
		"q":      {cfg.SessionID},
		"limit":  {fmt.Sprint(queryLimit)},
	}
	if cursor != "" {
		params.Set("since", cursor)
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, cfg.BaseURL+"/api/events?"+params.Encode(), nil,
	)
	if err != nil {
		return nil, fmt.Errorf("watch: build query: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("watch: query events: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("watch: events service returned %d", resp.StatusCode)
	}
	var entries []entry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("watch: decode events: %w", err)
	}
	return entries, nil
}

// sessionFiles are the two cache files one session's watchers share: the
// cursor (the id of the newest item delivered) and the owner token.
type sessionFiles struct {
	cursor string
	owner  string
}

func filesFor(cfg Config) sessionFiles {
	cursor := cache.SessionFile(cfg.CacheDir, "watch-", cfg.SessionID)
	if cursor == "" {
		return sessionFiles{}
	}
	return sessionFiles{cursor: cursor, owner: cursor + ".pid"}
}

// deliver hands the items back only if this watcher still owns the session
// and the cursor moved past them. A newer watcher may have claimed the
// session while the poll was in flight; it will find the same items, and
// two watchers delivering them would wake the session twice.
func (f sessionFiles) deliver(owner string, items []Item) []Item {
	if !f.ownedBy(owner) || !f.writeCursor(items[len(items)-1].ID) {
		return nil
	}
	return items
}

// claim makes this watcher the session's owner, superseding any older one.
func (f sessionFiles) claim(owner string) bool {
	return writeAtomic(f.owner, owner) == nil
}

// ownedBy reports whether the owner file still names this watcher. An
// unreadable file reads as "not ours", which ends the watcher: it cannot
// tell whether a newer one took over.
func (f sessionFiles) ownedBy(owner string) bool {
	raw, err := os.ReadFile(f.owner)
	return err == nil && strings.TrimSpace(string(raw)) == owner
}

// readCursor returns the id of the newest item already delivered, "" when
// none has been: then every item addressed to the session is new.
func (f sessionFiles) readCursor() string {
	raw, err := os.ReadFile(f.cursor)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func (f sessionFiles) writeCursor(id string) bool {
	return writeAtomic(f.cursor, id) == nil
}

// writeAtomic replaces path with content through a rename, so a concurrent
// reader sees the old value or the new one, never a torn write.
func writeAtomic(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("watch: create cache dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("watch: create temp file: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.WriteString(content + "\n"); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("watch: write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("watch: close %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("watch: replace %s: %w", path, err)
	}
	return nil
}
