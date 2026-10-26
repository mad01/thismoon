package hint

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

// moondbProbeTimeout bounds the doctor probe. Doctor is human-invoked, so it
// can wait longer than the session-start budget.
const moondbProbeTimeout = 2 * time.Second

// ErrMoondbNoInflight is what MoondbProbe returns when moondb serve answers
// but has no in-flight route: a moondb older than this belt.
var ErrMoondbNoInflight = errors.New("hint: moondb serve has no /api/inflight route")

// MoondbBaseURL reports where the moondb-inflight hint looks for moondb
// serve, so doctor can probe the same instance.
func MoondbBaseURL() string { return moondbBaseURL() }

// moondbProbeQuery is the smallest valid in-flight question: the root
// directory, which no session edits under as a checkout, and one file.
const moondbProbeQuery = "/api/inflight?worktree=%2F&limit=1"

// MoondbProbe asks moondb serve a valid in-flight question and reads the
// status: 200 means the route answers, 404 means a moondb older than the
// route, and anything else is an error. The hint renders all three, and
// moondb being down, as silence; doctor tells them apart.
func MoondbProbe(base string) error {
	client := &http.Client{Timeout: moondbProbeTimeout}
	resp, err := client.Get(base + moondbProbeQuery)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusNotFound:
		return ErrMoondbNoInflight
	default:
		return fmt.Errorf("hint: moondb probe status %s", resp.Status)
	}
}
