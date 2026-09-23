package server

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/mad01/thismoon/services/present/internal/store"
)

// PageWatcher tells the server when a page may have changed, so it can push
// the page's version to open tabs instead of waiting for their next poll.
// The cluster store is one. The filesystem store is not: on one machine the
// MCP process writes the pages, and serve never hears of it.
type PageWatcher interface {
	// WatchPage returns a channel that receives after page id changes,
	// coalescing bursts, and a stop func that releases it.
	WatchPage(id string) (<-chan struct{}, func())
}

const (
	// DefaultHeartbeat is how often an idle stream resends the version. It
	// stays under the 60 second idle timeout common in proxies, and it lets
	// the browser tell a live stream from one a buffering proxy holds back.
	DefaultHeartbeat = 25 * time.Second
	// reconnectDelay is the retry hint each stream opens with: how long the
	// browser waits before reconnecting after the stream drops, which it
	// does on every rollout.
	reconnectDelay = 2 * time.Second
)

// handleEvents streams a page's version as server-sent events: once on
// connect, again whenever the watcher reports a change or the heartbeat
// fires, and a final gone event once the page is deleted or expired. It
// subscribes before its first read, so a change between the two is not
// lost. The stream ends when the client leaves, the page goes, a read
// fails, or CloseStreams runs at shutdown; the browser reconnects on its
// own in every case but gone.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	changed, stop := s.watcher.WatchPage(id)
	defer stop()
	p, err := s.store.GetMeta(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	// nginx buffers proxied responses unless told otherwise, which would
	// hold every event back until its buffer filled.
	h.Set("X-Accel-Buffering", "no")
	rc := http.NewResponseController(w)
	if _, err := fmt.Fprintf(w, "retry: %d\n\n", reconnectDelay.Milliseconds()); err != nil {
		return
	}
	if !writeEvent(w, rc, "version", strconv.Itoa(p.Version)) {
		return
	}

	tick := time.NewTicker(s.heartbeat)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.streamsDone:
			return
		case <-changed:
		case <-tick.C:
		}
		p, err := s.store.GetMeta(r.Context(), id)
		if errors.Is(err, store.ErrNotFound) {
			writeEvent(w, rc, "gone", "")
			return
		}
		if err != nil {
			log.Printf("present: event stream for %s: %v", id, err)
			return
		}
		if !writeEvent(w, rc, "version", strconv.Itoa(p.Version)) {
			return
		}
	}
}

// writeEvent writes one named event and flushes it to the client. It
// reports false once the client is gone or the writer cannot flush.
func writeEvent(w http.ResponseWriter, rc *http.ResponseController, name, data string) bool {
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data); err != nil {
		return false
	}
	return rc.Flush() == nil
}

// CloseStreams ends every open event stream and any opened after it. Serve
// calls it when shutdown starts: a stream never finishes on its own, so
// without this a graceful shutdown would wait out its whole drain timeout.
// It is safe to call more than once.
func (s *Server) CloseStreams() {
	s.closeStreams.Do(func() { close(s.streamsDone) })
}
