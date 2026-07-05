package client

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDoSurfacesAPIError verifies an API error body is surfaced verbatim — the
// serve side already package-prefixes its messages, so the client must not add a
// second "reminder:" prefix.
func TestDoSurfacesAPIError(t *testing.T) {
	tests := []struct {
		name string
		code int
		body string
		want string
	}{
		{
			name: "prefixed sentinel passes through unchanged",
			code: http.StatusNotFound,
			body: `{"error":"reminder: not found"}`,
			want: "reminder: not found",
		},
		{
			name: "validation message passes through unchanged",
			code: http.StatusBadRequest,
			body: `{"error":"title is required"}`,
			want: "title is required",
		},
		{
			name: "empty error body falls back to status",
			code: http.StatusInternalServerError,
			body: `{}`,
			want: "reminder: serve returned 500",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(tt.code)
					_, _ = w.Write([]byte(tt.body))
				}),
			)
			defer srv.Close()

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
	_, err := New("http://127.0.0.1:0").Get("x")
	if err == nil {
		t.Fatal("got nil error, want unreachable error")
	}
	if !strings.Contains(err.Error(), "reminder serve not reachable") {
		t.Errorf("error = %q, want it to mention 'reminder serve not reachable'", err.Error())
	}
}
