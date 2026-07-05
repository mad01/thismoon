package webkit

import (
	"testing"
	"testing/fstest"
)

// TestHashFSChangesWithBytes pins the cache-busting invariant (issue #1): the
// content hash is stable for identical bytes and changes the moment any asset
// byte changes — which is exactly what the old pseudo-version ETag failed to do.
func TestHashFSChangesWithBytes(t *testing.T) {
	base := fstest.MapFS{
		"dist/webkit.css": {Data: []byte("a{}")},
		"dist/webkit.js":  {Data: []byte("console.log(1)")},
	}
	changed := fstest.MapFS{
		"dist/webkit.css": {Data: []byte("a{}")},
		"dist/webkit.js":  {Data: []byte("console.log(2)")}, // one byte differs
	}

	h1 := hashFS(base, "dist")
	h2 := hashFS(base, "dist")
	h3 := hashFS(changed, "dist")

	if h1 == "" || len(h1) != 16 {
		t.Fatalf("hashFS returned %q, want a 16-char digest", h1)
	}
	if h1 != h2 {
		t.Errorf("hashFS not deterministic: %q != %q", h1, h2)
	}
	if h1 == h3 {
		t.Errorf("hashFS did not change when asset bytes changed: both %q", h1)
	}
}
