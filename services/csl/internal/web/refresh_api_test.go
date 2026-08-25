package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mad01/thismoon/services/csl/internal/refresh"
)

// fakeRefresher implements the refresher seam for handler tests.
type fakeRefresher struct {
	status  refresh.Status
	kickErr error
	kicked  []string
}

func (f *fakeRefresher) Status() refresh.Status { return f.status }

func (f *fakeRefresher) Kick(only string) error {
	f.kicked = append(f.kicked, only)
	return f.kickErr
}

func TestRefreshStatusEndpoint(t *testing.T) {
	fake := &fakeRefresher{status: refresh.Status{
		Enabled:         true,
		IntervalMinutes: 15,
		Repos:           []refresh.RepoStatus{{Name: "org/a", Path: "/repos/a", Status: "ok"}},
	}}
	h := (&Server{ref: fake}).Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/refresh_status", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var got refresh.Status
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.Enabled || got.IntervalMinutes != 15 || len(got.Repos) != 1 {
		t.Errorf("payload = %+v, want the fake's status", got)
	}
}

func TestRefreshKickEndpoint(t *testing.T) {
	fake := &fakeRefresher{}
	h := (&Server{ref: fake}).Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/refresh?repo=org/a", nil))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body: %s", rec.Code, rec.Body.String())
	}
	if len(fake.kicked) != 1 || fake.kicked[0] != "org/a" {
		t.Errorf("kicked = %v, want [org/a]", fake.kicked)
	}
}

func TestRefreshKickBusy(t *testing.T) {
	fake := &fakeRefresher{kickErr: refresh.ErrBusy}
	h := (&Server{ref: fake}).Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/refresh", nil))

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body: %s", rec.Code, rec.Body.String())
	}
}

func TestRefreshEndpointsWithoutRefresher(t *testing.T) {
	h := (&Server{}).Handler()

	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/refresh_status", nil),
		httptest.NewRequest(http.MethodPost, "/api/refresh", nil),
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s %s = %d, want 503", req.Method, req.URL.Path, rec.Code)
		}
	}
}
