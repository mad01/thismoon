package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2/google"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

const (
	// defaultEndpoint is the Cloud Storage JSON API host.
	defaultEndpoint = "https://storage.googleapis.com"
	// scopeReadWrite lets the credentials read and create objects.
	scopeReadWrite = "https://www.googleapis.com/auth/devstorage.read_write"
	// maxAttempts bounds the tries of one request that keeps failing in
	// transit or with a status Cloud Storage says to retry.
	maxAttempts = 4
	// firstBackoff is the wait before the second try; each later wait
	// doubles it.
	firstBackoff = 250 * time.Millisecond
	// maxErrorBody is how much of an error answer is read for its message.
	maxErrorBody = 4 << 10
)

// errExists is what create returns when it was asked to create an object
// only if absent and the bucket already holds one by that name.
var errExists = errors.New("cloud: object exists")

// Bucket is a thin client for one Cloud Storage bucket over the JSON API:
// create an object, read one, read an object's size, and check the bucket.
// It carries no SDK. NewBucket authenticates its requests with application
// default credentials (ADC); tests point one at an httptest server.
type Bucket struct {
	name     string
	endpoint string
	http     *http.Client
	// wait is how long to back off after failed try n, from 1.
	wait func(n int) time.Duration
}

// NewBucket returns a client for the bucket name, authenticated with the
// application default credentials: the workload identity of a pod, or
// GOOGLE_APPLICATION_CREDENTIALS. ctx bounds the life of the token source,
// so pass the process's.
func NewBucket(ctx context.Context, name string) (*Bucket, error) {
	if name == "" {
		return nil, errors.New("cloud: no bucket named")
	}
	hc, err := google.DefaultClient(ctx, scopeReadWrite)
	if err != nil {
		return nil, fmt.Errorf("cloud: application default credentials: %w", err)
	}
	return newBucket(name, defaultEndpoint, hc), nil
}

func newBucket(name, endpoint string, hc *http.Client) *Bucket {
	return &Bucket{
		name:     name,
		endpoint: strings.TrimRight(endpoint, "/"),
		http:     hc,
		wait:     func(n int) time.Duration { return firstBackoff << (n - 1) },
	}
}

// Name is the bucket's name.
func (b *Bucket) Name() string { return b.name }

// upload is one object to create: its name and size, a way to read its
// bytes that a retry opens again, and whether an object already there wins.
type upload struct {
	object      string
	size        int64
	open        func() (io.ReadCloser, error)
	contentType string
	ifAbsent    bool
}

// bytesBody opens data as an upload body.
func bytesBody(data []byte) func() (io.ReadCloser, error) {
	return func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil }
}

// create uploads u in one request. With u.ifAbsent the request carries
// ifGenerationMatch=0, so an object already there stays and create returns
// errExists.
func (b *Bucket) create(ctx context.Context, u upload) error {
	q := url.Values{"uploadType": {"media"}, "name": {u.object}}
	if u.ifAbsent {
		q.Set("ifGenerationMatch", "0")
	}
	target := b.endpoint + "/upload/storage/v1/b/" + url.PathEscape(b.name) + "/o?" + q.Encode()
	res, err := b.do(ctx, func() (*http.Request, error) {
		body, err := u.open()
		if err != nil {
			return nil, err
		}
		if u.size == 0 {
			// A nil-length body would go out chunked; say it is empty.
			_ = body.Close()
			body = http.NoBody
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, body)
		if err != nil {
			_ = body.Close()
			return nil, err
		}
		req.ContentLength = u.size
		req.Header.Set("Content-Type", u.contentType)
		return req, nil
	})
	if err != nil {
		return fmt.Errorf("cloud: create %s: %w", u.object, err)
	}
	defer drain(res)
	switch {
	case u.ifAbsent && res.StatusCode == http.StatusPreconditionFailed:
		return errExists
	case res.StatusCode/100 != 2:
		return statusErr(res, "create", u.object)
	}
	return nil
}

