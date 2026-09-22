package server

import (
	"bytes"
	"log"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// logSink collects what the server logs while a request is served. The
// handler goroutine writes while the test reads, so the buffer is guarded.
type logSink struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *logSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *logSink) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// captureLog routes the standard logger into a sink for the test.
func captureLog(t *testing.T) *logSink {
	t.Helper()
	sink := &logSink{}
	out, flags := log.Writer(), log.Flags()
	log.SetOutput(sink)
	t.Cleanup(func() { log.SetOutput(out); log.SetFlags(flags) })
	return sink
}

// getAs fetches a URL under a chosen User-Agent.
func getAs(t *testing.T, url, agent string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("User-Agent", agent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

// TestAccessLogSkipsKubeletProbes pins the one exception to the access log.
// Readiness every few seconds would otherwise fill a pod's log with the same
// line, hiding the requests an operator reads the log for.
func TestAccessLogSkipsKubeletProbes(t *testing.T) {
	ts, _ := setup(t)
	sink := captureLog(t)

	if code := getAs(t, ts.URL+"/version", "kube-probe/1.31"); code != http.StatusOK {
		t.Fatalf("probe GET /version = %d, want 200", code)
	}
	if got := sink.String(); strings.Contains(got, "/version") {
		t.Errorf("probe request was logged: %q", got)
	}

	if code := getAs(t, ts.URL+"/version", "curl/8.7.1"); code != http.StatusOK {
		t.Fatalf("GET /version = %d, want 200", code)
	}
	if got := sink.String(); !strings.Contains(got, "GET /version -> 200") {
		t.Errorf("non-probe request was not logged: %q", got)
	}
}

// TestAccessLogSkipsProbesOnEveryPath guards against narrowing the filter to
// /version: kubelet is configured with whatever path the manifests name, so
// the prober is identified by who it is, not by what it asks for.
func TestAccessLogSkipsProbesOnEveryPath(t *testing.T) {
	ts, st := setup(t)
	p := createLocal(t, st, "Probed")
	sink := captureLog(t)

	if code := getAs(t, ts.URL+"/p/"+p.ID+"/version", "kube-probe/1.33"); code != http.StatusOK {
		t.Fatalf("probe GET page version = %d, want 200", code)
	}
	if got := sink.String(); got != "" {
		t.Errorf("probe request was logged: %q", got)
	}
}
