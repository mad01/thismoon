package server

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/present/internal/store"
)

// fakeWatcher hands out one channel per watch and lets a test fire it. It
// counts the watches still held, so a test can check that every stream
// releases its watch.
type fakeWatcher struct {
	mu      sync.Mutex
	chans   map[string][]chan struct{}
	holding int
}

func (w *fakeWatcher) WatchPage(id string) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.chans == nil {
		w.chans = map[string][]chan struct{}{}
	}
	w.chans[id] = append(w.chans[id], ch)
	w.holding++
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			w.mu.Lock()
			defer w.mu.Unlock()
			w.holding--
		})
	}
}

// fire wakes every stream watching id.
func (w *fakeWatcher) fire(id string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, ch := range w.chans[id] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (w *fakeWatcher) held() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.holding
}

type eventsFixture struct {
	ts      *httptest.Server
	srv     *Server
	st      store.Store
	watcher *fakeWatcher
}

func setupEvents(t *testing.T, heartbeat time.Duration) *eventsFixture {
	t.Helper()
	dir := t.TempDir()
	st, err := store.NewFS(dir)
	if err != nil {
		t.Fatalf("store.NewFS: %v", err)
	}
	f := &eventsFixture{st: st, watcher: &fakeWatcher{}}
	f.srv = New(st, Options{
		Workdir: dir, Info: testInfo, Watcher: f.watcher, Heartbeat: heartbeat,
	})
	f.ts = httptest.NewServer(f.srv.Handler())
	t.Cleanup(f.ts.Close)
	return f
}

// event is one server-sent event as the browser would dispatch it.
type event struct{ name, data string }

// openStream starts the event stream at url and returns its response and a
// channel of parsed events that closes when the stream ends.
func openStream(t *testing.T, url string) (*http.Response, <-chan event) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("GET %s: %v", url, err)
	}
	t.Cleanup(func() {
		cancel()
		_ = resp.Body.Close()
	})
	events := make(chan event, 16)
	go func() {
		defer close(events)
		readEvents(resp.Body, events)
	}()
	return resp, events
}

// readEvents parses the event-stream format into named events until the
// body ends. Retry hints and blank lines carry no event.
func readEvents(body io.Reader, out chan<- event) {
	sc := bufio.NewScanner(body)
	var e event
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if e.name != "" {
				out <- e
			}
			e = event{}
		case strings.HasPrefix(line, "event: "):
			e.name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			e.data = strings.TrimPrefix(line, "data: ")
		}
	}
}

// next waits for the stream's next event.
func next(t *testing.T, events <-chan event) event {
	t.Helper()
	select {
	case e, ok := <-events:
		if !ok {
			t.Fatal("stream ended, want another event")
		}
		return e
	case <-time.After(5 * time.Second):
		t.Fatal("no event within 5s")
	}
	return event{}
}

// ended waits for the stream to close.
func ended(t *testing.T, events <-chan event) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-events:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("stream still open after 5s")
		}
	}
}

