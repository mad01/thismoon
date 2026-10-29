package cloud

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeGCS answers the four JSON API calls a Bucket makes, for one bucket,
// from memory: a media upload (with ifGenerationMatch=0 honoured), a media
// download, an object's metadata and the bucket's metadata. failNext
// answers the next requests with its statuses instead.
type fakeGCS struct {
	bucket string

	mu       sync.Mutex
	objects  map[string][]byte
	failNext []int
	calls    []string
}

func newFakeGCS(bucket string) *fakeGCS {
	return &fakeGCS{bucket: bucket, objects: map[string][]byte{}}
}

// serve starts f on an httptest server and returns a Bucket that talks to
// it without backing off between tries.
func (f *fakeGCS) serve(t *testing.T) *Bucket {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	b := newBucket(f.bucket, srv.URL, srv.Client())
	b.wait = func(int) time.Duration { return 0 }
	return b
}

func (f *fakeGCS) object(name string) ([]byte, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.objects[name]
	return data, ok
}

func (f *fakeGCS) put(name string, data []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[name] = data
}

func (f *fakeGCS) failWith(statuses ...int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failNext = append(f.failNext, statuses...)
}

// callsTo counts the requests whose method and object start with prefix,
// such as "POST log/" or "GET objects/".
func (f *fakeGCS) callsTo(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func (f *fakeGCS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	if len(f.failNext) > 0 {
		code := f.failNext[0]
		f.failNext = f.failNext[1:]
		f.mu.Unlock()
		writeAPIError(w, code, "injected failure")
		return
	}
	f.mu.Unlock()
	path := r.URL.EscapedPath()
	upload := "/upload/storage/v1/b/" + f.bucket + "/o"
	objects := "/storage/v1/b/" + f.bucket + "/o/"
	switch {
	case r.Method == http.MethodPost && path == upload:
		f.handleUpload(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(path, objects):
		name, err := url.PathUnescape(strings.TrimPrefix(path, objects))
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}
		f.handleGet(w, r, name)
	case r.Method == http.MethodGet && path == "/storage/v1/b/"+f.bucket:
		writeAPIJSON(w, map[string]string{"name": f.bucket})
	default:
		writeAPIError(w, http.StatusNotFound, "no route "+r.Method+" "+path)
	}
}

func (f *fakeGCS) handleUpload(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	name := q.Get("name")
	if q.Get("uploadType") != "media" || name == "" {
		writeAPIError(w, http.StatusBadRequest, "want a media upload with a name")
		return
	}
	data, err := io.ReadAll(r.Body)
	if err != nil || int64(len(data)) != r.ContentLength {
		writeAPIError(w, http.StatusBadRequest, "body does not match its length")
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "POST "+name)
	if _, ok := f.objects[name]; ok && q.Get("ifGenerationMatch") == "0" {
		writeAPIError(w, http.StatusPreconditionFailed, "conditionNotMet")
		return
	}
	f.objects[name] = data
	writeAPIJSON(w, map[string]string{"name": name, "size": strconv.Itoa(len(data))})
}

func (f *fakeGCS) handleGet(w http.ResponseWriter, r *http.Request, name string) {
	f.mu.Lock()
	f.calls = append(f.calls, "GET "+name)
	data, ok := f.objects[name]
	f.mu.Unlock()
	switch {
	case !ok:
		writeAPIError(w, http.StatusNotFound, "No such object: "+name)
	case r.URL.Query().Get("alt") == "media":
		_, _ = w.Write(data)
	default:
		writeAPIJSON(w, map[string]string{"size": strconv.Itoa(len(data))})
	}
}

func writeAPIJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeAPIError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = fmt.Fprintf(w, `{"error":{"code":%d,"message":%q}}`, code, msg)
}
