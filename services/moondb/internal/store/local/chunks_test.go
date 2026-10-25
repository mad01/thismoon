package local

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
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