func TestEventsStreamVersionThenChanges(t *testing.T) {
	f := setupEvents(t, time.Hour)
	p, err := f.st.Create(t.Context(), store.Draft{Title: "T", Content: "<p>1</p>"})
	if err != nil {
		t.Fatal(err)
	}
	resp, events := openStream(t, f.ts.URL+"/p/"+p.ID+"/events")
	for header, want := range map[string]string{
		"Content-Type": "text/event-stream", "Cache-Control": "no-cache", "X-Accel-Buffering": "no",
	} {
		if got := resp.Header.Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if e := next(t, events); e != (event{"version", "1"}) {
		t.Fatalf("first event = %+v, want version 1", e)
	}

	content := "<p>2</p>"
	if _, err := f.st.Update(t.Context(), p.ID, store.Patch{Content: &content}); err != nil {
		t.Fatal(err)
	}
	f.watcher.fire(p.ID)
	if e := next(t, events); e != (event{"version", "2"}) {
		t.Fatalf("event after update = %+v, want version 2", e)
	}
}

// TestEventsHeartbeatResendsVersion pins the heartbeat: with nothing
// changing, an idle stream still repeats the version, which keeps proxies
// from timing it out and tells the browser the stream is live.
func TestEventsHeartbeatResendsVersion(t *testing.T) {
	f := setupEvents(t, 20*time.Millisecond)
	p, err := f.st.Create(t.Context(), store.Draft{Title: "T", Content: "<p>1</p>"})
	if err != nil {
		t.Fatal(err)
	}
	_, events := openStream(t, f.ts.URL+"/p/"+p.ID+"/events")
	for i := range 3 {
		if e := next(t, events); e != (event{"version", "1"}) {
			t.Fatalf("event %d = %+v, want version 1", i, e)
		}
	}
}

func TestEventsEndWithGoneWhenPageIsDeleted(t *testing.T) {
	f := setupEvents(t, time.Hour)
	p, err := f.st.Create(t.Context(), store.Draft{Title: "T", Content: "<p>1</p>"})
	if err != nil {
		t.Fatal(err)
	}
	_, events := openStream(t, f.ts.URL+"/p/"+p.ID+"/events")
	next(t, events)
	if err := f.st.Delete(t.Context(), p.ID); err != nil {
		t.Fatal(err)
	}
	f.watcher.fire(p.ID)
	if e := next(t, events); e.name != "gone" {
		t.Fatalf("event after delete = %+v, want gone", e)
	}
	ended(t, events)
	waitHeld(t, f.watcher, 0)
}

func TestEventsUnknownPageIs404(t *testing.T) {
	f := setupEvents(t, time.Hour)
	if code, _ := get(t, f.ts.URL+"/p/"+store.NewSharedID()+"/events"); code != http.StatusNotFound {
		t.Fatalf("events for an unknown page = %d, want 404", code)
	}
	waitHeld(t, f.watcher, 0)
}

// TestEventsNeedAWatcher pins the fallback contract: without a change feed
// the route does not exist, and the browser's 404 sends it back to polling.
func TestEventsNeedAWatcher(t *testing.T) {
	ts, st := setup(t)
	p, err := st.Create(t.Context(), store.Draft{Title: "T", Content: "<p>1</p>"})
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := get(t, ts.URL+"/p/"+p.ID+"/events"); code != http.StatusNotFound {
		t.Fatalf("events without a watcher = %d, want 404", code)
	}
}

// TestCloseStreamsEndsOpenStreams covers shutdown: CloseStreams finishes
// every open stream, so the drain is not held up, and releases its watch.
func TestCloseStreamsEndsOpenStreams(t *testing.T) {
	f := setupEvents(t, time.Hour)
	p, err := f.st.Create(t.Context(), store.Draft{Title: "T", Content: "<p>1</p>"})
	if err != nil {
		t.Fatal(err)
	}
	_, events := openStream(t, f.ts.URL+"/p/"+p.ID+"/events")
	next(t, events)
	f.srv.CloseStreams()
	f.srv.CloseStreams() // a second call must not panic
	ended(t, events)
	waitHeld(t, f.watcher, 0)
}

// TestEventsReleaseTheWatchWhenTheClientLeaves guards against a leak: a
// browser closing its tab must free the stream's watch.
func TestEventsReleaseTheWatchWhenTheClientLeaves(t *testing.T) {
	f := setupEvents(t, time.Hour)
	p, err := f.st.Create(t.Context(), store.Draft{Title: "T", Content: "<p>1</p>"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.ts.URL+"/p/"+p.ID+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	waitHeld(t, f.watcher, 1)
	cancel()
	_, _ = io.Copy(io.Discard, resp.Body) // ends with the cancellation's error
	_ = resp.Body.Close()
	waitHeld(t, f.watcher, 0)
}

// waitHeld waits until the watcher holds exactly want watches.
func waitHeld(t *testing.T, w *fakeWatcher, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if w.held() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("watcher holds %d watches, want %d", w.held(), want)
}
