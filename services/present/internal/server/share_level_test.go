package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	present "github.com/mad01/thismoon/services/present"
	"github.com/mad01/thismoon/services/present/internal/author"
	"github.com/mad01/thismoon/services/present/internal/sharedclient"
)

// A shared write answers with the instance's render level. A bundle from a
// client older than render levels is taken as before, and one from a newer
// client is taken too: the instance logs it and refuses nothing.
func TestSharedWriteAnswersItsRenderLevel(t *testing.T) {
	f := setupShared(t)
	logs := captureLog(t)
	key := author.NewKey()

	old := f.create(t, key, page("Old client"))
	if old.RenderLevel != present.RenderLevel {
		t.Errorf("create render_level = %d, want %d", old.RenderLevel, present.RenderLevel)
	}
	if strings.Contains(logs.String(), "render level") {
		t.Errorf("a bundle without a level was logged: %q", logs.String())
	}

	newer := page("Newer client")
	newer["render_level"] = present.RenderLevel + 1
	resp, body := f.call(t, http.MethodPut, "/api/p/"+old.ID, key, newer, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("replace from a newer client = %d (%s), want 200", resp.StatusCode, body)
	}
	var out sharedPage
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode replace: %v", err)
	}
	if out.RenderLevel != present.RenderLevel || out.Version != 2 {
		t.Errorf(
			"replace answer = %+v, want render_level %d at version 2",
			out,
			present.RenderLevel,
		)
	}
	if got := apiPageOf(t, f.ts.URL+"/api/p/"+old.ID); got.Title != "Newer client" {
		t.Errorf("stored title = %q, want the newer client's page", got.Title)
	}
	if !strings.Contains(logs.String(), old.ID) ||
		!strings.Contains(logs.String(), "render level") {
		t.Errorf("a page above this instance's level was not logged: %q", logs.String())
	}
}

// predatingInstance answers a create the way a shared instance from before
// render levels does: no render_level key.
func predatingInstance(t *testing.T) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/pages" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		id := strings.Repeat("c", 32)
		_, _ = w.Write(
			[]byte(`{"id":"` + id + `","url":"http://old.example/p/` + id + `","version":1}`),
		)
	}))
	t.Cleanup(ts.Close)
	return ts
}

// The Share button's reply carries the warning a share to an older instance
// raises, and a share to a current one carries none.
func TestShareReplyCarriesTheLevelWarning(t *testing.T) {
	current := setupLocalSharing(t, author.NewKey())
	p := createLocal(t, current.st, "Current")
	if got := shareOK(t, current.ts, p.ID, ""); got.Warning != "" {
		t.Errorf("share to a current instance warned: %q", got.Warning)
	}

	ts := newLocalServer(
		t,
		current.st,
		current.dir,
		sharedclient.New(predatingInstance(t).URL, "k"),
	)
	q := createLocal(t, current.st, "Old")
	got := shareOK(t, ts, q.ID, "")
	if got.URL == "" || !strings.Contains(got.Warning, "predates render levels") {
		t.Errorf(
			"share to an instance from before render levels = %+v, want the link and a warning",
			got,
		)
	}
	if again := apiPageOf(t, ts.URL+"/api/p/"+q.ID); again.Share.Warning != "" {
		t.Errorf("the stored share block carries a warning: %+v", again.Share)
	}
}
