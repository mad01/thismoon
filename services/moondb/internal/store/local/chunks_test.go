package local

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

func gzipped(t *testing.T, data string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(data)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestChunkCorruptionIsDetected(t *testing.T) {
	ctx := context.Background()
	c := &chunkStore{dir: t.TempDir()}
	sha, err := c.Put(ctx, []byte("hello"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	cases := []struct {
		name    string
		content []byte
		want    error
	}{
		{"other content", gzipped(t, "goodbye"), errCorruptChunk},
		{"not gzip", []byte("garbage"), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(c.path(sha), tc.content, filePerm); err != nil {
				t.Fatal(err)
			}
			_, err := c.Get(ctx, sha)
			if err == nil || (tc.want != nil && !errors.Is(err, tc.want)) {
				t.Errorf("Get of a damaged object: err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestChunkKeysAreChecked(t *testing.T) {
	ctx := context.Background()
	c := &chunkStore{dir: t.TempDir()}
	sha, err := c.Put(ctx, []byte("hello"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	for _, key := range []string{"", "../../etc/passwd", strings.ToUpper(sha), sha[:63] + "g", sha + "0"} {
		if _, err := c.Get(ctx, key); err == nil {
			t.Errorf("Get(%q) succeeded, want an invalid-key error", key)
		}
		if _, err := c.Has(ctx, key); err == nil {
			t.Errorf("Has(%q) succeeded, want an invalid-key error", key)
		}
	}
}

// TestChunkSizeReadsTheTrailer: size gives the length Put stored, read
// from the gzip trailer; a key with no object is not found, and an object
// too short to be gzip is damaged.
func TestChunkSizeReadsTheTrailer(t *testing.T) {
	ctx := context.Background()
	c := &chunkStore{dir: t.TempDir()}
	for _, data := range [][]byte{{}, []byte("hello"), bytes.Repeat([]byte("line\n"), 300_000)} {
		sha, err := c.Put(ctx, data)
		if err != nil {
			t.Fatalf("Put: %v", err)
		}
		if n, err := c.size(sha); err != nil || n != int64(len(data)) {
			t.Errorf("size of %d bytes = %d, %v", len(data), n, err)
		}
	}
	missing := strings.Repeat("0", 64)
	if _, err := c.size(missing); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("size of a missing object: err = %v, want ErrNotFound", err)
	}
	sha, err := c.Put(ctx, []byte("hello"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := os.WriteFile(c.path(sha), []byte("short"), filePerm); err != nil {
		t.Fatal(err)
	}
	if _, err := c.size(sha); !errors.Is(err, errCorruptChunk) {
		t.Errorf("size of a short object: err = %v, want errCorruptChunk", err)
	}
}