// read returns the bytes of object. A missing object wraps
// store.ErrNotFound.
func (b *Bucket) read(ctx context.Context, object string) ([]byte, error) {
	res, err := b.get(ctx, b.objectURL(object, "alt=media"))
	if err != nil {
		return nil, fmt.Errorf("cloud: read %s: %w", object, err)
	}
	defer drain(res)
	if res.StatusCode != http.StatusOK {
		return nil, statusErr(res, "read", object)
	}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("cloud: read %s: %w", object, err)
	}
	return data, nil
}

// size returns the size of object from its metadata. A missing object
// wraps store.ErrNotFound.
func (b *Bucket) size(ctx context.Context, object string) (int64, error) {
	res, err := b.get(ctx, b.objectURL(object, "fields=size"))
	if err != nil {
		return 0, fmt.Errorf("cloud: stat %s: %w", object, err)
	}
	defer drain(res)
	if res.StatusCode != http.StatusOK {
		return 0, statusErr(res, "stat", object)
	}
	var meta struct {
		Size string `json:"size"`
	}
	if err := json.NewDecoder(res.Body).Decode(&meta); err != nil {
		return 0, fmt.Errorf("cloud: stat %s: %w", object, err)
	}
	n, err := strconv.ParseInt(meta.Size, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("cloud: stat %s: size %q: %w", object, meta.Size, err)
	}
	return n, nil
}

// check reads the bucket's metadata: it exists and the credentials reach it.
func (b *Bucket) check(ctx context.Context) error {
	res, err := b.get(ctx, b.endpoint+"/storage/v1/b/"+url.PathEscape(b.name)+"?fields=name")
	if err != nil {
		return fmt.Errorf("cloud: bucket %s: %w", b.name, err)
	}
	defer drain(res)
	if res.StatusCode != http.StatusOK {
		return statusErr(res, "check bucket", b.name)
	}
	return nil
}

// objectURL is the JSON API URL of object with query; the name is escaped
// whole, its slashes included, as the API wants.
func (b *Bucket) objectURL(object, query string) string {
	return b.endpoint + "/storage/v1/b/" + url.PathEscape(b.name) +
		"/o/" + url.PathEscape(object) + "?" + query
}

func (b *Bucket) get(ctx context.Context, target string) (*http.Response, error) {
	return b.do(ctx, func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	})
}

// do sends the request build makes, and again after a backoff when the
// try failed in transit or got 408, 429 or a 5xx, the answers Cloud
// Storage says to retry, at most maxAttempts times. Every request do sends
// is safe to repeat: a read, a create only if absent, or a whole-object
// write of the same bytes.
func (b *Bucket) do(
	ctx context.Context,
	build func() (*http.Request, error),
) (*http.Response, error) {
	for n := 1; ; n++ {
		req, err := build()
		if err != nil {
			return nil, err
		}
		res, err := b.http.Do(req)
		if !retryable(res, err) || n == maxAttempts || ctx.Err() != nil {
			return res, err
		}
		if res != nil {
			drain(res)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(b.wait(n)):
		}
	}
}

func retryable(res *http.Response, err error) bool {
	if err != nil {
		return true
	}
	c := res.StatusCode
	return c == http.StatusRequestTimeout || c == http.StatusTooManyRequests || c >= 500
}

// drain reads what is left of a body so the connection can be reused, and
// closes it.
func drain(res *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, maxErrorBody))
	_ = res.Body.Close()
}

// statusErr names a failed answer with the message Cloud Storage gave. A
// 404 wraps store.ErrNotFound.
func statusErr(res *http.Response, op, name string) error {
	if res.StatusCode == http.StatusNotFound {
		return fmt.Errorf("cloud: %s %s: %w", op, name, store.ErrNotFound)
	}
	raw, _ := io.ReadAll(io.LimitReader(res.Body, maxErrorBody))
	var body struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	msg := strings.TrimSpace(string(raw))
	if json.Unmarshal(raw, &body) == nil && body.Error.Message != "" {
		msg = body.Error.Message
	}
	return fmt.Errorf("cloud: %s %s: %s: %s", op, name, res.Status, msg)
}
