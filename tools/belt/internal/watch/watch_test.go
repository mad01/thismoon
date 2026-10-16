package watch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	session = "s-1"
	inbox   = "belt-inbox"
	// Short enough to keep the suite fast, long enough for several polls.
	pollEvery = 10 * time.Millisecond
	shortWait = 150 * time.Millisecond
)

// fakeEvents serves GET /api/events the way the events service does: the
// source and q filters, since as an exclusive id cursor, newest first.
type fakeEvents struct {
	mu      sync.Mutex
	entries []entry // oldest first
	source  map[string]string
	// ignoreSince makes the fake answer as if it did not know the since
	// cursor, returning delivered entries again.
	ignoreSince bool
	// onQuery, when set, runs as each query arrives, before the answer:
	// the seam for something happening while a poll is in flight.
	onQuery func()
}

func newFakeEvents(t *testing.T) (*fakeEvents, *httptest.Server) {
	t.Helper()
	f := &fakeEvents{source: map[string]string{}}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeEvents) add(source string, e entry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, e)
	f.source[e.ID] = source
}

func (f *fakeEvents) serve(w http.ResponseWriter, r *http.Request) {
	if f.onQuery != nil {
		f.onQuery()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	q := r.URL.Query()
	var out []entry
	for _, e := range f.entries {
		since := q.Get("since")
		if f.ignoreSince {
			since = ""
		}
		if f.source[e.ID] != q.Get("source") || (since != "" && e.ID <= since) {
			continue
		}
		if !mentions(e, q.Get("q")) {
			continue
		}
		out = append(out, e)
	}
	slices.Reverse(out)
	_ = json.NewEncoder(w).Encode(out)
}

// mentions is the service's q filter: a substring over title, message and
// every tag value.
func mentions(e entry, q string) bool {
	if strings.Contains(e.Title, q) || strings.Contains(e.Message, q) {
		return true
	}
	for _, v := range e.Tags {
		if strings.Contains(v, q) {
			return true
		}
	}
	return false
}

func addressed(id, title string) entry {
	return entry{
		ID: id, Title: title, Message: "details of " + title,
		Tags: map[string]string{TagTo: session, TagFrom: "orchestrator"},
	}
}

func testConfig(base, cacheDir, owner string) Config {
	return Config{
		BaseURL:      base,
		Source:       inbox,
		SessionID:    session,
		PollInterval: pollEvery,
		MaxWait:      shortWait,
		CacheDir:     cacheDir,
		Owner:        owner,
	}
}

func TestRunDeliversOnceAndMovesTheCursor(t *testing.T) {
	events, srv := newFakeEvents(t)
	events.add(inbox, addressed("0002", "rebase done"))
	events.add(inbox, addressed("0001", "tests green"))
	events.add("belt-ledger", addressed("0003", "wrong source"))
	dir := t.TempDir()

	got := Run(context.Background(), testConfig(srv.URL, dir, "100"))

	var ids []string
	for _, it := range got {
		ids = append(ids, it.ID)
	}
	if strings.Join(ids, ",") != "0001,0002" {
		t.Fatalf("delivered %v, want 0001,0002 oldest first", ids)
	}
	if got[0].From != "orchestrator" || got[0].Message != "details of tests green" {
		t.Errorf("first item = %+v", got[0])
	}
	cursor, err := os.ReadFile(filepath.Join(dir, "watch-"+session))
	if err != nil || strings.TrimSpace(string(cursor)) != "0002" {
		t.Errorf("cursor = %q (%v), want 0002", cursor, err)
	}

	if again := Run(context.Background(), testConfig(srv.URL, dir, "101")); again != nil {
		t.Errorf("second run redelivered %+v, want nothing", again)
	}
}

// TestRunNeverRedeliversWhenTheServiceIgnoresSince pins the client-side
// cursor check: even a service that hands delivered entries back must not
// make the next watcher wake the session with them again, or every Stop
// would wake it with the same item.
func TestRunNeverRedeliversWhenTheServiceIgnoresSince(t *testing.T) {
	events, srv := newFakeEvents(t)
	events.ignoreSince = true
	events.add(inbox, addressed("0001", "rebase done"))
	dir := t.TempDir()

	if first := Run(context.Background(), testConfig(srv.URL, dir, "100")); len(first) != 1 {
		t.Fatalf("first run delivered %+v, want the one item", first)
	}
	if again := Run(context.Background(), testConfig(srv.URL, dir, "101")); again != nil {
		t.Errorf("second run redelivered %+v, want nothing", again)
	}
	events.add(inbox, addressed("0002", "tests green"))
	next := Run(context.Background(), testConfig(srv.URL, dir, "102"))
	if len(next) != 1 || next[0].ID != "0002" {
		t.Errorf("third run delivered %+v, want only the new item", next)
	}
}

// TestRunWakesOnAnItemThatArrivesWhileWaiting covers the idle case the
// watcher exists for: nothing pending at start, an item lands mid-wait.
func TestRunWakesOnAnItemThatArrivesWhileWaiting(t *testing.T) {
	events, srv := newFakeEvents(t)
	time.AfterFunc(3*pollEvery, func() { events.add(inbox, addressed("0001", "your turn")) })
	cfg := testConfig(srv.URL, t.TempDir(), "100")
	cfg.MaxWait = 5 * time.Second

	got := Run(context.Background(), cfg)
	if len(got) != 1 || got[0].Title != "your turn" {
		t.Errorf("delivered %+v, want the item that arrived mid-wait", got)
	}
}

func TestRunReturnsNothing(t *testing.T) {
	unreachable := func() string {
		srv := httptest.NewServer(http.NotFoundHandler())
		srv.Close()
		return srv.URL
	}
	tests := []struct {
		name  string
		setup func(t *testing.T, events *fakeEvents) (base string, cfg func(*Config))
	}{
		{
			name: "an entry that only mentions the session id is not addressed to it",
			setup: func(t *testing.T, events *fakeEvents) (string, func(*Config)) {
				events.add(inbox, entry{
					ID: "0001", Title: "about " + session,
					Tags: map[string]string{TagTo: "s-2"},
				})
				return "", nil
			},
		},
		{
			name: "nothing arrives before the max wait",
			setup: func(t *testing.T, events *fakeEvents) (string, func(*Config)) {
				return "", nil
			},
		},
		{
			name: "events service down",
			setup: func(t *testing.T, events *fakeEvents) (string, func(*Config)) {
				return unreachable(), nil
			},
		},
		{
			name: "no session id",
			setup: func(t *testing.T, events *fakeEvents) (string, func(*Config)) {
				events.add(inbox, addressed("0001", "for nobody"))
				return "", func(c *Config) { c.SessionID = "" }
			},
		},
		{
			name: "a cursor that cannot be written delivers nothing",
			setup: func(t *testing.T, events *fakeEvents) (string, func(*Config)) {
				events.add(inbox, addressed("0001", "would loop"))
				return "", func(c *Config) {
					// The cursor path is taken by a directory, so the
					// rename that writes it fails while the owner file
					// still works.
					if err := os.Mkdir(filepath.Join(c.CacheDir, "watch-"+session), 0o755); err != nil {
						t.Fatal(err)
					}
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, srv := newFakeEvents(t)
			base, tweak := tt.setup(t, events)
			if base == "" {
				base = srv.URL
			}
			cfg := testConfig(base, t.TempDir(), "100")
			if tweak != nil {
				tweak(&cfg)
			}
			start := time.Now()
			if got := Run(context.Background(), cfg); got != nil {
				t.Errorf("Run() = %+v, want nothing", got)
			}
			if elapsed := time.Since(start); elapsed > shortWait+time.Second {
				t.Errorf("Run() took %s, want it bounded by the %s max wait", elapsed, shortWait)
			}
		})
	}
}

// TestNewerWatcherSupersedesOlder: a session that goes idle again starts a
// second watcher while the first still waits; the first must step aside as
// soon as it sees the owner file name the newer one.
func TestNewerWatcherSupersedesOlder(t *testing.T) {
	_, srv := newFakeEvents(t)
	dir := t.TempDir()
	older := testConfig(srv.URL, dir, "100")
	older.MaxWait = 10 * time.Second

	done := make(chan []Item, 1)
	go func() { done <- Run(context.Background(), older) }()
	time.Sleep(3 * pollEvery)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Run(ctx, testConfig(srv.URL, dir, "200"))

	select {
	case got := <-done:
		if got != nil {
			t.Errorf("older watcher returned %+v, want nothing", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("older watcher kept running after a newer one claimed the session")
	}
}

// TestSupersededDuringThePollDeliversNothing: a newer watcher that claims
// the session while the older one's poll is in flight will find the same
// items, so the older one must not deliver them or move the cursor.
func TestSupersededDuringThePollDeliversNothing(t *testing.T) {
	events, srv := newFakeEvents(t)
	events.add(inbox, addressed("0001", "rebase done"))
	dir := t.TempDir()
	owner := filepath.Join(dir, "watch-"+session+".pid")
	events.onQuery = func() {
		if err := os.WriteFile(owner, []byte("200\n"), 0o644); err != nil {
			t.Error(err)
		}
	}

	if got := Run(context.Background(), testConfig(srv.URL, dir, "100")); got != nil {
		t.Errorf("superseded watcher delivered %+v, want nothing", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "watch-"+session)); !os.IsNotExist(err) {
		t.Errorf("superseded watcher wrote the cursor (stat err %v), want it untouched", err)
	}
}

func TestRender(t *testing.T) {
	got := Render([]Item{
		{ID: "1", From: "orchestrator", Title: "rebase done", Message: "branch feat/x is ready"},
		{ID: "2", Title: "ping"},
	})
	want := "belt[watch]: from orchestrator: rebase done\nbranch feat/x is ready\n\n" +
		"belt[watch]: from unknown sender: ping"
	if got != want {
		t.Errorf("Render() =\n%s\nwant\n%s", got, want)
	}
}
