package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mad01/thismoon/buildinfo"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/query"
	"github.com/mad01/thismoon/services/moondb/internal/replicate"
	"github.com/mad01/thismoon/services/moondb/internal/server/servertest"
	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/store/local"
)

const testKey = "s3cret-key"

func hashOf(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func testKeys(t *testing.T) Keys {
	t.Helper()
	k, err := ParseKeys(strings.NewReader("# the laptop\n" + hashOf(testKey) + "\n\n"))
	if err != nil {
		t.Fatalf("ParseKeys: %v", err)
	}
	return k
}

func TestParseKeys(t *testing.T) {
	cases := []struct {
		name    string
		file    string
		wantErr bool
	}{
		{"one hash among comments", "# a\n" + hashOf("k") + "\n", false},
		{"no hash", "# nothing here\n\n", true},
		{"a key instead of its hash", "k\n", true},
		{"uppercase hex", strings.ToUpper(hashOf("k")) + "\n", true},
		{"the empty key", hashOf("") + "\n", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseKeys(strings.NewReader(c.file))
			if (err != nil) != c.wantErr {
				t.Errorf("ParseKeys = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}

func TestKeysAllows(t *testing.T) {
	k := testKeys(t)
	for key, want := range map[string]bool{testKey: true, "other": false, "": false} {
		if got := k.Allows(key); got != want {
			t.Errorf("Allows(%q) = %v, want %v", key, got, want)
		}
	}
}

// sharedServer serves the fixture store the way a shared instance does,
// with testKey accepted and a stand-in at /mcp.
func sharedServer(t *testing.T) *httptest.Server {
	t.Helper()
	st := servertest.NewStore()
	s := New(st, servertest.NewQuerier(st), buildinfo.Info{})
	mcp := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	ts := httptest.NewServer(s.SharedHandler(SharedConfig{Keys: testKeys(t), MCP: mcp}))
	t.Cleanup(ts.Close)
	return ts
}

func TestSharedHandlerNeedsAKey(t *testing.T) {
	ts := sharedServer(t)
	bearer := func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+testKey) }
	wrong := func(r *http.Request) { r.Header.Set("Authorization", "Bearer nope") }
	cookie := func(r *http.Request) { r.AddCookie(&http.Cookie{Name: moondb.KeyCookie, Value: testKey}) }
	none := func(*http.Request) {}
	cases := []struct {
		name   string
		method string
		path   string
		sign   func(*http.Request)
		want   int
	}{
		{"page shell is open", "GET", "/", none, http.StatusOK},
		{"page script is open", "GET", "/app.js", none, http.StatusOK},
		{"healthz is open", "GET", "/healthz", none, http.StatusNoContent},
		{"version is open", "GET", "/version", none, http.StatusOK},
		{"a listing needs a key", "GET", "/api/sessions", none, http.StatusUnauthorized},
		{"a wrong key is refused", "GET", "/api/sessions", wrong, http.StatusUnauthorized},
		{"a bearer key reads", "GET", "/api/sessions", bearer, http.StatusOK},
		{"the page cookie reads", "GET", "/api/sessions", cookie, http.StatusOK},
		{
			"a chunk needs a key",
			"GET",
			"/api/chunks/" + servertest.PromptSHA,
			none,
			http.StatusUnauthorized,
		},
		{"the cookie cannot sync", "POST", "/api/sync", cookie, http.StatusUnauthorized},
		{"the cookie cannot reach mcp", "POST", "/mcp", cookie, http.StatusUnauthorized},
		{"a bearer key reaches mcp", "POST", "/mcp", bearer, http.StatusTeapot},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req, err := http.NewRequest(c.method, ts.URL+c.path, strings.NewReader("{}"))
			if err != nil {
				t.Fatal(err)
			}
			c.sign(req)
			res, err := ts.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = res.Body.Close()
			if res.StatusCode != c.want {
				t.Errorf("%s %s = %d, want %d", c.method, c.path, res.StatusCode, c.want)
			}
		})
	}
}

// syncServer serves a shared instance over a local store in a temp dir and
// returns the store beside it.
func syncServer(t *testing.T) (*httptest.Server, *local.Store) {
	t.Helper()
	st, err := local.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	s := New(st, &query.Service{Store: st, NoCheckouts: true}, buildinfo.Info{})
	ts := httptest.NewServer(s.SharedHandler(SharedConfig{Keys: testKeys(t)}))
	t.Cleanup(ts.Close)
	return ts, st
}

func postSync(t *testing.T, ts *httptest.Server, path string, body any) (int, []byte) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+path, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testKey)
	res, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(res.Body)
	return res.StatusCode, buf.Bytes()
}

func TestSyncTakesChunksThenRecords(t *testing.T) {
	ts, st := syncServer(t)
	data := []byte("after content\n")
	sha := store.ChunkKey(data)

	code, body := postSync(
		t,
		ts,
		"/api/sync/missing",
		replicate.MissingRequest{Chunks: []string{sha}},
	)
	if code != http.StatusOK || !strings.Contains(string(body), sha) {
		t.Fatalf("missing = %d %s, want 200 naming %s", code, body, sha)
	}
	edit, _ := json.Marshal(store.Edit{ID: "e1", SessionID: "s1", AfterChunk: sha, Path: "x.go"})
	batch := replicate.Batch{
		Chunks: [][]byte{data},
		Records: []local.Line{
			{Kind: "edit", Record: edit},
			{Kind: "future", Record: json.RawMessage(`{}`)},
		},
	}
	code, body = postSync(t, ts, "/api/sync", batch)
	var res replicate.BatchResult
	if err := json.Unmarshal(body, &res); err != nil || code != http.StatusOK {
		t.Fatalf("sync = %d %s", code, body)
	}
	if res != (replicate.BatchResult{Chunks: 1, Records: 1, Skipped: 1}) {
		t.Errorf("sync result = %+v, want 1 chunk, 1 record, 1 skipped", res)
	}
	if ok, err := st.Chunks().Has(context.Background(), sha); !ok || err != nil {
		t.Errorf("Has(%s) = %v, %v; want the chunk stored", sha, ok, err)
	}
	if edits, err := st.Edits(context.Background(), store.EditQuery{}); err != nil ||
		len(edits) != 1 {
		t.Errorf("edits = %d, %v; want the one sent", len(edits), err)
	}
}

func TestSyncRefusesABadBatch(t *testing.T) {
	ts, _ := syncServer(t)
	cases := []struct {
		name string
		path string
		body any
	}{
		{
			"a key that is not one",
			"/api/sync/missing",
			replicate.MissingRequest{Chunks: []string{"../x"}},
		},
		{"a record that does not decode", "/api/sync", replicate.Batch{Records: []local.Line{
			{Kind: "edit", Record: json.RawMessage(`{"id": 7}`)},
		}}},
		{"a body that is not JSON", "/api/sync", "not a batch"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if code, body := postSync(t, ts, c.path, c.body); code != http.StatusBadRequest {
				t.Errorf("POST %s = %d %s, want 400", c.path, code, body)
			}
		})
	}
}
