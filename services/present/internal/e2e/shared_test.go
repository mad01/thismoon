// Package e2e drives a deployed shared instance over plain HTTP, the way a
// client would. It is off unless PRESENT_E2E_URL names the instance, so
// `go test ./...` never needs a cluster; scripts/kind-e2e.sh sets it to a
// port-forward into kind, locally and in CI.
package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
)

type client struct {
	t    *testing.T
	base string
}

func (c client) do(method, path, key string, body any) (int, []byte) {
	c.t.Helper()
	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		payload = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.base+path, payload)
	if err != nil {
		c.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

func TestSharedInstance(t *testing.T) {
	base := strings.TrimRight(os.Getenv("PRESENT_E2E_URL"), "/")
	if base == "" {
		t.Skip("set PRESENT_E2E_URL to a running shared instance (scripts/kind-e2e.sh does)")
	}
	c := client{t: t, base: base}
	key, other := "e2e-"+strings.Repeat("a", 60), "e2e-"+strings.Repeat("b", 60)

	code, body := c.do(http.MethodGet, "/version", "", nil)
	if code != http.StatusOK {
		t.Fatalf("GET /version = %d", code)
	}
	var info map[string]any
	if err := json.Unmarshal(body, &info); err != nil {
		t.Fatalf("/version is not JSON: %s", body)
	}
	for _, k := range []string{"version", "commit", "tag", "build_time"} {
		if _, ok := info[k]; !ok {
			t.Errorf("/version missing %q: %s", k, body)
		}
	}

	if code, body := c.do(http.MethodGet, "/", "", nil); code != http.StatusOK ||
		!strings.Contains(string(body), "data-howto") {
		t.Errorf("GET / = %d, want the how-to page", code)
	}

	page := map[string]any{"title": "e2e", "content": "<p>e2e</p>", "ephemeral": true}
	if code, _ := c.do(http.MethodPost, "/api/pages", "", page); code != http.StatusUnauthorized {
		t.Errorf("create without key = %d, want 401", code)
	}
	code, body = c.do(http.MethodPost, "/api/pages", key, page)
	if code != http.StatusCreated {
		t.Fatalf("create = %d: %s", code, body)
	}
	var created struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(body, &created); err != nil ||
		!regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(created.ID) {
		t.Fatalf("create returned %s", body)
	}
	if !strings.HasSuffix(created.URL, "/p/"+created.ID) {
		t.Errorf("url = %q, want it to end in /p/%s", created.URL, created.ID)
	}

	if code, _ := c.do(http.MethodGet, "/api/p/"+created.ID, "", nil); code != http.StatusOK {
		t.Errorf("read without key = %d, want 200", code)
	}
	if code, _ := c.do(http.MethodGet, "/api/pages", "", nil); code == http.StatusOK {
		t.Error("GET /api/pages must not list pages on a shared instance")
	}
	if code, _ := c.do(http.MethodPut, "/api/p/"+created.ID, other, page); code != http.StatusForbidden {
		t.Errorf("replace with another key = %d, want 403", code)
	}
	if code, _ := c.do(http.MethodPut, "/api/p/"+created.ID, key, page); code != http.StatusOK {
		t.Errorf("replace by author = %d, want 200", code)
	}
	if code, body := c.do(http.MethodGet, "/p/"+created.ID+"/version", "", nil); code != http.StatusOK ||
		strings.TrimSpace(string(body)) != "2" {
		t.Errorf("version after replace = %d %q, want 200 \"2\"", code, body)
	}
	if code, _ := c.do(http.MethodDelete, "/p/"+created.ID, other, nil); code != http.StatusForbidden {
		t.Errorf("delete with another key = %d, want 403", code)
	}
	if code, _ := c.do(http.MethodDelete, "/p/"+created.ID, key, nil); code != http.StatusNoContent {
		t.Errorf("delete by author = %d, want 204", code)
	}
	if code, _ := c.do(http.MethodGet, "/api/p/"+created.ID, "", nil); code != http.StatusNotFound {
		t.Errorf("read after delete = %d, want 404", code)
	}
}
