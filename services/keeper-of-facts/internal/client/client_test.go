package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// stubServer runs a hand-rolled handler standing in for `kof serve`, so these
// tests exercise the client without importing the real server package.
func stubServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func TestAssertSendsBodyAndDecodes(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody AssertBody
	srv := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(Assertion{ID: "a1", Kind: gotBody.Kind, Status: "fresh"})
	})

	got, err := New(srv.URL).Assert(AssertBody{
		Kind:       "code-behavior",
		Subject:    "repo:mad01/thismoon/services/keeper-of-facts",
		Statement:  "serve is the single writer",
		Confidence: "verified",
		SessionID:  "sess-1",
		Pins: []PinRef{
			{RepoPath: "mad01/thismoon", File: "kof.go", StartLine: 1, EndLine: 3},
		},
	})
	if err != nil {
		t.Fatalf("Assert: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/assertions" {
		t.Errorf("request = %s %s, want POST /api/assertions", gotMethod, gotPath)
	}
	if len(gotBody.Pins) != 1 || gotBody.Pins[0].File != "kof.go" {
		t.Errorf("pins not round-tripped: %+v", gotBody.Pins)
	}
	if got.ID != "a1" || got.Status != "fresh" {
		t.Errorf("decoded assertion = %+v", got)
	}
}

func TestListEncodesQueryParams(t *testing.T) {
	var gotQuery url.Values
	srv := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		_ = json.NewEncoder(w).Encode(struct {
			Assertions []Assertion `json:"assertions"`
		}{Assertions: []Assertion{{ID: "a1"}, {ID: "a2"}}})
	})

	got, err := New(srv.URL).List("repo:mad01/thismoon", "decision", "fresh")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if gotQuery.Get("subject") != "repo:mad01/thismoon" ||
		gotQuery.Get("kind") != "decision" ||
		gotQuery.Get("status") != "fresh" {
		t.Errorf("query = %v", gotQuery)
	}
	if len(got) != 2 {
		t.Errorf("len = %d, want 2", len(got))
	}
}

func TestListOmitsEmptyParams(t *testing.T) {
	var gotRawQuery string
	srv := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotRawQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"assertions":[]}`))
	})

	if _, err := New(srv.URL).List("", "", ""); err != nil {
		t.Fatalf("List: %v", err)
	}
	if gotRawQuery != "" {
		t.Errorf("raw query = %q, want empty", gotRawQuery)
	}
}

func TestGetDecodesAssertion(t *testing.T) {
	srv := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/assertions/a1" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(Assertion{ID: "a1", Statement: "x"})
	})

	got, err := New(srv.URL).Get("a1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != "a1" || got.Statement != "x" {
		t.Errorf("got = %+v", got)
	}
}

func TestRetractSendsNote(t *testing.T) {
	var gotPath string
	var gotBody retractBody
	srv := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).
			Encode(Assertion{ID: "a1", Status: "retracted", RetractNote: gotBody.Note})
	})

	got, err := New(srv.URL).Retract("a1", "superseded by ADR 0009")
	if err != nil {
		t.Fatalf("Retract: %v", err)
	}
	if gotPath != "/api/assertions/a1/retract" {
		t.Errorf("path = %q", gotPath)
	}
	if gotBody.Note != "superseded by ADR 0009" {
		t.Errorf("note = %q", gotBody.Note)
	}
	if got.Status != "retracted" {
		t.Errorf("status = %q", got.Status)
	}
}

func TestCheckOneSendsID(t *testing.T) {
	var gotBody map[string]any
	srv := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(CheckReport{Checked: 1, Fresh: 1})
	})

	got, err := New(srv.URL).Check("a1")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if gotBody["id"] != "a1" {
		t.Errorf("body id = %v, want a1", gotBody["id"])
	}
	if got.Checked != 1 || got.Fresh != 1 {
		t.Errorf("report = %+v", got)
	}
}

// TestCheckAllOmitsID confirms an empty id marshals to `{}`, which serve reads
// as "check every non-retracted assertion".
func TestCheckAllOmitsID(t *testing.T) {
	var gotBody map[string]any
	srv := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(CheckReport{Checked: 3, Fresh: 2, Stale: 1, Flipped: 1})
	})

	got, err := New(srv.URL).Check("")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if _, present := gotBody["id"]; present {
		t.Errorf("body = %v, want no id key", gotBody)
	}
	if got.Checked != 3 || got.Flipped != 1 {
		t.Errorf("report = %+v", got)
	}
}

// TestDoSurfacesAPIError verifies an API error body is surfaced verbatim — the
// serve side already package-prefixes its messages, so the client must not add a
// second "kof:" prefix.
func TestDoSurfacesAPIError(t *testing.T) {
	tests := []struct {
		name string
		code int
		body string
		want string
	}{
		{
			name: "validation message passes through unchanged",
			code: http.StatusBadRequest,
			body: `{"error":"boom"}`,
			want: "boom",
		},
		{
			name: "prefixed sentinel passes through unchanged",
			code: http.StatusNotFound,
			body: `{"error":"kof: not found"}`,
			want: "kof: not found",
		},
		{
			name: "empty error body falls back to status",
			code: http.StatusInternalServerError,
			body: `{}`,
			want: "kof: serve returned 500",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := stubServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.code)
				_, _ = w.Write([]byte(tt.body))
			})

			_, err := New(srv.URL).Get("anything")
			if err == nil {
				t.Fatalf("got nil error, want %q", tt.want)
			}
			if got := err.Error(); got != tt.want {
				t.Errorf("error = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestDoReportsUnreachableServe checks that a transport failure yields the
// actionable "serve not reachable" guidance rather than a bare dial error.
func TestDoReportsUnreachableServe(t *testing.T) {
	// Port 0 on a closed address: New never dials until a call is made.
	_, err := New("http://127.0.0.1:0").List("", "", "")
	if err == nil {
		t.Fatal("got nil error, want unreachable error")
	}
	if !strings.Contains(err.Error(), "t-man status keeper-of-facts") {
		t.Errorf("error = %q, want it to mention 't-man status keeper-of-facts'", err.Error())
	}
}
