package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/services/reminder/internal/store"
)

// testInfo is the build metadata the test server reports on /version.
var testInfo = buildinfo.Info{
	Version:   "test",
	Commit:    "0123456789abcdef0123456789abcdef01234567",
	Tag:       "reminder/v0.0.0",
	BuildTime: "2026-08-13T09:00:00Z",
}

// fakeNotifier records Notify calls instead of firing a real macOS
// notification, so tests can assert delivery without side effects.
type fakeNotifier struct {
	calls []struct{ title, body string }
}

func (f *fakeNotifier) Notify(title, body string) error {
	f.calls = append(f.calls, struct{ title, body string }{title, body})
	return nil
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	ts, _ := newTestServerN(t)
	return ts
}

// newTestServerN also returns the fake notifier so test/fire tests can assert a
// notification was delivered.
func newTestServerN(t *testing.T) (*httptest.Server, *fakeNotifier) {
	t.Helper()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	fn := &fakeNotifier{}
	return httptest.NewServer(New(st, testInfo, fn).Handler()), fn
}

func TestCreateWithRelativeIn(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	res, err := http.Post(ts.URL+"/api/reminders", "application/json",
		strings.NewReader(`{"title":"ping","in":"1h"}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", res.StatusCode)
	}
	var got apiReminder
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Title != "ping" || got.ID == "" {
		t.Fatalf("unexpected reminder: %+v", got)
	}
	if got.Due.IsZero() {
		t.Error("due should be resolved from 'in'")
	}
}

func TestCreateRequiresDueOrIn(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	res, err := http.Post(ts.URL+"/api/reminders", "application/json",
		strings.NewReader(`{"title":"x"}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("missing due/in: status = %d, want 400", res.StatusCode)
	}
}

func TestListAndCancelFlow(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	res, err := http.Post(ts.URL+"/api/reminders", "application/json",
		strings.NewReader(`{"title":"x","in":"1h"}`))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	var created apiReminder
	_ = json.NewDecoder(res.Body).Decode(&created)
	_ = res.Body.Close()

	// cancel
	cres, err := http.Post(ts.URL+"/api/reminders/"+created.ID+"/cancel", "application/json", nil)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	_ = cres.Body.Close()
	if cres.StatusCode != http.StatusOK {
		t.Fatalf("cancel status = %d", cres.StatusCode)
	}

	// list filtered by cancelled
	lres, err := http.Get(ts.URL + "/api/reminders?status=cancelled")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	defer func() { _ = lres.Body.Close() }()
	var list struct {
		Reminders []apiReminder `json:"reminders"`
	}
	_ = json.NewDecoder(lres.Body).Decode(&list)
	if len(list.Reminders) != 1 || list.Reminders[0].Status != store.StatusCancelled {
		t.Errorf("cancelled list wrong: %+v", list.Reminders)
	}
}

func TestPageServesShell(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	// Seed a reminder so we can prove the page does not inline it: the body is
	// rendered client-side by app.js from GET /api/reminders.
	cres, err := http.Post(ts.URL+"/api/reminders", "application/json",
		strings.NewReader(`{"title":"secret-reminder","in":"1h"}`))
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	_ = cres.Body.Close()

	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	page := string(body)
	for _, want := range []string{`id="app"`, "/app.js", "/webkit/webkit.js"} {
		if !strings.Contains(page, want) {
			t.Errorf("shell missing %q", want)
		}
	}
	if strings.Contains(page, "secret-reminder") {
		t.Error("shell should not inline reminder data")
	}
}

func TestAppJS(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	res, err := http.Get(ts.URL + "/app.js")
	if err != nil {
		t.Fatalf("GET /app.js: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/javascript") {
		t.Errorf("Content-Type = %q, want application/javascript", ct)
	}
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "/api/reminders") {
		t.Error("app.js should fetch /api/reminders")
	}
}

// createReminder posts a reminder and returns it, failing the test on error.
func createReminder(t *testing.T, ts *httptest.Server, body string) apiReminder {
	t.Helper()
	res, err := http.Post(ts.URL+"/api/reminders", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", res.StatusCode)
	}
	var r apiReminder
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return r
}

func TestTestEndpointDeliversWithoutMutating(t *testing.T) {
	ts, fn := newTestServerN(t)
	defer ts.Close()

	created := createReminder(t, ts, `{"title":"ping","body":"note","in":"1h"}`)

	res, err := http.Post(ts.URL+"/api/reminders/"+created.ID+"/test", "application/json", nil)
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("test status = %d, want 200", res.StatusCode)
	}
	if len(fn.calls) != 1 || fn.calls[0].title != "ping" || fn.calls[0].body != "note" {
		t.Fatalf("notifier calls = %+v, want one {ping, note}", fn.calls)
	}

	// State must be untouched: still pending, never fired.
	got, err := http.Get(ts.URL + "/api/reminders/" + created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = got.Body.Close() }()
	var after apiReminder
	_ = json.NewDecoder(got.Body).Decode(&after)
	if after.Status != store.StatusPending {
		t.Errorf("status after test = %q, want pending", after.Status)
	}
	if after.FiredAt != nil {
		t.Errorf("test should not set FiredAt, got %v", after.FiredAt)
	}
}

