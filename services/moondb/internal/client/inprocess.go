package client

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
)

// inProcessURL is the base URL a client of an in-process handler names in
// its errors; no request leaves the process.
const inProcessURL = "http://moondb.in-process"

// NewForHandler returns a Client whose requests h serves in process, with
// the same timeouts as New. A shared instance's MCP tools use it to read
// the serve they run in without a network hop or a key of their own.
func NewForHandler(h http.Handler) *Client {
	c := New(inProcessURL)
	c.http.Transport = handlerTransport{h}
	c.slow.Transport = handlerTransport{h}
	return c
}

// handlerTransport answers each request by calling a handler directly.
type handlerTransport struct {
	h http.Handler
}

// RoundTrip serves req with the handler and returns what it wrote.
func (t handlerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := &recorder{header: http.Header{}}
	t.h.ServeHTTP(rec, req)
	if rec.status == 0 {
		rec.status = http.StatusOK
	}
	return &http.Response{
		Status:        strconv.Itoa(rec.status) + " " + http.StatusText(rec.status),
		StatusCode:    rec.status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        rec.header,
		Body:          io.NopCloser(bytes.NewReader(rec.body.Bytes())),
		ContentLength: int64(rec.body.Len()),
		Request:       req,
	}, nil
}

// recorder is the http.ResponseWriter handlerTransport hands the handler:
// it keeps the status, the header and the body in memory.
type recorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (r *recorder) Header() http.Header { return r.header }

func (r *recorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
}

func (r *recorder) Write(p []byte) (int, error) {
	r.WriteHeader(http.StatusOK)
	return r.body.Write(p)
}
