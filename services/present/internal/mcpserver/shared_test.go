package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/kit/mcptest"
	present "github.com/mad01/thismoon/services/present"
	"github.com/mad01/thismoon/services/present/internal/author"
	"github.com/mad01/thismoon/services/present/internal/store"
)

func noChecks(context.Context) []doctor.Check { return nil }

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func toolNames(t *testing.T, s *mcp.Server) []string {
	t.Helper()
	var names []string
	for _, tool := range mcptest.ListTools(t, s) {
		names = append(names, tool.Name)
	}
	return names
}

func TestToolSetsPerMode(t *testing.T) {
	cases := []struct {
		mode Mode
		want []string
	}{
		{ModeLocal, []string{
			"present_create", "present_read", "present_source", "present_update",
			"present_list", "present_open", "present_doctor",
		}},
		{ModeShared, []string{
			"present_create", "present_read", "present_source", "present_update", "present_doctor",
		}},
	}
	for _, tc := range cases {
		s, err := New(
			"test",
			Config{Workdir: t.TempDir(), Port: 7423, Mode: tc.mode, Checks: noChecks},
		)
		if err != nil {
			t.Fatalf("New(mode %d): %v", tc.mode, err)
		}
		got := toolNames(t, s)
		slices.Sort(got)
		slices.Sort(tc.want)
		if !slices.Equal(got, tc.want) {
			t.Errorf("mode %d tools = %v, want %v", tc.mode, got, tc.want)
		}
		mcptest.VerifyToolAnnotations(t, s)
	}
}

func TestSharedInstructionsDescribeTheInstance(t *testing.T) {
	s, err := New("test", Config{Workdir: t.TempDir(), Mode: ModeShared, Checks: noChecks})
	if err != nil {
		t.Fatal(err)
	}
	tools := mcptest.ListTools(t, s)
	var create *mcp.Tool
	for _, tool := range tools {
		if tool.Name == "present_create" {
			create = tool
		}
	}
	if create == nil {
		t.Fatal("no present_create")
	}
	if !strings.Contains(create.Description, "ephemeral") ||
		!strings.Contains(create.Description, "author key") {
		t.Errorf(
			"shared create description must explain ephemeral and the author key: %q",
			create.Description,
		)
	}
	if !strings.Contains(string(mustJSON(t, create.InputSchema)), `"ephemeral"`) {
		t.Errorf("shared create schema must expose ephemeral: %s", mustJSON(t, create.InputSchema))
	}
}

// sharedHandlers builds handlers in shared mode over a fresh filesystem
// store with a fixed clock.
func sharedHandlers(t *testing.T) (*handlers, *time.Time) {
	t.Helper()
	st, err := store.NewFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	h := &handlers{
		store: st,
		mode:  ModeShared,
		now:   func() time.Time { return now },
		open:  func(string) error { return nil },
	}
	return h, &now
}

func request(key string, extra map[string]string) *mcp.CallToolRequest {
	hdr := http.Header{}
	if key != "" {
		hdr.Set("Authorization", "Bearer "+key)
	}
	for k, v := range extra {
		hdr.Set(k, v)
	}
	return &mcp.CallToolRequest{Extra: &mcp.RequestExtra{Header: hdr}}
}

func TestSharedCreateNeedsKeyAndSetsAuthor(t *testing.T) {
	h, now := sharedHandlers(t)
	ctx := context.Background()

	_, _, err := h.handleCreateShared(ctx, request("", nil), sharedCreateInput{
		createInput: createInput{Title: "T", Content: "<p>x</p>"},
	})
	if !errors.Is(err, author.ErrMissing) {
		t.Fatalf("create without key: err = %v, want ErrMissing", err)
	}
	_, _, err = h.handleCreateShared(ctx, nil, sharedCreateInput{
		createInput: createInput{Title: "T", Content: "<p>x</p>"},
	})
	if !errors.Is(err, author.ErrMissing) {
		t.Fatalf("create with no request: err = %v, want ErrMissing", err)
	}

	key := author.NewKey()
	req := request(
		key,
		map[string]string{"X-Forwarded-Host": "present.example.com", "X-Forwarded-Proto": "https"},
	)
	_, out, err := h.handleCreateShared(ctx, req, sharedCreateInput{
		createInput: createInput{Title: "T", Content: "<p>x</p>"},
		Ephemeral:   true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(out.ID) {
		t.Errorf("id = %q, want a 32-hex capability id", out.ID)
	}
	if want := "https://present.example.com/p/" + out.ID; out.URL != want {
		t.Errorf("url = %q, want %q", out.URL, want)
	}
	if !out.Ephemeral || out.ExpiresAt != now.Add(present.SharedTTL).Format(time.RFC3339) {
		t.Errorf("ephemeral output = %+v", out)
	}
	p, _ := h.store.Get(ctx, out.ID)
	if p.Author != author.Hash(key) {
		t.Errorf("stored author = %q, want hash of key", p.Author)
	}
}

func TestSharedUpdateIsAuthorOnlyAndResetsExpiry(t *testing.T) {
	h, now := sharedHandlers(t)
	ctx := context.Background()
	owner, other := author.NewKey(), author.NewKey()
	_, created, err := h.handleCreateShared(ctx, request(owner, nil), sharedCreateInput{
		createInput: createInput{Title: "T", Content: "<p>v1</p>"},
		Ephemeral:   true,
	})
	if err != nil {
		t.Fatal(err)
	}

	in := updateInput{ID: created.ID, Content: ptrStr("<p>v2</p>")}
	if _, _, err := h.handleUpdate(ctx, request(other, nil), in); !errors.Is(
		err,
		author.ErrMismatch,
	) {
		t.Errorf("update with other key: err = %v, want ErrMismatch", err)
	}
	if _, _, err := h.handleUpdate(ctx, request("", nil), in); !errors.Is(err, author.ErrMissing) {
		t.Errorf("update without key: err = %v, want ErrMissing", err)
	}

	*now = now.Add(5 * 24 * time.Hour)
	_, out, err := h.handleUpdate(ctx, request(owner, nil), in)
	if err != nil {
		t.Fatalf("update by owner: %v", err)
	}
	if out.Version != 2 {
		t.Errorf("version = %d, want 2", out.Version)
	}
	if want := now.Add(present.SharedTTL).Format(time.RFC3339); out.ExpiresAt != want {
		t.Errorf("expires_at = %q, want %q (reset on update)", out.ExpiresAt, want)
	}
}

func TestLocalCreateIgnoresBearer(t *testing.T) {
	h, _ := newTestHandlers(t)
	_, out, err := h.handleCreate(
		context.Background(),
		request(author.NewKey(), nil),
		createInput{Title: "T", Content: "<p>x</p>"},
	)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := h.store.Get(context.Background(), out.ID)
	if p.Author != "" || len(out.ID) != 10 {
		t.Errorf(
			"local create must mint a local id with no author: id %q author %q",
			out.ID,
			p.Author,
		)
	}
}
