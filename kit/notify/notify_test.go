package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBaseURL(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want string
	}{
		{name: "unset falls back", want: DefaultBaseURL},
		{name: "env wins", env: "http://events.this", want: "http://events.this"},
		{name: "empty falls back", env: "", want: DefaultBaseURL},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("EVENTS_BASE_URL", tc.env)
			if got := BaseURL(); got != tc.want {
				t.Errorf("BaseURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPost(t *testing.T) {
	type received struct {
		path        string
		contentType string
		body        []byte
	}
	got := make(chan received, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got <- received{path: r.URL.Path, contentType: r.Header.Get("Content-Type"), body: body}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	ev := Event{
		Source:  "belt",
		Level:   "warn",
		Title:   "guard denied a command",
		Message: "git push origin main",
		Tags:    map[string]string{"guard": "git-push-main"},
	}
	if err := post(context.Background(), srv.URL, ev, syncTimeout); err != nil {
		t.Fatalf("post() error: %v", err)
	}

	r := <-got
	if r.path != "/api/events" {
		t.Errorf("POST path = %q, want %q", r.path, "/api/events")
	}
	if r.contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", r.contentType)
	}
	var decoded map[string]any
	if err := json.Unmarshal(r.body, &decoded); err != nil {
		t.Fatalf("decode body %q: %v", r.body, err)
	}
	want := map[string]any{
		"source":  "belt",
		"level":   "warn",
		"title":   "guard denied a command",
		"message": "git push origin main",
		"tags":    map[string]any{"guard": "git-push-main"},
	}
	for k, v := range want {
		if !equal(decoded[k], v) {
			t.Errorf("payload[%q] = %#v, want %#v", k, decoded[k], v)
		}
	}
	if len(decoded) != len(want) {
		t.Errorf("payload has keys %v, want exactly %v", keys(decoded), keys(want))
	}
}

func TestPostNilTagsSerialize(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
	}))
	defer srv.Close()

	if err := post(context.Background(), srv.URL, Event{Source: "csl"}, syncTimeout); err != nil {
		t.Fatalf("post() error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode body %q: %v", body, err)
	}
	if _, ok := decoded["tags"]; !ok {
		t.Errorf("payload %q drops the tags key, want it present", body)
	}
	if decoded["tags"] != nil {
		t.Errorf("payload tags = %#v, want null", decoded["tags"])
	}
}

func TestPostErrors(t *testing.T) {
	t.Run("service error status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()
		if err := post(context.Background(), srv.URL, Event{}, syncTimeout); err == nil {
			t.Error("post() to a failing service = nil error, want one")
		}
	})

	t.Run("service down", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		url := srv.URL
		srv.Close()
		if err := post(context.Background(), url, Event{}, syncTimeout); err == nil {
			t.Error("post() to a closed service = nil error, want one")
		}
	})

	t.Run("timeout", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			time.Sleep(100 * time.Millisecond)
		}))
		defer srv.Close()
		if err := post(context.Background(), srv.URL, Event{}, time.Millisecond); err == nil {
			t.Error("post() past its budget = nil error, want one")
		}
	})
}

// TestEmitFromTestsIsSilent pins the contract that keeps `go test` off the
// live timeline: the exported emitters no-op while testing.Testing() is
// true, which is why the tests above exercise post directly.
func TestEmitFromTestsIsSilent(t *testing.T) {
	hits := make(chan struct{}, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hits <- struct{}{}
	}))
	defer srv.Close()
	t.Setenv("EVENTS_BASE_URL", srv.URL)

	EmitEvent("kit", "info", "t", "m", nil)
	EmitEventSync("kit", "info", "t", "m", nil)

	select {
	case <-hits:
		t.Error("an emitter posted from a test, want silence")
	case <-time.After(50 * time.Millisecond):
	}
}

func equal(got, want any) bool {
	gotJSON, err := json.Marshal(got)
	if err != nil {
		return false
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		return false
	}
	return string(gotJSON) == string(wantJSON)
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