func TestFireEndpointFiresOneShot(t *testing.T) {
	ts, fn := newTestServerN(t)
	defer ts.Close()

	created := createReminder(t, ts, `{"title":"x","in":"1h"}`)

	res, err := http.Post(ts.URL+"/api/reminders/"+created.ID+"/fire", "application/json", nil)
	if err != nil {
		t.Fatalf("fire: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("fire status = %d, want 200", res.StatusCode)
	}
	var fired apiReminder
	_ = json.NewDecoder(res.Body).Decode(&fired)
	if len(fn.calls) != 1 {
		t.Fatalf("notifier calls = %d, want 1", len(fn.calls))
	}
	if fired.Status != store.StatusFired {
		t.Errorf("status after fire = %q, want fired", fired.Status)
	}
}

func TestFireRecurringReschedules(t *testing.T) {
	ts, _ := newTestServerN(t)
	defer ts.Close()

	// A past-due recurring reminder: firing it reschedules to the next future
	// occurrence and stays pending (the same path the ticker takes when due).
	created := createReminder(t, ts, `{"title":"daily","in":"-1h","repeat":"daily"}`)

	res, err := http.Post(ts.URL+"/api/reminders/"+created.ID+"/fire", "application/json", nil)
	if err != nil {
		t.Fatalf("fire: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	var fired apiReminder
	_ = json.NewDecoder(res.Body).Decode(&fired)
	if fired.Status != store.StatusPending {
		t.Errorf("recurring status after fire = %q, want pending", fired.Status)
	}
	if !fired.Due.After(created.Due) {
		t.Errorf("recurring due should advance: was %v, now %v", created.Due, fired.Due)
	}
}

func TestGlobalTestEndpoint(t *testing.T) {
	ts, fn := newTestServerN(t)
	defer ts.Close()

	res, err := http.Post(ts.URL+"/api/test", "application/json", nil)
	if err != nil {
		t.Fatalf("global test: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if len(fn.calls) != 1 {
		t.Errorf("notifier calls = %d, want 1", len(fn.calls))
	}
}

func TestTestEndpointUnknownReminder(t *testing.T) {
	ts, _ := newTestServerN(t)
	defer ts.Close()
	res, err := http.Post(ts.URL+"/api/reminders/nope/test", "application/json", nil)
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("unknown id test status = %d, want 404", res.StatusCode)
	}
}

// TestListJSONShape pins the field names app.js renders from — the JSON shape
// is the contract between the API and the client renderer.
func TestListJSONShape(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	cres, err := http.Post(ts.URL+"/api/reminders", "application/json",
		strings.NewReader(`{"title":"ping","body":"note","in":"1h","repeat":"daily"}`))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_ = cres.Body.Close()

	res, err := http.Get(ts.URL + "/api/reminders")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	body, _ := io.ReadAll(res.Body)
	for _, want := range []string{
		`"reminders"`, `"id"`, `"title"`, `"body"`, `"due"`,
		`"repeat"`, `"status"`, `"overdue"`,
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("api/reminders JSON missing field %s", want)
		}
	}
}
