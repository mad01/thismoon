package baseurl

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFromHeader(t *testing.T) {
	cases := []struct {
		name     string
		header   http.Header
		override string
		want     string
	}{
		{"override wins", http.Header{"X-Forwarded-Host": {"x"}}, "https://present.example.com/", "https://present.example.com"},
		{"nil header", nil, "", ""},
		{"no forwarded host", http.Header{"X-Forwarded-Proto": {"https"}}, "", ""},
		{"host only defaults to http", http.Header{"X-Forwarded-Host": {"present.internal"}}, "", "http://present.internal"},
		{"proto and host", http.Header{"X-Forwarded-Host": {"present.internal"}, "X-Forwarded-Proto": {"https"}}, "", "https://present.internal"},
		{"first of a proxy chain", http.Header{"X-Forwarded-Host": {"edge.example.com, inner"}, "X-Forwarded-Proto": {"https, http"}}, "", "https://edge.example.com"},
		{"host with port", http.Header{"X-Forwarded-Host": {"127.0.0.1:17423"}}, "", "http://127.0.0.1:17423"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FromHeader(tc.header, tc.override); got != tc.want {
				t.Fatalf("FromHeader = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMiddlewareFillsFromRequest(t *testing.T) {
	var got string
	h := Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = FromHeader(r.Header, "")
	}))

	r := httptest.NewRequest(http.MethodGet, "http://present.local:7423/p/x", nil)
	h.ServeHTTP(httptest.NewRecorder(), r)
	if got != "http://present.local:7423" {
		t.Fatalf("plain request: %q", got)
	}

	r = httptest.NewRequest(http.MethodGet, "https://present.local/p/x", nil)
	r.TLS = &tls.ConnectionState{}
	h.ServeHTTP(httptest.NewRecorder(), r)
	if got != "https://present.local" {
		t.Fatalf("tls request: %q", got)
	}

	r = httptest.NewRequest(http.MethodGet, "http://pod-ip:7423/p/x", nil)
	r.Header.Set("X-Forwarded-Host", "present.example.com")
	r.Header.Set("X-Forwarded-Proto", "https")
	h.ServeHTTP(httptest.NewRecorder(), r)
	if got != "https://present.example.com" {
		t.Fatalf("forwarded request must keep the proxy's headers: %q", got)
	}
}
